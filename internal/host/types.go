// Package host probes the operating system: distro, package manager, CPU,
// RAM, user groups, kernel command line and container engines.
package host

// OSRelease is the subset of /etc/os-release rdnactl uses.
type OSRelease struct {
	ID        string `json:"id"`
	IDLike    string `json:"id_like"`
	VersionID string `json:"version_id"`
	Pretty    string `json:"pretty"`
}

// CPU summarizes /proc/cpuinfo.
type CPU struct {
	Model   string `json:"model"`
	Vendor  string `json:"vendor"`
	Sockets int    `json:"sockets"`
	Cores   int    `json:"cores"`
	Threads int    `json:"threads"`
}

// User is the invoking user.
type User struct {
	Name   string   `json:"name"`
	UID    int      `json:"uid"`
	Home   string   `json:"home"`
	Groups []string `json:"groups"`
}

// Engine is a container engine found on PATH.
type Engine struct {
	Name     string `json:"name"` // docker | podman
	Path     string `json:"path"`
	Version  string `json:"version"`
	Rootless bool   `json:"rootless"`
	Usable   bool   `json:"usable"`  // `<engine> info` succeeded for this user
	Compose  string `json:"compose"` // compose command, e.g. "docker compose", "podman compose", "podman-compose", ""
}

// Info is the full host probe.
type Info struct {
	Hostname       string    `json:"hostname"`
	OS             OSRelease `json:"os"`
	Kernel         string    `json:"kernel"`
	PkgManager     string    `json:"pkg_manager"` // dnf|apt|pacman|zypper|""
	CPU            CPU       `json:"cpu"`
	RAMBytes       uint64    `json:"ram_bytes"`
	Virtualization string    `json:"virtualization"` // none|kvm|vmware|...|""
	User           User      `json:"user"`
	Engines        []Engine  `json:"engines"`
	SELinux        string    `json:"selinux"`
	Cmdline        string    `json:"cmdline"`
	ShmBytes       uint64    `json:"shm_bytes"`
}
