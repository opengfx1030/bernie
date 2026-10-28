package recipe

import (
	"fmt"
	"strings"
)

// Setting is one plan setting and the reason attached to it.
type Setting struct {
	Setting string `json:"setting"`
	Value   string `json:"value"`
	Why     string `json:"why"`
}

// planSettings adds the annotated settings to a plan. Settings are sorted by
// key so the text and JSON forms have one stable order.
func planSettings(r *Recipe, p *Plan, interp func(string) string) {
	p.Settings = nil
	for _, key := range sortedKeys(r.Why) {
		value, ok := settingValue(r, p, key, interp)
		if !ok {
			continue
		}
		p.Settings = append(p.Settings, Setting{Setting: key, Value: value, Why: r.Why[key]})
	}
}

// settingValue returns the final rendered value of a setting path.
func settingValue(r *Recipe, p *Plan, key string, interp func(string) string) (string, bool) {
	switch {
	case strings.HasPrefix(key, "env."):
		name := strings.TrimPrefix(key, "env.")
		v, ok := p.Env[name]
		return v, ok
	case strings.HasPrefix(key, "args."):
		name := strings.TrimPrefix(key, "args.")
		v, ok := r.Args[name]
		if !ok {
			return "", false
		}
		v = interpAny(v, interp)
		switch v.(type) {
		case map[string]any, []any:
			j, err := compactJSON(v)
			if err != nil {
				return "", false
			}
			return j, true
		default:
			return scalarString(v), true
		}
	case key == "tuning.tunableop_profile":
		return p.TunableOpProfile, true
	}
	return "", false
}

// RefreshSettingValues updates values that the runner changes after
// BuildPlan, such as the selected TunableOp profile and mounted paths.
func (p *Plan) RefreshSettingValues() {
	for i := range p.Settings {
		key := p.Settings[i].Setting
		switch {
		case strings.HasPrefix(key, "env."):
			name := strings.TrimPrefix(key, "env.")
			p.Settings[i].Value = p.Env[name]
		case key == "tuning.tunableop_profile":
			p.Settings[i].Value = p.TunableOpProfile
		}
	}
}

// validateWhy checks that every reason names a setting and stays on one line.
func validateWhy(r *Recipe) []string {
	var errs []string
	for _, key := range sortedKeys(r.Why) {
		reason := r.Why[key]
		if strings.TrimSpace(reason) == "" {
			errs = append(errs, fmt.Sprintf("why.%s: reason must not be empty", key))
		}
		if strings.ContainsAny(reason, "\r\n") {
			errs = append(errs, fmt.Sprintf("why.%s: reason must be one line", key))
		}
		switch {
		case strings.HasPrefix(key, "env."):
			name := strings.TrimPrefix(key, "env.")
			if !varPattern.MatchString(name) {
				errs = append(errs, fmt.Sprintf("why.%s: not a valid environment variable name", key))
			} else if _, ok := r.Env[name]; !ok {
				errs = append(errs, fmt.Sprintf("why.%s: recipe has no env.%s setting", key, name))
			}
		case strings.HasPrefix(key, "args."):
			name := strings.TrimPrefix(key, "args.")
			if !argPattern.MatchString(name) {
				errs = append(errs, fmt.Sprintf("why.%s: not a valid argument name", key))
			} else if _, ok := r.Args[name]; !ok {
				errs = append(errs, fmt.Sprintf("why.%s: recipe has no args.%s setting", key, name))
			}
		case key == "tuning.tunableop_profile":
		default:
			errs = append(errs, fmt.Sprintf("why.%s: want env.NAME, args.FLAG or tuning.tunableop_profile", key))
		}
	}
	return errs
}
