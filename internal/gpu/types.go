// Package gpu enumerates AMD GPUs from the KFD topology and DRM/PCI sysfs,
// and samples telemetry. It reads only files under a root path ("/" in
// production, a fixture tree in tests).
package gpu

// Link is the negotiated PCIe link of a GPU.
type Link struct {
	Gen   int `json:"gen"`
	Width int `json:"width"`
}

// GPU is one KFD GPU node. HIPIndex is its position among KFD GPU nodes,
// which is the index HIP_VISIBLE_DEVICES uses.
type GPU struct {
	HIPIndex         int     `json:"hip_index"`
	KFDNode          int     `json:"kfd_node"`
	Name             string  `json:"name"`
	GFX              string  `json:"gfx"`
	GFXTargetVersion int     `json:"gfx_target_version"`
	PCI              string  `json:"pci"`
	DeviceID         string  `json:"device_id"`
	VRAMBytes        uint64  `json:"vram_bytes"`
	RenderMinor      int     `json:"render_minor"`
	NUMANode         int     `json:"numa_node"`
	Link             Link    `json:"link"`
	PowerCapW        float64 `json:"power_cap_w"`
	SysfsPath        string  `json:"sysfs_path"`
}

// Sample is one telemetry reading of a GPU.
type Sample struct {
	HIPIndex  int     `json:"hip_index"`
	BusyPct   float64 `json:"busy_pct"`
	TempC     float64 `json:"temp_c"`
	PowerW    float64 `json:"power_w"`
	PowerCapW float64 `json:"power_cap_w"`
	VRAMUsed  uint64  `json:"vram_used"`
	VRAMTotal uint64  `json:"vram_total"`
}
