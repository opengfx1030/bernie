package rocm

// VenvProbeScript prints one marked JSON line. Imports are isolated (-I) so
// the working directory (for example a vLLM source tree) never shadows
// packages. The script also runs the checks of the gfx1030 wiki: the RDNA
// ops of the fork are registered, the libraries of the extension resolve
// and rocBLAS comes from the wheel of the venv.
//
// The evidence carries a small ordered "components" list of the MOST
// IMPORTANT parts of the venv (spec §6.3): python, torch (with the hip
// build), the rocm-sdk wheel, triton, vllm (with the fork's git sha and
// branch when the editable source has a .git, read from files, not by
// running git), flash attention (the package when it is there, else the
// fork's FA-RDNA2 op count), causal-conv1d, transformers, rocBLAS (with
// the wheel library version), RCCL and the host ROCm SDK (for
// comparison). Missing parts are reported as "absent", never omitted.
//
// With BERNIE_VENV_STRICT set the script exits non-zero when a check fails,
// so a caller can use it as a gate. Without it the script always exits 0
// and reports the findings, which is what the probe wants.
const VenvProbeScript = `
import json, os, platform, re, subprocess, sys

r = {"python": platform.python_version()}
errs = []
verr = []
strict = bool(os.environ.get("BERNIE_VENV_STRICT"))

def finish():
    if errs:
        r["error"] = "; ".join(errs)
    r["verify_errors"] = verr
    print("BERNIE_JSON:" + json.dumps(r))
    if strict and (errs or verr):
        sys.exit(1)
    sys.exit(0)

def ldd(path):
    try:
        return subprocess.run(["ldd", path], capture_output=True, text=True).stdout
    except Exception:
        return ""

def pkg_version(names):
    try:
        from importlib.metadata import version, PackageNotFoundError
    except ImportError:
        try:
            from importlib_metadata import version, PackageNotFoundError  # type: ignore
        except ImportError:
            return None
    for n in names:
        try:
            return version(n)
        except PackageNotFoundError:
            continue
        except Exception:
            continue
    return None

def read_text(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return f.read()
    except Exception:
        return None

def host_rocm_version():
    # The host ROCm tree lives at /opt/rocm or /opt/rocm-<ver> or
    # /opt/rocm/core-<ver>. The version is the name of the directory or
    # the contents of .info/version.
    roots = ["/opt/rocm/core-7.14", "/opt/rocm-7.14.0", "/opt/rocm-7.2.0", "/opt/rocm"]
    for root in roots:
        info = os.path.join(root, ".info", "version")
        v = read_text(info)
        if v:
            v = v.strip().splitlines()[0].strip()
            if v:
                return v, root
    for root in roots:
        if os.path.isdir(root) and os.path.isfile(os.path.join(root, "bin", "hipcc")):
            m = re.match(r"/opt/rocm(?:[-/]core)?-(.+)", root)
            if m:
                return m.group(1), root
            return "unknown", root
    return None, None

def git_from_files():
    # The fork is an editable install. The .git of the source sits next
    # to __init__.py: read HEAD and config, do not run git.
    try:
        import vllm
        src = os.path.dirname(getattr(vllm, "__file__", "") or "")
    except Exception:
        return None, None
    if not src:
        return None, None
    cur = src
    while cur != os.path.dirname(cur):
        if os.path.isdir(os.path.join(cur, ".git")):
            head = read_text(os.path.join(cur, ".git", "HEAD"))
            config = read_text(os.path.join(cur, ".git", "config"))
            if head:
                head = head.strip()
                sha = None
                ref = None
                if head.startswith("ref:"):
                    ref = head.split(":", 1)[1].strip()
                    sha_file = os.path.join(cur, ".git", ref)
                    s = read_text(sha_file)
                    if s:
                        sha = s.strip()[:8]
                else:
                    sha = head[:8]
                branch = None
                if config:
                    m = re.search(r"\[branch\s+\"([^\"]+)\"\]", config)
                    if m:
                        branch = m.group(1)
                return sha, branch
            return None, None
        cur = os.path.dirname(cur)
    return None, None

# Components: an ordered list, never omitted when absent (spec §6.3).
components = []
def add(name, version, note=""):
    components.append({"name": name, "version": version or "absent", "note": note or ""})

add("python", platform.python_version(), "")
try:
    import torch
    r["torch"] = torch.__version__
    r["hip"] = getattr(torch.version, "hip", None) or ""
    add("torch", torch.__version__, "hip " + (r["hip"] or "absent"))
except Exception as e:
    errs.append("import torch: %s" % e)
    add("torch", None, "")
    finish()
try:
    r["device_count"] = torch.cuda.device_count()
except Exception as e:
    errs.append("torch.cuda.device_count: %s" % e)
try:
    import vllm
    r["vllm"] = getattr(vllm, "__version__", "") or "unknown"
except Exception as e:
    errs.append("import vllm: %s" % e)
try:
    import vllm._rocm_C as rc
    r["rocm_c"] = getattr(rc, "__file__", "") or ""
except Exception as e:
    errs.append("import vllm._rocm_C: %s" % e)
try:
    s = [str(x) for x in torch._C._jit_get_all_schemas()]
    ops = [x for x in s if x.startswith("_rocm_C::")]
    r["rocm_ops"] = len(ops)
    r["rdna_ops"] = sum(1 for x in ops if "rdna" in x)
    r["all_reduce"] = any("all_reduce" in x for x in ops)
except Exception as e:
    errs.append("schemas: %s" % e)

# rocm-sdk-core / rocm wheel
rocm_sdk_ver = pkg_version(["rocm-sdk-core", "rocm"])
add("rocm-sdk-wheel", rocm_sdk_ver, "wheel" if rocm_sdk_ver else "")

# triton
add("triton", pkg_version(["triton"]), "")

# vllm (with fork git sha and branch from .git files)
sha, branch = git_from_files()
note = ""
if sha or branch:
    parts = []
    if sha:
        parts.append(sha)
    if branch:
        parts.append(branch)
    note = "fork @" + " ".join(parts)
add("vllm", r.get("vllm", None), note)

# flash attention: the package when it is there, else the fork's
# FA-RDNA2 op count.
fa_ver = pkg_version(["flash_attn"])
fa_note = ""
if fa_ver:
    add("flash-attn", fa_ver, "package")
else:
    rdna_fa = sum(1 for x in s if x.startswith("_rocm_C::") and "rdna" in x and ("fa" in x.lower() or "flash" in x.lower() or "FA" in x))
    r["fa_rdna2"] = rdna_fa
    if rdna_fa > 0:
        add("flash-attn", "fork", "FA-RDNA2 (%d ops)" % rdna_fa)
    else:
        add("flash-attn", None, "")

add("causal-conv1d", pkg_version(["causal_conv1d"]), "")
add("transformers", pkg_version(["transformers"]), "")
so = r.get("rocm_c", "")
rocblas_path = None
if so:
    out = ldd(so)
    r["missing_libs"] = [l.strip() for l in out.splitlines() if "not found" in l]
    for line in out.splitlines():
        if "librocblas.so" in line and "=>" in line:
            rocblas_path = line.split("=>", 1)[1].strip().split(" (")[0]
            r["rocblas"] = rocblas_path
    if not r.get("rocblas"):
        hip = os.path.join(os.path.dirname(torch.__file__), "lib", "libtorch_hip.so")
        for line in ldd(hip).splitlines():
            if "librocblas.so" in line and "=>" in line:
                rocblas_path = line.split("=>", 1)[1].strip().split(" (")[0]
                r["rocblas"] = rocblas_path
                break
    r["rocblas_wheel"] = "_rocm_sdk_libraries" in (r.get("rocblas") or "")
# rocBLAS wheel library version: the SONAME of the resolved library.
rocblas_ver = None
if rocblas_path and os.path.isfile(rocblas_path):
    soname_out = subprocess.run(["ldconfig", "-p"], capture_output=True, text=True).stdout
    needle = os.path.basename(rocblas_path)
    for line in soname_out.splitlines():
        if needle in line and "=>" in line:
            tail = line.split("=>", 1)[1].strip().split()
            if len(tail) >= 2 and tail[0] == rocblas_path:
                rocblas_ver = tail[1]
                break
    if not rocblas_ver:
        ldd_v = ldd(rocblas_path)
        for line in ldd_v.splitlines():
            if needle in line and "=>" in line:
                tail = line.split("=>", 1)[1].strip().split()
                if len(tail) >= 1 and tail[0] == rocblas_path:
                    rocblas_ver = tail[1] if len(tail) >= 2 else None
                    break
add("rocBLAS", rocblas_ver, "wheel" if r.get("rocblas_wheel") else "")
# RCCL: torch.cuda.nccl.version() returns (compiled, runtime).
try:
    nccl = torch.cuda.nccl.version()
    nccl_ver = "%s/%s" % nccl if isinstance(nccl, tuple) and len(nccl) == 2 else str(nccl)
except Exception:
    nccl_ver = None
add("RCCL", nccl_ver, "")

# Host ROCm SDK (for comparison).
hv, hroot = host_rocm_version()
add("rocm-sdk-host", hv, hroot or "")

r["components"] = components

if r.get("rdna_ops", 0) <= 10:
    verr.append("only %d _rocm_C rdna ops are registered (need more than 10)" % r.get("rdna_ops", 0))
if not r.get("all_reduce"):
    verr.append("_rocm_C::all_reduce is not registered")
if r.get("missing_libs"):
    verr.append("missing libraries: " + "; ".join(r["missing_libs"]))
if not r.get("rocblas"):
    verr.append("torch does not resolve a rocBLAS")
elif not r.get("rocblas_wheel"):
    verr.append("rocBLAS comes from " + r["rocblas"] + ", not the wheel (_rocm_sdk_libraries)")
finish()
`
