package rocm

import (
	"os"
	"path/filepath"
	"sort"
)

// venvLibDirs are the venv-relative (site-packages) library directories of
// the TheRock ROCm wheels and torch, in load order. _rocm_sdk_libraries
// holds the wheel rocBLAS and must load before any system rocBLAS.
var venvLibDirs = []string{
	"_rocm_sdk_libraries/lib",
	"_rocm_sdk_core/lib/host-math/lib",
	"_rocm_sdk_core/lib/rocm_sysdeps/lib",
	"_rocm_sdk_core/lib/core/lib",
	"torch/lib",
}

// SitePackages returns the venv's lib/python3.*/site-packages directories,
// newest Python first.
func SitePackages(venv string) []string {
	if venv == "" {
		return nil
	}
	m, _ := filepath.Glob(filepath.Join(venv, "lib", "python3.*", "site-packages"))
	sort.Sort(sort.Reverse(sort.StringSlice(m)))
	return m
}

// VenvLibPaths returns the existing ROCm wheel and torch library dirs of a
// venv, in LD_LIBRARY_PATH order.
func VenvLibPaths(venv string) []string {
	var out []string
	for _, sp := range SitePackages(venv) {
		for _, d := range venvLibDirs {
			p := filepath.Join(sp, d)
			if isDir(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// systemROCBLAS are the system rocBLAS locations, relative to the root.
var systemROCBLAS = []string{
	"opt/rocm/lib/librocblas.so.5",
	"usr/lib64/librocblas.so.5",
	"usr/lib/x86_64-linux-gnu/librocblas.so.5",
	"usr/lib/librocblas.so.5",
}

// FindROCBLAS returns the librocblas.so.5 that a venv process loads: the
// wheel copy in the venv first, then the system one under root. "" when
// none exists.
func FindROCBLAS(root, venv string) string {
	for _, sp := range SitePackages(venv) {
		for _, rel := range []string{"_rocm_sdk_libraries/lib/librocblas.so.5", "torch/lib/librocblas.so.5"} {
			if p := filepath.Join(sp, rel); isFile(p) {
				return p
			}
		}
	}
	if root == "" {
		root = "/"
	}
	for _, rel := range systemROCBLAS {
		if p := filepath.Join(root, rel); isFile(p) {
			return p
		}
	}
	return ""
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
