package host

import (
	"strings"
)

// AMD publishes ROCm in two repository shapes.
//
// The multi-arch repository carries one package per GPU architecture, for
// example `amdrocm-core-sdk7.14-gfx1030`. It serves ROCm 7.13 and later and
// has one directory per distribution family.
//
// The per-release repository carries the packages of one ROCm release. It
// serves ROCm 7.2 and earlier. Its package names do not name the GPU
// architecture.
//
// Sources: <https://repo.amd.com/rocm/packages-multi-arch/> (the directory
// list and the package index of every distribution) and
// <https://rocm.docs.amd.com/projects/install-on-linux/en/latest/>.
const (
	// MultiArchBase is the base URL of the multi-arch repository.
	MultiArchBase = "https://repo.amd.com/rocm/packages-multi-arch/"
	// MultiArchKeyURL is the ASCII-armored public key of the multi-arch
	// repository. Its fingerprint is
	// D0F004A0025A1145C7807FCD0701EAC4D5E02107 (AMD MLSE DevOps).
	MultiArchKeyURL = MultiArchBase + "gpg/rocm.gpg"
	// LegacyBase is the base URL of the per-release repository.
	LegacyBase = "https://repo.radeon.com/rocm/"
	// LegacyKeyURL is the ASCII-armored public key of the per-release
	// repository.
	LegacyKeyURL = "https://repo.radeon.com/rocm/rocm.gpg.key"
	// ROCmDocs is the AMD page that lists the supported distributions.
	ROCmDocs = "https://rocm.docs.amd.com/projects/install-on-linux/en/latest/"
)

// The ROCm lines bernie can install. The multi-arch repository serves
// ROCmVersion714 and later; the per-release repository serves
// ROCmVersion72.
const (
	// ROCmVersion714 is the newest validated ROCm line.
	ROCmVersion714 = "7.14"
	// ROCmVersion72 is the older ROCm line. It stays selectable because
	// some hosts need it.
	ROCmVersion72 = "7.2"
)

// ROCmRepo is one AMD package repository for one distribution.
type ROCmRepo struct {
	// Dir is the directory of the repository under MultiArchBase
	// ("rhel10", "ubuntu2404", "debian12"). It is empty for the
	// per-release repository.
	Dir string
	// Label names the distribution the repository serves ("rhel10",
	// "ubuntu2404", "el10", "noble").
	Label string
	// MultiArch tells whether the repository serves the per-architecture
	// packages.
	MultiArch bool
	// Family is the package family of the distribution: "rhel" (yum) or
	// "deb" (apt).
	Family string
	// Base is the base URL of the repository.
	Base string
	// Suite and Component are the apt suite and component. They are empty
	// for a yum repository.
	Suite, Component string
	// KeyURL is the URL of the ASCII-armored public key.
	KeyURL string
}

// ROCmRepository returns the repository that serves the ROCm line version on
// the distribution of osr. The second result is false when the distribution
// has no repository for that line.
//
// The multi-arch repository has a directory per distribution family. The
// per-release repository has an "elN" directory for the RHEL family and an
// apt suite per Debian and Ubuntu codename.
func ROCmRepository(osr OSRelease, version string) (ROCmRepo, bool) {
	dir, family, ok := rocmDistro(osr)
	if !ok {
		return ROCmRepo{}, false
	}
	if version == ROCmVersion72 {
		return rocmLegacyRepo(dir, family)
	}
	r := ROCmRepo{Dir: dir, Label: dir, MultiArch: true, Family: family, Suite: "stable", Component: "main", KeyURL: MultiArchKeyURL}
	if family == "rhel" {
		r.Base = MultiArchBase + dir + "/x86_64"
	} else {
		r.Base = MultiArchBase + dir
	}
	return r, true
}

// rocmLegacyRepo returns the per-release repository of ROCm 7.2.
func rocmLegacyRepo(dir, family string) (ROCmRepo, bool) {
	r := ROCmRepo{Family: family, KeyURL: LegacyKeyURL}
	switch family {
	case "rhel":
		// The per-release repository names the RHEL base of the
		// distribution: el8, el9 or el10.
		r.Label = "el" + dir[len("rhel"):]
		r.Base = LegacyBase + r.Label + "/7.2/main"
		return r, true
	case "deb":
		code, ok := legacyCodename[dir]
		if !ok {
			return ROCmRepo{}, false
		}
		r.Label = code
		r.Base = LegacyBase + "apt/7.2"
		r.Suite = code
		r.Component = "main"
		return r, true
	}
	return ROCmRepo{}, false
}

// legacyCodename maps a distribution of the multi-arch repository to the
// codename of the per-release apt repository. ROCm 7.2 has no apt suite for
// Debian, and none for Ubuntu 26.04.
var legacyCodename = map[string]string{
	"ubuntu2204": "jammy",
	"ubuntu2404": "noble",
	"debian12":   "bookworm",
	"debian13":   "trixie",
}

// ROCmPackages returns the packages that install the ROCm runtime, the HIP
// SDK, rocminfo, the SMI tools and rocBLAS for the code targets.
//
// The multi-arch repository names one package per GPU architecture. The
// per-release repository ships one package set for every architecture.
func ROCmPackages(version string, targets []string) []string {
	if version == ROCmVersion72 {
		return []string{
			"rocm-core", "rocm-hip-sdk", "hipcc", "rocminfo",
			"rocm-smi-lib", "amd-smi-lib", "rocblas", "rocm-device-libs",
			"rocm-developer-tools",
		}
	}
	if len(targets) == 0 {
		return []string{"amdrocm-core-sdk" + version, "amdrocm-core" + version, "amdrocm-blas" + version}
	}
	var out []string
	for _, t := range targets {
		out = append(out,
			"amdrocm-core-sdk"+version+"-"+t,
			"amdrocm-core"+version+"-"+t,
			"amdrocm-blas"+version+"-"+t,
		)
	}
	return out
}

// rocmDistro maps a distribution to its directory in the multi-arch
// repository and to its package family.
//
// Fedora 42 and later follow the RHEL 10 codebase, Fedora 39 to 41 the
// RHEL 9 one. The RHEL clones (Rocky, AlmaLinux, Oracle) name their RHEL
// base in ID_LIKE and keep their own version number.
func rocmDistro(osr OSRelease) (dir, family string, ok bool) {
	id := osr.ID
	major := versionMajor(osr.VersionID)
	switch id {
	case "fedora":
		switch {
		case major >= 42:
			return "rhel10", "rhel", true
		case major >= 39:
			return "rhel9", "rhel", true
		case major >= 8:
			return "rhel8", "rhel", true
		}
	case "rhel", "centos", "redhat", "rocky", "almalinux", "ol", "oracle":
		switch major {
		case 8:
			return "rhel8", "rhel", true
		case 9:
			return "rhel9", "rhel", true
		case 10:
			return "rhel10", "rhel", true
		}
	case "ubuntu":
		switch osr.VersionID {
		case "22.04":
			return "ubuntu2204", "deb", true
		case "24.04":
			return "ubuntu2404", "deb", true
		case "26.04":
			return "ubuntu2604", "deb", true
		}
	case "debian":
		switch major {
		case 12:
			return "debian12", "deb", true
		case 13:
			return "debian13", "deb", true
		}
	default:
		// ID_LIKE: "rhel fedora" on the RHEL clones, "debian" on Ubuntu
		// and its relatives.
		for _, l := range strings.Fields(osr.IDLike) {
			switch l {
			case "rhel":
				switch major {
				case 8:
					return "rhel8", "rhel", true
				case 9:
					return "rhel9", "rhel", true
				case 10:
					return "rhel10", "rhel", true
				}
			case "fedora":
				switch {
				case major >= 42:
					return "rhel10", "rhel", true
				case major >= 39:
					return "rhel9", "rhel", true
				case major >= 8:
					return "rhel8", "rhel", true
				}
			}
		}
	}
	return "", "", false
}

// versionMajor returns the major number of a version string ("43" of
// "43", 9 of "9.4"). It returns 0 when the version does not start with a
// number.
func versionMajor(v string) int {
	n := 0
	for i := 0; i < len(v) && v[i] >= '0' && v[i] <= '9'; i++ {
		n = n*10 + int(v[i]-'0')
	}
	return n
}
