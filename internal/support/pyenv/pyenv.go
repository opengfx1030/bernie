// Package pyenv reads the layout of a Python virtual environment on disk.
//
// It never starts an interpreter. A venv that imports vLLM at startup (a
// sitecustomize.py that pulls in torch, for example) takes seconds to
// start, and the answers bernie needs — the site-packages directories, the
// modules the venv holds — do not change between two reads of the same
// tree. The venv's Python is started only when bernie needs the version or
// the imports themselves (internal/machine/rocm).
package pyenv

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SitePackages returns the venv's own lib/python3.*/site-packages
// directories, newest Python first.
func SitePackages(venv string) []string {
	if venv == "" {
		return nil
	}
	m, _ := filepath.Glob(filepath.Join(venv, "lib", "python3.*", "site-packages"))
	sort.Sort(sort.Reverse(sort.StringSlice(m)))
	return m
}

// SearchPaths returns the directories the venv's Python imports from: the
// venv's own site-packages, plus the site-packages of the base prefix when
// the venv was made with --system-site-packages.
func SearchPaths(venv string) []string {
	out := SitePackages(venv)
	base, ok := systemPrefix(venv)
	if !ok {
		return out
	}
	m, _ := filepath.Glob(filepath.Join(base, "lib", "python3.*", "site-packages"))
	sort.Sort(sort.Reverse(sort.StringSlice(m)))
	return append(out, m...)
}

// HasModule reports whether the venv's Python can find the module. It looks
// for the package directory, the module file and the .pth file that
// setuptools writes for a package installed from a source tree (an
// editable install).
func HasModule(venv, module string) bool {
	if venv == "" || module == "" {
		return false
	}
	for _, dir := range SearchPaths(venv) {
		if hasModuleIn(dir, module) {
			return true
		}
	}
	return false
}

// hasModuleIn reports whether one directory holds the module.
func hasModuleIn(dir, module string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case name == module, name == module+".py":
			return true
		case strings.HasPrefix(name, "__editable__."+module) && strings.HasSuffix(name, ".pth"):
			return true
		}
	}
	return false
}

// systemPrefix returns the base prefix of the venv and whether the venv
// imports the site-packages of that prefix.
func systemPrefix(venv string) (string, bool) {
	home, shared := "", false
	for _, line := range strings.Split(readFile(filepath.Join(venv, "pyvenv.cfg")), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "include-system-site-packages":
			shared = strings.EqualFold(v, "true")
		case "home":
			home = v
		}
	}
	if !shared || home == "" {
		return "", false
	}
	// "home = /usr/bin" -> the prefix is /usr.
	return filepath.Dir(home), true
}

// readFile returns the content of a file, or "".
func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}
