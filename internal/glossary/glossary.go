// Package glossary holds the words that bernie uses that a reader who has
// never run vLLM may not know, with a plain meaning for each. It is data:
// it holds no logic and imports nothing. The page shows the words of the
// tab it is on, so the reader can ask what a word means.
package glossary

// Word is one word and what it means, in one sentence.
type Word struct {
	Term    string `json:"term"`
	Meaning string `json:"meaning"`
}

// Words are the words of bernie, in the order a reader meets them.
var Words = []Word{
	{"AMD", "The company that makes the GPUs bernie serves models on."},
	{"ROCm", "The AMD software that lets a program use the GPU."},
	{"HIP", "The number a program uses to name a GPU (0 is the first one)."},
	{"KFD", "The part of the Linux kernel that lets a program open a GPU."},
	{"amdgpu", "The Linux driver of the AMD GPUs. It must be loaded before bernie sees a card."},
	{"gfx1030", "The name of a generation of AMD GPU (the Radeon PRO V620 is one)."},
	{"gfx1100", "The name of a generation of AMD GPU (the Radeon PRO W7800 is one)."},
	{"hipcc", "The compiler of ROCm that builds the code for the GPU."},
	{"rocBLAS", "The maths library of ROCm: the linear algebra a model needs."},
	{"rocm-smi", "The tool that reads the state of the GPUs (temperature, power, memory)."},
	{"TunableOp", "A table of tuned settings for the maths of the GPU."},
	{"VRAM", "The memory of the GPU. The model has to fit in it."},
	{"KV cache", "The memory the server keeps for the text it has read so far."},
	{"Prompt reading", "The speed at which the server reads the words you sent. It matters most for long prompts."},
	{"Active requests", "The requests the server is answering right now. Another request can wait behind them."},
	{"First token", "The wait before the answer starts. It is called the time to the first token."},
	{"Prefix cache hits", "The share of prompt work the server could reuse from an earlier request."},
	{"Draft accepted", "The share of speculative draft tokens the model kept. Higher means the speculation saves work."},
	{"vLLM", "The server bernie starts: it loads a model and answers requests."},
	{"venv", "A Python environment of its own, with its own packages."},
	{"AWQ", "A way to keep a model small enough to fit in the memory of the GPU."},
	{"INT4", "A model that keeps four bits of every weight."},
	{"TP", "Tensor parallelism: one model split over several GPUs."},
	{"EP", "Expert parallelism: the parts of a mixture-of-experts model on different GPUs."},
	{"MTP", "Multi-token prediction: the model writes several tokens at once."},
	{"PLE", "A part of a mixture-of-experts model that only some tokens use."},
}

// Terms are the words of the glossary.
func Terms() []string {
	out := make([]string, 0, len(Words))
	for _, w := range Words {
		out = append(out, w.Term)
	}
	return out
}

// Meaning returns what a word means, or "" when the word is not in the
// glossary.
func Meaning(term string) string {
	for _, w := range Words {
		if w.Term == term {
			return w.Meaning
		}
	}
	return ""
}
