package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
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

func resolvePackage(name string, semver string) (p Package, err error) {
	deps := map[string]string{}
	tarball := ""

	m, err := getMetadata("react-dom")
	if err != nil {
		return p, err
	}
	v, exact := m.Versions[semver]
	if exact {
		maps.Copy(deps, v.Dependencies)
		maps.Copy(deps, v.OptionalDependencies)
		return Package{Dependencies: deps, Tarball: tarball}, nil
	}

	//allVersions := maps.Keys(m.Versions)
	return p, nil
}

func install(initialDeps map[string]string) error {
	deps := make(map[string]string)
	maps.Copy(deps, initialDeps)
	for name, semver := range deps {
		fmt.Printf("Installing %s@%s\n", name, semver)
		p, err := resolvePackage(name, semver)
		if err != nil {
			return err
		}
		maps.Copy(deps, p.Dependencies)
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
		fmt.Printf("Error inital: %w\n", err)
		return
	}
	err = os.MkdirAll("node_modules", 0755)
	if err != nil {
		fmt.Printf("Error mkdir: %w\n", err)
		return
	}
	err = install(initialDeps)
	if err != nil {
		fmt.Printf("Error install: %w\n", err)
		return
	}
	err = postinstall()
	if err != nil {
		fmt.Printf("Error postinstall: %w\n", err)
		return
	}
}
