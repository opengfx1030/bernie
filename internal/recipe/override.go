package recipe

import (
	"fmt"
	"strings"
)

// Override is one parsed --set.
type Override struct {
	Key   string   // as given
	Input bool     // KEY has no dot: sets the input KEY
	Path  []string // section path when !Input
	Raw   string   // VALUE as given
	Value any      // VALUE parsed as YAML (fields only)
}

var settable = map[string]bool{
	"name": true, "description": true, "status": true, "priority": true, "entrypoint": true,
	"tags": true, "model": true, "requires": true, "inputs": true, "container": true,
	"env": true, "args": true,
}

// ParseSet parses KEY=VALUE (spec §7.5). Input values stay plain strings;
// field values are parsed as YAML (empty VALUE is the empty string).
func ParseSet(s string) (Override, error) {
	k, v, ok := strings.Cut(s, "=")
	if !ok || strings.TrimSpace(k) == "" {
		return Override{}, fmt.Errorf("--set %q: want KEY=VALUE", s)
	}
	o := Override{Key: k, Raw: v}
	if !strings.Contains(k, ".") {
		if !varPattern.MatchString(k) {
			return o, fmt.Errorf("--set %q: %q is not an input name", s, k)
		}
		o.Input = true
		return o, nil
	}
	o.Path = strings.Split(k, ".")
	for _, p := range o.Path {
		if p == "" {
			return o, fmt.Errorf("--set %q: empty path element", s)
		}
	}
	if !settable[o.Path[0]] {
		return o, fmt.Errorf("--set %q: unknown section %q (sections: args, env, model, requires, inputs, container, ...)", s, o.Path[0])
	}
	val, err := parseValue(v)
	if err != nil {
		return o, fmt.Errorf("--set %q: value: %w", s, err)
	}
	o.Value = val
	return o, nil
}

// Apply returns a copy of r with the field overrides of sets applied, plus
// the input values they set. Unknown inputs are an error.
func Apply(r *Recipe, sets []string) (*Recipe, map[string]string, error) {
	doc := docOf(r)
	inputs := map[string]string{}
	var inputKeys []string
	for _, s := range sets {
		o, err := ParseSet(s)
		if err != nil {
			return nil, nil, err
		}
		if o.Input {
			inputs[o.Key] = o.Raw
			inputKeys = append(inputKeys, o.Key)
			continue
		}
		if err := setPath(doc, o.Path, o.Value); err != nil {
			return nil, nil, fmt.Errorf("--set %s: %w", o.Key, err)
		}
	}
	out, err := decode(doc)
	if err != nil {
		return nil, nil, fmt.Errorf("after --set: %w", err)
	}
	out.ID, out.Source, out.Fragment, out.Extends, out.doc = r.ID, r.Source, r.Fragment, r.Extends, r.doc
	for _, k := range inputKeys {
		if _, ok := out.Inputs[k]; !ok {
			return nil, nil, fmt.Errorf("--set %s: recipe %s has no input %s%s", k, r.ID, k, inputHint(out))
		}
	}
	return out, inputs, nil
}

func inputHint(r *Recipe) string {
	if len(r.Inputs) == 0 {
		return " (it has no inputs; use section.key=VALUE for fields, e.g. args.port=8001)"
	}
	return " (inputs: " + strings.Join(sortedKeys(r.Inputs), ", ") + ")"
}
