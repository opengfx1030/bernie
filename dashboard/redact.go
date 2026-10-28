package dashboard

import (
	"encoding/json"
	"strings"
)

const redacted = "<redacted>"

// secretName reports whether an environment key or flag name names a secret:
// its last `_`/`-`/`.` segment ends with TOKEN, KEY, SECRET, PASSWORD or
// PASSWD (HF_TOKEN, OPENAI_API_KEY, --api-key, --hf-token, DB_PASSWORD),
// case-insensitive. Names that only mention a word are kept: TOKENIZERS_PARALLELISM,
// --max-num-batched-tokens, --long-prefill-token-threshold, --ssl-keyfile.
func secretName(name string) bool {
	u := strings.ToUpper(strings.TrimLeft(name, "-"))
	if i := strings.LastIndexAny(u, "_-."); i >= 0 {
		u = u[i+1:]
	}
	for _, w := range [...]string{"TOKEN", "KEY", "SECRET", "PASSWORD", "PASSWD"} {
		if strings.HasSuffix(u, w) {
			return true
		}
	}
	return false
}

// redactJSON returns v as generic JSON with secret values replaced:
//   - object members whose name is a secret and whose value is a string;
//   - "KEY=VALUE" strings whose KEY is a secret (env lists, `-e K=V` argv);
//   - "--flag=VALUE" and "--flag VALUE" in string lists when the flag is a secret.
//
// Plans and run records carry the operator's environment; the dashboard may be
// reachable from the network, so it never returns secret values.
func redactJSON(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var g any
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	return redactValue(g), nil
}

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			if _, isStr := x.(string); isStr && secretName(k) {
				t[k] = redacted
				continue
			}
			t[k] = redactValue(x)
		}
		return t
	case []any:
		prevSecretFlag := false
		for i, x := range t {
			s, ok := x.(string)
			if !ok {
				t[i] = redactValue(x)
				prevSecretFlag = false
				continue
			}
			switch {
			case prevSecretFlag && !strings.HasPrefix(s, "-"):
				t[i] = redacted
				prevSecretFlag = false
				continue
			case strings.HasPrefix(s, "--"):
				name, _, hasVal := strings.Cut(s, "=")
				prevSecretFlag = !hasVal && secretName(name)
			default:
				prevSecretFlag = false
			}
			t[i] = redactString(s)
		}
		return t
	case string:
		return redactString(t)
	}
	return v
}

// redactString handles one "KEY=VALUE" or "--flag=VALUE" string.
func redactString(s string) string {
	k, _, ok := strings.Cut(s, "=")
	if !ok || k == "" || strings.ContainsAny(k, " \t") || !secretName(k) {
		return s
	}
	return k + "=" + redacted
}
