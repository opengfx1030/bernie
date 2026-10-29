package rocm

// venvProbeScript prints one marked JSON line. Imports are isolated (-I) so
// the working directory (for example a vLLM source tree) never shadows
// packages.
const venvProbeScript = `
import json, platform, sys
r = {"python": platform.python_version()}
errs = []
def done():
    if errs:
        r["error"] = "; ".join(errs)
    print("BERNIE_JSON:" + json.dumps(r))
    sys.exit(0)
try:
    import torch
    r["torch"] = torch.__version__
    r["hip"] = getattr(torch.version, "hip", None) or ""
except Exception as e:
    errs.append("import torch: %s" % e)
    done()
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
    import vllm._rocm_C
except Exception as e:
    errs.append("import vllm._rocm_C: %s" % e)
try:
    s = [str(x) for x in torch._C._jit_get_all_schemas()]
    r["rdna_ops"] = sum(1 for x in s if x.startswith("_rocm_C::") and "rdna" in x)
except Exception as e:
    errs.append("schemas: %s" % e)
done()
`
