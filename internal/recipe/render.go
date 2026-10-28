package recipe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// interpolate replaces ${NAME} with vals[NAME]; $${ is a literal ${.
func interpolate(s string, vals map[string]string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	const esc = "\x00"
	s = strings.ReplaceAll(s, "$${", esc)
	s = refPattern.ReplaceAllStringFunc(s, func(m string) string {
		return vals[m[2:len(m)-1]]
	})
	return strings.ReplaceAll(s, esc, "${")
}

func interpAny(v any, f func(string) string) any {
	switch t := v.(type) {
	case string:
		return f(t)
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = interpAny(x, f)
		}
		return m
	case []any:
		l := make([]any, len(t))
		for i, x := range t {
			l[i] = interpAny(x, f)
		}
		return l
	}
	return v
}

// scalarString renders an env or arg scalar.
func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// RenderArgs renders vllm flags (spec §7.6): sorted by key; true gives
// --key; false and null give nothing; maps and lists give compact JSON;
// other values give --key VALUE.
func RenderArgs(args map[string]any) ([]string, error) {
	var out []string
	for _, k := range sortedKeys(args) {
		switch v := args[k].(type) {
		case nil:
		case bool:
			if v {
				out = append(out, "--"+k)
			}
		case map[string]any, []any:
			j, err := compactJSON(v)
			if err != nil {
				return nil, fmt.Errorf("args.%s: %w", k, err)
			}
			out = append(out, "--"+k, j)
		default:
			out = append(out, "--"+k, scalarString(v))
		}
	}
	return out, nil
}

func compactJSON(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
