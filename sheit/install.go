package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Package struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Tarball              string            `json:"tarball"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
}

type PostPackage struct {
	Postinstall string
}

type PostPackageRaw struct {
	Bin     any `json:"bin"`
	Scripts struct {
		Postinstall string `json:"postinstall"`
	} `json:"scripts"`
}

type Version struct {
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	Dist                 struct {
		Tarball string `json:"tarball"`
	} `json:"dist"`
}

type Metadata struct {
	Versions map[string]Version `json:"versions"`
}

func getMetadata(name string) (m Metadata, err error) {
	r, err := http.Get("https://registry.npmjs.com/" + name)
	if err != nil {
		return m, err
	}
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		return m, err
	}
	return m, nil
}

var (
	prereleaseRegex = regexp.MustCompile("[a-zA-Z]")
	plusMinusRegex  = regexp.MustCompile("[-+]")
)

func versionCmp(a, b string) int {
	return slices.Compare(versionKey(a), versionKey(b))
}

func versionKey(version string) []int {
	parts := make([]int, 3)
	for _, p := range strings.Split(version, ".") {
		n := plusMinusRegex.Split(p, -1)[0]
		r, err := strconv.Atoi(n)
		if err == nil {
			parts = append(parts, r)
		}
	}
	return parts
}

func getLatest(versions []string) string {
	valid := make([]string, 0)
	for _, v := range versions {
		firstPart := strings.Split(v, ".")[0]
		if !prereleaseRegex.Match([]byte(firstPart)) {
			valid = append(valid, v)
		}
	}

	if len(valid) > 0 {
		slices.SortFunc(valid, versionCmp)
		return valid[len(valid)-1]
	}
	if len(versions) > 0 {
		slices.SortFunc(versions, versionCmp)
		return versions[len(versions)-1]
	}
	return ""
}

func matchCaret(ver string, versions []string) string {
	major := strings.Split(ver, ".")[0]
	matching := make([]string, 0)
	for _, v := range versions {
		if strings.Split(v, ".")[0] == major {
			matching = append(matching, v)
		}
	}
	return getLatest(matching)
}

func matchTilde(ver string, versions []string) string {
	parts := strings.Split(ver, ".")
	if len(parts) < 2 {
		return matchCaret(ver, versions)
	}
	major, minor := parts[0], parts[1]
	matching := make([]string, 0)
	for _, v := range versions {
		vParts := strings.Split(v, ".")
		if len(vParts) < 2 {
			continue
		}
		if vParts[0] == major && vParts[1] == minor {
			matching = append(matching, v)
		}
	}
	return getLatest(matching)
}

func matchRange(ver string, versions []string) string {
	parts := strings.Split(ver, ".")
	if len(parts) < 3 {
		return matchTilde(ver, versions)
	}
	major, minor, patch := parts[0], parts[1], parts[2]
	matching := make([]string, 0)
	for _, v := range versions {
		vParts := strings.Split(v, ".")
		if len(vParts) < 3 {
			continue
		}
		if vParts[0] == major && vParts[1] == minor && vParts[2] == patch {
			matching = append(matching, v)
		}
	}
	return getLatest(matching)
}

func resolvePackage(name string, semver string) (p Package, err error) {
	deps := map[string]string{}

	m, err := getMetadata(name)
	if err != nil {
		return p, err
	}
	version := ""
	allVersions := slices.Collect(maps.Keys(m.Versions))
	if semver == "*" || semver == "latest" || semver == "" {
		version = getLatest(allVersions)
	} else if strings.HasPrefix(semver, "^") {
		version = matchCaret(semver[1:], allVersions)
	} else if strings.HasPrefix(semver, "~") {
		version = matchTilde(semver[1:], allVersions)
	} else if strings.HasPrefix(semver, ">=") {
		version = matchRange(semver[2:], allVersions)
	} else {
		version = matchCaret(semver, allVersions)
	}

	v, exact := m.Versions[version]
	if exact {
		maps.Copy(deps, v.Dependencies)
		maps.Copy(deps, v.OptionalDependencies)
		return Package{Name: name, Dependencies: deps, Tarball: v.Dist.Tarball}, nil
	}
	return p, errors.New("No matching version found for " + semver)
}

func install(initialDeps map[string]string) (pps map[string]PostPackage, err error) {
	pps = make(map[string]PostPackage)
	count := 0
	deps := make(map[string]string)
	installed := make(map[string]struct{})
	maps.Copy(deps, initialDeps)
	for count < len(deps) {
		for name, semver := range deps {
			if _, skip := installed[name]; skip {
				continue
			}
			fmt.Printf("Installing %s@%s\n", name, semver)
			p, err := resolvePackage(name, semver)
			if err != nil {
				return pps, err
			}
			pp, err := p.Install()
			if err != nil {
				return pps, err
			}
			pps[name] = pp
			installed[name] = struct{}{}
			maps.Copy(deps, p.Dependencies)
			count += 1
		}
	}
	fmt.Printf("Installed %d packages\n", count)
	return pps, nil
}

func (p Package) Install() (pp PostPackage, err error) {
	if p.Tarball == "" {
		return pp, errors.New("No tarball found for " + p.Name)
	}
	resp, err := http.Get(p.Tarball)
	if err != nil {
		return pp, err
	}
	defer resp.Body.Close()

	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return pp, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return pp, err
		}
		fp := strings.Join(strings.Split(header.Name, "/")[1:], "/")
		target := filepath.Join("node_modules", p.Name, filepath.Clean(fp))
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return pp, err
			}
			io.Copy(f, tr)
			f.Close()
			if fp == "package.json" {
				f, err := os.Open(target)
				if err != nil {
					return pp, err
				}
				var ppr PostPackageRaw
				err = json.NewDecoder(f).Decode(&ppr)
				if err != nil {
					return pp, err
				}
				f.Close()
				err = p.createBinLinks(ppr.Bin)
				if err != nil {
					return pp, err
				}
				pp.Postinstall = ppr.Scripts.Postinstall
			}
		}
	}
	return pp, nil
}

func (p Package) createBinLinks(bin any) error {
	pkgDir := filepath.Join("node_modules", p.Name)
	binDir := filepath.Join("node_modules", ".bin")

	switch b := bin.(type) {
	case string:
		nameParts := strings.Split(p.Name, "/")
		name := nameParts[len(nameParts)-1]
		createSymlink(binDir, name, filepath.Join(pkgDir, b))
	case map[string]interface{}:
		for name, path := range b {
			spath, ok := path.(string)
			if ok {
				createSymlink(binDir, name, filepath.Join(pkgDir, spath))
			}
		}
	default:
	}
	return nil
}

func createSymlink(binDir string, name string, target string) {
	linkPath := filepath.Join(binDir, name)
	os.Remove(linkPath)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		fmt.Println("Failed to create symlink %s: %v", name, err)
		return
	}
	info, err := os.Stat(absTarget)

	if err == nil && !info.IsDir() {
		os.Chmod(absTarget, 0755)
		if err := os.Symlink(absTarget, linkPath); err == nil {
			fmt.Printf("Linked %s -> %s\n", name, absTarget)
		} else {
			fmt.Printf("Failed to create symlink %s: %v\n", name, err)
		}
	}
}

func postinstall(postPackages map[string]PostPackage) error {
	return nil
}

func getInitialDeps() (deps map[string]string, err error) {
	f, err := os.Open("package.json")
	if err != nil {
		return deps, err
	}
	defer f.Close()

	var p Package
	if err := json.NewDecoder(f).Decode(&p); err != nil {
		return deps, err
	}

	maps.Copy(p.Dependencies, p.OptionalDependencies)
	maps.Copy(p.Dependencies, p.DevDependencies)
	return p.Dependencies, nil
}

func main() {
	start := time.Now()
	defer func() { fmt.Printf("Done in %s\n", time.Since(start)) }()
	initialDeps, err := getInitialDeps()
	if err != nil {
		fmt.Printf("Error inital: %v\n", err)
		return
	}
	err = os.MkdirAll("node_modules/.bin", 0755)
	if err != nil {
		fmt.Printf("Error mkdir: %v\n", err)
		return
	}
	postPackages, err := install(initialDeps)
	if err != nil {
		fmt.Printf("Error install: %v\n", err)
		return
	}
	err = postinstall(postPackages)
	if err != nil {
		fmt.Printf("Error postinstall: %v\n", err)
		return
	}
}
