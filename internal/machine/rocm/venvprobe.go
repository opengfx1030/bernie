package rocm

// VenvProbeScript prints one marked JSON line. Imports are isolated (-I) so
// the working directory (for example a vLLM source tree) never shadows
// packages. The script also runs the checks of the gfx1030 wiki: the RDNA
// ops of the fork are registered, the libraries of the extension resolve
// and rocBLAS comes from the wheel of the venv.
//
// With BERNIE_VENV_STRICT set the script exits non-zero when a check fails,
// so a caller can use it as a gate. Without it the script always exits 0
// and reports the findings, which is what the probe wants.
const VenvProbeScript = `
import json, os, platform, subprocess, sys

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

try:
    import torch
    r["torch"] = torch.__version__
    r["hip"] = getattr(torch.version, "hip", None) or ""
except Exception as e:
    errs.append("import torch: %s" % e)
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
so = r.get("rocm_c", "")
if so:
    out = ldd(so)
    r["missing_libs"] = [l.strip() for l in out.splitlines() if "not found" in l]
    for line in out.splitlines():
        if "librocblas.so" in line and "=>" in line:
            r["rocblas"] = line.split("=>", 1)[1].strip().split(" (")[0]
    if not r.get("rocblas"):
        hip = os.path.join(os.path.dirname(torch.__file__), "lib", "libtorch_hip.so")
        for line in ldd(hip).splitlines():
            if "librocblas.so" in line and "=>" in line:
                r["rocblas"] = line.split("=>", 1)[1].strip().split(" (")[0]
    r["rocblas_wheel"] = "_rocm_sdk_libraries" in r.get("rocblas", "")
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
