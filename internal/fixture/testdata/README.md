# Fixtures

Each `<name>.tar.gz` is a root tree for `fixture.Root(t, "<name>")`.

| Fixture | Origin | GPUs |
|---|---|---|
| `par1-cs25` | Captured with `scripts/capture-fixture.sh` on a build server. | 2× Radeon PRO W7800 48GB (gfx1100), 8× Radeon PRO V620 (gfx1030) |
| `w7800-single` | Derived from `par1-cs25`: the CPU node and the first W7800 node with their sysfs tree, nothing else. The P2P link directories and counters are dropped. All values are the captured ones. | 1× Radeon PRO W7800 48GB (gfx1100) |
| `rx7900xtx-synthetic` | **Synthetic.** The captured W7800 KFD node (gfx1100) with the device id, SIMD count and PCI location of an RX 7900 XTX (96 CU). Distro, kernel, CPU, RAM, VRAM size (24 GiB), power cap (355 W) and the PCI topology are written by hand. | 1× RX 7900 XTX (gfx1100) |
| `rx6800xt-synthetic` | **Synthetic.** The captured V620 KFD node (gfx1030, also 72 CU) with the device id of an RX 6800 XT. Distro, kernel, CPU, RAM, VRAM size (16 GiB), power cap (300 W) and the PCI topology are written by hand. | 1× RX 6800 XT (gfx1030) |

A synthetic fixture carries a `SYNTHETIC` file at its root. Neither desktop
fixture has a DRM `product_name` (consumer cards do not publish one), so the
GPU name comes from the device tables.
