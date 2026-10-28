package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	plan := map[string]any{
		"name": "x",
		"env": map[string]string{"HF_TOKEN": "hf_abc", "OPENAI_API_KEY": "sk-1", "VLLM_RDNA_AR": "1", "MAX_NUM_BATCHED_TOKENS": "8192",
			"TOKENIZERS_PARALLELISM": "false"},
		"exec": map[string]any{
			"argv": []string{"podman", "run", "-e", "HF_TOKEN=hf_abc", "-e", "DB_PASSWORD=pw", "-e", "VLLM_RDNA_AR=1",
				"img", "serve", "m", "--api-key", "sk-2", "--max-num-batched-tokens", "8192", "--hf-token=hf_x", "--enable-prefix-caching", "--port", "8000",
				"--long-prefill-token-threshold", "512", "--ssl-keyfile", "/k.pem"},
			"env": []string{"AWS_SECRET_ACCESS_KEY=a", "PATH=/usr/bin"},
		},
	}
	g, err := JSON(plan)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(g)
	out := string(b)
	for _, leak := range []string{"hf_abc", "sk-1", "sk-2", "hf_x", `"pw"`, "=pw", "AWS_SECRET_ACCESS_KEY=a"} {
		if strings.Contains(out, leak) {
			t.Errorf("secret %q leaked: %s", leak, out)
		}
	}
	for _, keep := range []string{`"VLLM_RDNA_AR":"1"`, `"MAX_NUM_BATCHED_TOKENS":"8192"`, `"TOKENIZERS_PARALLELISM":"false"`, `"--max-num-batched-tokens","8192"`,
		`"--enable-prefix-caching","--port","8000"`, `"PATH=/usr/bin"`, `"VLLM_RDNA_AR=1"`, `"--long-prefill-token-threshold","512"`, `"--ssl-keyfile","/k.pem"`} {
		if !strings.Contains(out, keep) {
			t.Errorf("non-secret %q was changed: %s", keep, out)
		}
	}
}

func TestText(t *testing.T) {
	in := strings.Join([]string{
		"INFO loading model with max_num_batched_tokens=8192 and TOKENIZERS_PARALLELISM=false",
		"env HF_TOKEN=hf_abcdefghijklmnopqrstuvwx OPENAI_API_KEY: sk-abcdefghijklmnopqrstuv",
		`{"api_key": "plainvalue", "port": 8000}`,
		"Authorization: Bearer abcdef0123456789abcdef",
		"download hf_ZZZZZZZZZZZZZZZZZZZZZZ failed",
	}, "\n")
	out := Text(in)
	for _, leak := range []string{"hf_abcdef", "hf_ZZZZ", "sk-abcdef", "plainvalue", "abcdef0123456789"} {
		if strings.Contains(out, leak) {
			t.Errorf("secret %q leaked: %s", leak, out)
		}
	}
	for _, keep := range []string{"max_num_batched_tokens=8192", "TOKENIZERS_PARALLELISM=false", `"port": 8000`} {
		if !strings.Contains(out, keep) {
			t.Errorf("non-secret %q was changed: %s", keep, out)
		}
	}
}
