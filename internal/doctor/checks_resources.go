package doctor

import (
	"fmt"
)

func resourceChecks(in inputs) []Check {
	const cat = "resources"
	var out []Check
	ram := in.host.RAMBytes
	switch {
	case ram == 0:
		out = append(out, check(cat, "ram", Skip, "Host RAM", "unknown"))
	case ram < 32*gib:
		out = append(out, check(cat, "ram", Warn, "Host RAM", fmt.Sprintf("%.0f GiB: below 32 GiB, model loading may swap", float64(ram)/gib)))
	default:
		out = append(out, check(cat, "ram", OK, "Host RAM", fmt.Sprintf("%.0f GiB", float64(ram)/gib)))
	}
	switch free := in.hfFree; {
	case free == 0:
		out = append(out, check(cat, "disk", Skip, "Free disk in the Hugging Face cache", in.hfCache+": unknown"))
	case free < 20*gib:
		out = append(out, check(cat, "disk", Fail, "Free disk in the Hugging Face cache", fmt.Sprintf("%s: %.0f GiB free", in.hfCache, float64(free)/gib)).
			with("Free space or set hf_home in the config (rdnactl config set hf_home DIR).", ""))
	case free < 100*gib:
		out = append(out, check(cat, "disk", Warn, "Free disk in the Hugging Face cache", fmt.Sprintf("%s: %.0f GiB free (large models need 100+ GiB)", in.hfCache, float64(free)/gib)).
			with("Free space or set hf_home in the config (rdnactl config set hf_home DIR).", ""))
	default:
		out = append(out, check(cat, "disk", OK, "Free disk in the Hugging Face cache", fmt.Sprintf("%s: %.0f GiB free", in.hfCache, float64(free)/gib)))
	}
	switch shm := in.host.ShmBytes; {
	case shm == 0:
		out = append(out, check(cat, "shm", Skip, "/dev/shm size", "unknown"))
	case shm < 8*gib:
		out = append(out, check(cat, "shm", Warn, "/dev/shm size", fmt.Sprintf("%.1f GiB: tensor parallel in host mode needs 8+ GiB", float64(shm)/gib)).
			with("sudo mount -o remount,size=16G /dev/shm", docRunning))
	default:
		out = append(out, check(cat, "shm", OK, "/dev/shm size", fmt.Sprintf("%.0f GiB", float64(shm)/gib)))
	}
	return out
}
