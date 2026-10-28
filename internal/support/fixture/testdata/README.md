# Fixtures

Each `<name>.tar.gz` is a root tree for `fixture.Root(t, "<name>")`.

| Fixture | Origin | GPUs |
|---|---|---|
| `par1-cs25` | Captured with `scripts/capture-fixture.sh` on a build server. The PCI `resource` files (the largest BAR) were added later from the same host. | 2× Radeon PRO W7800 48GB (gfx1100), 8× Radeon PRO V620 (gfx1030) |
| `p2p-degraded-synthetic` | **Synthetic.** Derived from `par1-cs25`: the kernel command line lost `iommu=pt`, `pcie_acs_override` and the `pci=` parameter, KFD node 5 lost its peer links in both directions, GPU `0000:47:00.0` runs at gen3 x8 while its slot offers gen4 x16, and its largest BAR was reduced to 256 MiB. Every other value is the captured one. | 2× Radeon PRO W7800 48GB (gfx1100), 8× Radeon PRO V620 (gfx1030) |
| `w7800-single` | Derived from `par1-cs25`: the CPU node and the first W7800 node with their sysfs tree, nothing else. The P2P link directories and counters are dropped. All values are the captured ones. | 1× Radeon PRO W7800 48GB (gfx1100) |
| `v620-ecc` | Derived from a `scripts/capture-fixture.sh` run on the same host: the CPU node and one V620 (KFD node 3, `0000:47:00.0`) with its sysfs tree, the `ras` directory, the hwmon node and the `pp_dpm_sclk`/`pp_dpm_mclk` clock files. The P2P link directories are dropped. All values are the captured ones. | 1× Radeon PRO V620 (gfx1030) |
| `ecc-synthetic` | **Synthetic.** The captured `v620-ecc` tree with ECC errors: the RAS counters are written by hand (`ue 2`, `ce 5`, `de 1`). Every other value is the captured one. | 1× Radeon PRO V620 (gfx1030) |
| `rx7900xtx-synthetic` | **Synthetic.** The captured W7800 KFD node (gfx1100) with the device id, SIMD count and PCI location of an RX 7900 XTX (96 CU). Distro, kernel, CPU, RAM, VRAM size (24 GiB), power cap (355 W) and the PCI topology are written by hand. | 1× RX 7900 XTX (gfx1100) |
| `rx6800xt-synthetic` | **Synthetic.** The captured V620 KFD node (gfx1030, also 72 CU) with the device id of an RX 6800 XT. Distro, kernel, CPU, RAM, VRAM size (16 GiB), power cap (300 W) and the PCI topology are written by hand. | 1× RX 6800 XT (gfx1030) |

A synthetic fixture carries a `SYNTHETIC` file at its root. Neither desktop
fixture has a DRM `product_name` (consumer cards do not publish one), so the
GPU name comes from the device tables.
