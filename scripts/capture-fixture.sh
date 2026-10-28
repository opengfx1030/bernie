#!/usr/bin/env bash
# capture-fixture.sh NAME [OUTDIR]
#
# Copies the sysfs/procfs subset that bernie reads into NAME.tar.gz (in
# OUTDIR, default: the current directory). The archive is a root tree:
#   NAME/sys/..., NAME/proc/..., NAME/etc/os-release
# Symlinks keep their relative targets, so the tree resolves like the real
# /sys. Serial numbers (unique_id) and user data are not copied.
#
# Read-only on the host. Run as a normal user. Never writes outside OUTDIR.
set -euo pipefail

name=${1:?usage: capture-fixture.sh NAME [OUTDIR]}
outdir=${2:-$PWD}
mkdir -p "$outdir"
stage=$(mktemp -d "$outdir/.bernie-fixture.XXXXXX")
trap 'rm -rf "$stage"' EXIT
dst="$stage/$name"
mkdir -p "$dst"

# copy_file SRC [FILTER]: copy the content of a regular file (sysfs-safe, no
# size assumptions). SRC must be a real path (no symlink components).
copy_file() {
	local src=$1
	[ -f "$src" ] && [ -r "$src" ] || return 0
	mkdir -p "$dst$(dirname "$src")"
	cat "$src" >"$dst$src" 2>/dev/null || rm -f "$dst$src"
}

# link_chain PATH: recreate every symlink met while resolving PATH, at its
# real location, and print the fully resolved path.
link_chain() {
	local path=$1 cur="" comp
	local IFS=/
	for comp in ${path#/}; do
		cur="$cur/$comp"
		if [ -L "$cur" ]; then
			mkdir -p "$dst$(dirname "$cur")"
			ln -sfn "$(readlink "$cur")" "$dst$cur"
			cur=$(readlink -f "$cur")
		fi
	done
	printf '%s\n' "$cur"
}

# KFD topology.
kfd=$(link_chain /sys/class/kfd/kfd)
for node in "$kfd"/topology/nodes/*; do
	[ -d "$node" ] || continue
	mkdir -p "$dst$node"
	grep -v '^unique_id ' "$node/properties" >"$dst$node/properties" || true
	copy_file "$node/name"
	copy_file "$node/gpu_id"
	for l in "$node"/io_links/* "$node"/p2p_links/*; do
		[ -d "$l" ] && copy_file "$l/properties"
	done
done

# GPU devices: DRM render node -> PCI device, its hwmon and every bridge above.
dev_files="vendor device subsystem_vendor subsystem_device class revision
mem_info_vram_total mem_info_vram_used mem_info_vis_vram_total
current_link_speed current_link_width max_link_speed max_link_width numa_node
product_name product_number gpu_busy_percent power_dpm_force_performance_level
resource
pp_dpm_sclk pp_dpm_mclk"
hwmon_files="name power1_cap power1_cap_min power1_cap_max power1_cap_default
power1_average power1_input power1_label temp1_input temp1_label temp1_crit
temp2_input temp2_label temp2_crit temp3_input temp3_label temp3_crit"
ras_files="umc_err_count features event_state version schema"

for node in "$kfd"/topology/nodes/*; do
	props="$node/properties"
	simd=$(awk '$1=="simd_count"{print $2}' "$props")
	minor=$(awk '$1=="drm_render_minor"{print $2}' "$props")
	[ "${simd:-0}" -gt 0 ] && [ -n "$minor" ] || continue
	link_chain "/sys/class/drm/renderD$minor" >/dev/null
	dev=$(link_chain "/sys/class/drm/renderD$minor/device")
	for f in $dev_files; do copy_file "$dev/$f"; done
	for f in $ras_files; do copy_file "$dev/ras/$f"; done
	for h in "$dev"/hwmon/hwmon*; do
		[ -d "$h" ] || continue
		for f in $hwmon_files; do copy_file "$h/$f"; done
	done
	bdf=$(basename "$dev")
	link_chain "/sys/bus/pci/devices/$bdf" >/dev/null
	parent=$(dirname "$dev")
	while [ -f "$parent/class" ]; do
		for f in vendor device class; do copy_file "$parent/$f"; done
		link_chain "/sys/bus/pci/devices/$(basename "$parent")" >/dev/null
		parent=$(dirname "$parent")
	done
done

# OS and kernel.
copy_file "$(link_chain /etc/os-release)"
for f in /proc/cmdline /proc/meminfo /proc/sys/kernel/osrelease \
	/proc/sys/kernel/hostname /sys/fs/selinux/enforce /sys/module/amdgpu/initstate \
	/sys/module/amdgpu/parameters/ras_enable \
	/sys/class/dmi/id/sys_vendor /sys/class/dmi/id/product_name /sys/hypervisor/type; do
	[ -e "$f" ] && copy_file "$(link_chain "$f")"
done
# cpuinfo without the long flag lists (keep the hypervisor marker).
mkdir -p "$dst/proc"
awk '/^flags/ { if ($0 ~ / hypervisor( |$)/) print "flags\t\t: hypervisor"; next }
     /^(bugs|vmx flags|power management)/ { next } { print }' /proc/cpuinfo >"$dst/proc/cpuinfo"
grep '^amdgpu ' /proc/modules >"$dst/proc/modules" 2>/dev/null || true

tar -czf "$outdir/$name.tar.gz" -C "$stage" "$name"
echo "wrote $outdir/$name.tar.gz" >&2
