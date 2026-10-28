package recipe

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// parseYAML decodes one YAML document into plain Go values: map[string]any,
// []any, string, bool, int64, json.Number (floats keep their source text, so
// 0.90 renders as 0.90) and nil. An empty document is nil.
func parseYAML(b []byte) (any, error) {
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	if n.Kind == 0 {
		return nil, nil
	}
	return nodeValue(&n)
}

func nodeValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return nodeValue(n.Content[0])
	case yaml.AliasNode:
		return nodeValue(n.Alias)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Tag == "!!merge" {
				return nil, fmt.Errorf("line %d: YAML merge keys (<<) are not supported; use extends", k.Line)
			}
			if k.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("line %d: map keys must be scalars", k.Line)
			}
			if _, dup := m[k.Value]; dup {
				return nil, fmt.Errorf("line %d: duplicate key %q", k.Line, k.Value)
			}
			val, err := nodeValue(v)
			if err != nil {
				return nil, err
			}
			m[k.Value] = val
		}
		return m, nil
	case yaml.SequenceNode:
		l := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := nodeValue(c)
			if err != nil {
				return nil, err
			}
			l = append(l, v)
		}
		return l, nil
	case yaml.ScalarNode:
		return scalarValue(n)
	}
	return nil, fmt.Errorf("line %d: unsupported YAML node", n.Line)
}

func scalarValue(n *yaml.Node) (any, error) {
	switch n.ShortTag() {
	case "!!null":
		return nil, nil
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return nil, err
		}
		return b, nil
	case "!!int":
		var i int64
		if err := n.Decode(&i); err != nil {
			return nil, fmt.Errorf("line %d: %w", n.Line, err)
		}
		return i, nil
	case "!!float":
		if json.Valid([]byte(n.Value)) {
			return json.Number(n.Value), nil
		}
		var f float64
		if err := n.Decode(&f); err != nil {
			return nil, fmt.Errorf("line %d: %w", n.Line, err)
		}
		return json.Number(strconv.FormatFloat(f, 'f', -1, 64)), nil
	default:
		return n.Value, nil
	}
}

// parseValue parses a --set VALUE. Empty is the empty string (not null).
func parseValue(s string) (any, error) {
	if strings.TrimSpace(s) == "" {
		return s, nil
	}
	return parseYAML([]byte(s))
}

// deepCopy copies maps and lists; scalars are immutable.
func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = deepCopy(x)
		}
		return m
	case []any:
		l := make([]any, len(t))
		for i, x := range t {
			l[i] = deepCopy(x)
		}
		return l
	}
	return v
}

// merge merges src into dst (spec §7.3): maps merge key by key, lists and
// scalars replace, null deletes. Under the top-level "env" map a null is
// kept as an unset marker so the plan can also remove the variable from the
// inherited environment.
func merge(dst, src map[string]any, path string) {
	for k, sv := range src {
		p := k
		if path != "" {
			p = path + "." + k
		}
		if sv == nil {
			if path == "env" {
				dst[k] = nil
			} else {
				delete(dst, k)
			}
			continue
		}
		sm, sok := sv.(map[string]any)
		dm, dok := dst[k].(map[string]any)
		if sok && dok {
			merge(dm, sm, p)
			continue
		}
		if sok {
			// Merge into an empty map so nested nulls follow the same rules.
			nm := map[string]any{}
			merge(nm, sm, p)
			dst[k] = nm
			continue
		}
		dst[k] = deepCopy(sv)
	}
}

// setPath sets a dotted path in doc, creating maps on the way.
func setPath(doc map[string]any, keys []string, v any) error {
	m := doc
	for i, k := range keys[:len(keys)-1] {
		next, ok := m[k].(map[string]any)
		if !ok {
			if m[k] != nil {
				return fmt.Errorf("%s is not a map", strings.Join(keys[:i+1], "."))
			}
			next = map[string]any{}
			m[k] = next
		}
		m = next
	}
	merge(m, map[string]any{keys[len(keys)-1]: v}, strings.Join(keys[:len(keys)-1], "."))
	return nil
}
