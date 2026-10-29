// Package redact hides secret values in documents and log text before they
// leave the process: dashboard responses, MCP tool results and audit records.
package redact

import (
	"encoding/json"
	"regexp"
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

// JSON returns v as generic JSON with secret values replaced:
//   - object members whose name is a secret and whose value is a string;
//   - "KEY=VALUE" strings whose KEY is a secret (env lists, `-e K=V` argv);
//   - "--flag=VALUE" and "--flag VALUE" in string lists when the flag is a secret;
//   - the credentials that are secret by shape in every string (a log line
//     that carries one loses it).
//
// Plans and run records carry the operator's environment; the dashboard and
// the MCP server may be reachable from the network, so they never return
// secret values.
func JSON(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var g any
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	return value(g), nil
}

func value(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			if _, isStr := x.(string); isStr && secretName(k) {
				t[k] = redacted
				continue
			}
			t[k] = value(x)
		}
		return t
	case []any:
		prevSecretFlag := false
		for i, x := range t {
			s, ok := x.(string)
			if !ok {
				t[i] = value(x)
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
			t[i] = keyValue(s)
		}
		return t
	case string:
		return keyValue(shapes(t))
	}
	return v
}

// keyValue handles one "KEY=VALUE" or "--flag=VALUE" string.
func keyValue(s string) string {
	k, _, ok := strings.Cut(s, "=")
	if !ok || k == "" || strings.ContainsAny(k, " \t") || !secretName(k) {
		return s
	}
	return k + "=" + redacted
}

var (
	// secretToken matches credentials by shape: Hugging Face and OpenAI-style
	// keys and bearer tokens.
	secretToken = regexp.MustCompile(`\bhf_[A-Za-z0-9]{20,}|\bsk-[A-Za-z0-9_-]{20,}|(?i:bearer)\s+[A-Za-z0-9._~+/=-]{16,}`)
	// assignment matches `name=value`, `name: value` and `"name": "value"`.
	assignment = regexp.MustCompile(`("?)([A-Za-z][A-Za-z0-9_.-]*)("?\s*[=:]\s*"?)([^\s"',;}]+)`)
)

// Text redacts free text such as a run log: credentials by shape, and the
// value of every `name=value` or `name: value` whose name is a secret name.
func Text(s string) string {
	s = shapes(s)
	return assignment.ReplaceAllStringFunc(s, func(m string) string {
		g := assignment.FindStringSubmatch(m)
		if !secretName(g[2]) || g[4] == redacted {
			return m
		}
		return g[1] + g[2] + g[3] + redacted
	})
}

// shapes replaces the credentials that are secret by shape: Hugging Face and
// OpenAI-style keys and bearer tokens. Every string of a document goes
// through it, so a log line that carries one loses it.
func shapes(s string) string {
	return secretToken.ReplaceAllString(s, redacted)
}
