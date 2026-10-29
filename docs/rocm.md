# ROCm on the host

This page says which ROCm bernie installs, on which distributions, and
which commands it prints. It is the reference for the `rocm.runtime` step of
`bernie setup plan` (spec [§6](spec.md#6-setup)).

## The two repository shapes

AMD publishes ROCm in two shapes.

The **multi-arch repository** carries one package per GPU architecture, for
example `amdrocm-core-sdk7.14-gfx1030`. It serves ROCm 7.13 and later. Its
address is `https://repo.amd.com/rocm/packages-multi-arch/<dir>/`, and the
public key is
`https://repo.amd.com/rocm/packages-multi-arch/gpg/rocm.gpg` (ASCII-armored,
fingerprint `D0F004A0025A1145C7807FCD0701EAC4D5E02107`, AMD MLSE DevOps).

The **per-release repository** carries the packages of one ROCm release. It
serves ROCm 7.2 and earlier. Its address is `https://repo.radeon.com/rocm/`
and its public key is `https://repo.radeon.com/rocm/rocm.gpg.key`.

## The distributions

| Distribution | Multi-arch directory | Per-release directory |
|---|---|---|
| Fedora 42, 43 | `rhel10/x86_64` | `el10/7.2/main` |
| Fedora 39 to 41 | `rhel9/x86_64` | `el9/7.2/main` |
| Fedora 8 to 38 | `rhel8/x86_64` | `el8/7.2/main` |
| RHEL 10, Rocky 10, AlmaLinux 10 | `rhel10/x86_64` | `el10/7.2/main` |
| RHEL 9, Rocky 9, AlmaLinux 9 | `rhel9/x86_64` | `el9/7.2/main` |
| RHEL 8, Rocky 8, AlmaLinux 8 | `rhel8/x86_64` | `el8/7.2/main` |
| Ubuntu 22.04 | `ubuntu2204` (suite `stable`) | `apt/7.2` suite `jammy` |
| Ubuntu 24.04 | `ubuntu2404` (suite `stable`) | `apt/7.2` suite `noble` |
| Ubuntu 26.04 | `ubuntu2604` (suite `stable`) | none |
| Debian 12 | `debian12` (suite `stable`) | `apt/7.2` suite `bookworm` |
| Debian 13 | `debian13` (suite `stable`) | `apt/7.2` suite `trixie` |

Fedora 42 and later follow the RHEL 10 codebase, Fedora 39 to 41 the RHEL 9
one. The mapping of Fedora below 39 is an inference from the RHEL cycle.

The per-release apt repository has no suite for Ubuntu 26.04 and no suite for
Debian at all: on those distributions ROCm 7.2 cannot be installed from AMD.
AMD publishes no repository for Arch, so `bernie setup plan` prints the AMD
install guide instead of a command there.

## The packages

ROCm 7.14 names one package set per GPU architecture. For every code target
of the host (the gfx target of the card, or the target of its
`hsa_override`, spec [§4.4](spec.md#44-device-tables)) bernie installs:

```
amdrocm-core-sdk7.14-<gfx>   hipcc, the ROCm headers, the developer tools
amdrocm-core7.14-<gfx>      the runtime: rocminfo, amd-smi, the HIP runtime
amdrocm-blas7.14-<gfx>      rocBLAS
```

`amdrocm-core7.14-<gfx>` pulls in `amdrocm-base7.14`, `amdrocm-llvm7.14` and
`amdrocm-amdsmi7.14`. `amdrocm-core-sdk7.14-<gfx>` pulls in
`amdrocm-developer-tools7.14`. The three names above are the ones the
build server `par1-cs25` carries.

ROCm 7.2 has no per-architecture packages. bernie installs:

```
rocm-core rocm-hip-sdk hipcc rocminfo rocm-smi-lib amd-smi-lib rocblas
rocm-device-libs rocm-developer-tools
```

## The versions

`bernie setup plan` installs ROCm 7.14 by default. `--rocm 7.2` selects the
older line. ROCm 7.2.1 to 7.13 have the RCCL tensor-parallel deadlock, so
the two lines that are safe are the two bernie offers.

The step is done when the host has a ROCm tree with `hipcc`. Three layouts
count: the versioned tree (`/opt/rocm-7.2.0`), the TheRock tree
(`/opt/rocm/core-7.14`) and the plain tree (`/opt/rocm`). `bernie doctor`
reads the version from `.info/version`.

## The loader

The ROCm 7.14 packages ship `/etc/ld.so.conf.d/10-rocm-7.14.conf`. When the
tree is there and the file is not, bernie prints the command that writes it.
ROCm 7.2 has no such file, so the command is printed there too.

## The venv

The venv is Python 3.12. The wheels come from
`https://repo.amd.com/rocm/whl-multi-arch/`:

```
torch==2.12.0+rocm7.14.0
torchvision==0.27.0+rocm7.14.0
torchaudio==2.11.0+rocm7.14.0
triton==3.7.1
rocm-sdk-device-<gfx>==7.14.0
amd-torch-device-<gfx>==2.12.0+rocm7.14.0
```

Triton stays at 3.7.1: Triton 3.9 dropped RDNA1 and RDNA2.

The venv holds the build tools of the fork as well:

```
setuptools setuptools-scm setuptools-rust cmake ninja wheel jinja2 packaging
```

The fork is built with `--no-build-isolation`, so the build backend and
the build tools have to be in the venv: without them the build stops with
`No module named setuptools_scm`. The bin directory of the venv is first
on `PATH` while a build runs: `cmake` and `ninja` live there.

At run time the loader must find the wheel libraries before the system
ones. The order is:

```
<site-packages>/_rocm_sdk_libraries/lib
<site-packages>/_rocm_sdk_core/lib
<site-packages>/_rocm_sdk_core/lib/host-math/lib
<site-packages>/_rocm_sdk_core/lib/rocm_sysdeps/lib
<site-packages>/_rocm_sdk_core/lib/core/lib
<site-packages>/torch/lib
```

`_rocm_sdk_libraries` holds the wheel rocBLAS. It must load before any
system rocBLAS: the TunableOp profiles are bound to the rocBLAS build
(its sha256), and a different one makes the profile invalid.

## The fork

The fork is `https://github.com/opengfx1030/vllm-rdna.git` on the branch
`rdna_extras` (upstream vLLM 0.31.0 with the RDNA port). The build needs:

```
VLLM_TARGET_DEVICE=rocm
VLLM_USE_PRECOMPILED=0
ENABLE_CK=0
FLASH_ATTENTION_TRITON_AMD_ENABLE=TRUE
PYTORCH_ROCM_ARCH=<gfx>;<gfx>
ROCM_HOME=<tree with bin/hipcc>
ROCM_PATH=<tree with bin/hipcc>
HIP_PATH=<tree with bin/hipcc>
CMAKE_HIP_COMPILER=<tree with bin/hipcc>/bin/hipcc
HIP_ROOT_DIR=<tree with bin/hipcc>
CMAKE_HIP_COMPILER_ROCM_ROOT=<tree with bin/hipcc>
CMAKE_BUILD_TYPE=RelWithDebInfo
VLLM_PYTHON_EXECUTABLE=<venv>/bin/python
SETUPTOOLS_SCM_PRETEND_VERSION=0.31.0+rdna
MAX_JOBS=<cores, 16 at most>
```

`SETUPTOOLS_SCM_PRETEND_VERSION` is needed because the branch carries no
tag: without it setuptools-scm cannot name the package. `HIP_ROOT_DIR` and
`CMAKE_HIP_COMPILER_ROCM_ROOT` keep cmake from falling back to
`/opt/rocm`, where the wrong headers win.

The install takes no dependencies (`--no-deps`): the runtime ones are
installed from `requirements/common.txt` of the fork, the build ones are
in the venv. The ROCm-only extras of `requirements/rocm.txt` are not
installed: they are not needed to serve and some of them have no wheel.

## The check

The `venv.verify` step is the postcondition of the build. It runs the
checks of the gfx1030 wiki in the venv:

- `torch` and `vllm` import.
- The fork registered more than ten RDNA ops in the `_rocm_C` namespace
  and one of them is an `all_reduce`. The ops are counted with
  `torch._C._jit_get_all_schemas()`: `dir(torch.ops._rocm_C)` shows
  nothing, even when the ops are registered.
- `ldd` of the extension `_rocm_C.abi3.so` finds every library it needs.
  Without the wheel libraries in `LD_LIBRARY_PATH` the extension loads
  but registers no op.
- rocBLAS comes from the wheel (`_rocm_sdk_libraries`), not from the
  system.

The `doctor` reports the same findings under the `python` category.

## What bernie runs and what it prints

bernie never runs a root command. The `rocm.runtime` step is printed:

```
sudo rpm --import https://repo.amd.com/rocm/packages-multi-arch/gpg/rocm.gpg
printf '[rocm]\nname=AMD ROCm 7.14 multi-arch (rhel10)\nbaseurl=https://repo.amd.com/rocm/packages-multi-arch/rhel10/x86_64\nenabled=1\ngpgcheck=1\npriority=50\ngpgkey=https://repo.amd.com/rocm/packages-multi-arch/gpg/rocm.gpg\n' | sudo tee /etc/yum.repos.d/rocm.repo
sudo dnf install -y amdrocm-core-sdk7.14-gfx1030 amdrocm-core7.14-gfx1030 amdrocm-blas7.14-gfx1030
```

On Ubuntu and Debian the key goes to a keyring and the source line is
written to `/etc/apt/sources.list.d/rocm.list`:

```
sudo mkdir -p -m 0755 /etc/apt/keyrings && curl -fsSL https://repo.amd.com/rocm/packages-multi-arch/gpg/rocm.gpg | gpg --dearmor | sudo tee /etc/apt/keyrings/rocm.gpg >/dev/null
echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.amd.com/rocm/packages-multi-arch/ubuntu2404 stable main" | sudo tee /etc/apt/sources.list.d/rocm.list
sudo apt update && sudo apt install -y amdrocm-core-sdk7.14-gfx1030 amdrocm-core7.14-gfx1030 amdrocm-blas7.14-gfx1030
```

bernie runs the steps that need no root itself: the venv, the wheels, the
fork build and the model download.

## The driver

The in-tree `amdgpu` driver of the kernel is enough for a card the kernel
knows. `amdgpu-dkms` is only needed when the kernel of the distribution is
too old for the GPU. `bernie doctor` reports the driver state; it does not
install a kernel module.

## Sources

- <https://repo.amd.com/rocm/packages-multi-arch/> — the directory list of
  the multi-arch repository.
- <https://repo.amd.com/rocm/packages-multi-arch/rhel10/x86_64/repodata/repomd.xml>
  — the package index of the RHEL 10 repository.
- <https://repo.amd.com/rocm/packages-multi-arch/ubuntu2404/dists/stable/main/binary-amd64/Packages.gz>
  — the package index of the Ubuntu 24.04 repository.
- <https://repo.radeon.com/rocm/el10/7.2/main/repodata/repomd.xml> — the
  package index of the ROCm 7.2 repository for RHEL 10.
- <https://repo.radeon.com/rocm/apt/7.2/dists/> — the apt suites of the
  ROCm 7.2 repository.
- <https://rocm.docs.amd.com/projects/install-on-linux/en/latest/> — the AMD
  install guide.
- <https://blivioniag.github.io/gfx1030-wiki/vllm/host-venv.html> — the host
  venv recipe and the library path order.
- <https://blivioniag.github.io/gfx1030-wiki/llama-cpp/building.html> — the
  ROCm 7.2 repository file and the Fedora install commands.
