package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
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

var prereleaseRegex = regexp.MustCompile("[a-zA-Z]")
var plusMinusRegex = regexp.MustCompile("[-+]")

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
	if semver == "*" || semver == "latest" {
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

func install(initialDeps map[string]string) error {
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
				return err
			}
			err = p.Install()
			if err != nil {
				return err
			}
			installed[name] = struct{}{}
			maps.Copy(deps, p.Dependencies)
			count += 1
		}
	}
	fmt.Printf("Installed %d packages\n", count)
	// 539
	return nil
}

func (p Package) Install() error {
	if p.Tarball == "" {
		return errors.New("No tarball found for " + p.Name)
	}
	return nil
}

func postinstall() error {
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
	err = os.MkdirAll("node_modules", 0755)
	if err != nil {
		fmt.Printf("Error mkdir: %v\n", err)
		return
	}
	err = install(initialDeps)
	if err != nil {
		fmt.Printf("Error install: %v\n", err)
		return
	}
	err = postinstall()
	if err != nil {
		fmt.Printf("Error postinstall: %v\n", err)
		return
	}
}
