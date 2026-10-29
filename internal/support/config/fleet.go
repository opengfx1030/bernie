package config

import (
	"fmt"
	"net/url"
	"strings"
)

// FleetEntry is one configured peer of the Fleet tab: the name the page
// shows, the dashboard URL and the token that host printed. The token
// stays in the file (mode 0600); it never reaches the browser (spec §11).
type FleetEntry struct {
	Name  string `json:"name" yaml:"name"`
	URL   string `json:"url" yaml:"url"`
	Token string `json:"token,omitempty" yaml:"token,omitempty"`
}

// UpsertFleet adds or replaces one entry by name. A name that exists keeps
// its token when the new entry has none.
func (c *Config) UpsertFleet(e FleetEntry) {
	for i, cur := range c.Fleet {
		if cur.Name == e.Name {
			if e.Token == "" {
				e.Token = cur.Token
			}
			c.Fleet[i] = e
			return
		}
	}
	c.Fleet = append(c.Fleet, e)
}

// RemoveFleet drops the entry with this name. The result reports whether a
// change was made.
func (c *Config) RemoveFleet(name string) bool {
	out := c.Fleet[:0:0]
	removed := false
	for _, e := range c.Fleet {
		if e.Name == name {
			removed = true
			continue
		}
		out = append(out, e)
	}
	c.Fleet = out
	return removed
}

// ValidateFleetEntry checks one entry: name non-empty, URL http or https,
// token allowed to be empty.
func ValidateFleetEntry(e FleetEntry) error {
	if e.Name == "" {
		return fmt.Errorf("name is required")
	}
	if e.URL == "" {
		return fmt.Errorf("url is required")
	}
	u, err := url.Parse(e.URL)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must be http or https, not %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("url must name a host")
	}
	return nil
}

// parseFleetOp splits the value of "config set fleet VALUE" into one fleet
// command. The shape is "add NAME URL [TOKEN]" or "rm NAME".
func parseFleetOp(value string) (op, name, url, token string, err error) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "", "", "", "", fmt.Errorf("config fleet: need add NAME URL or rm NAME")
	}
	op = fields[0]
	switch op {
	case "add":
		if len(fields) < 3 {
			return "", "", "", "", fmt.Errorf("config fleet add: need NAME URL [TOKEN]")
		}
		name, url = fields[1], fields[2]
		if len(fields) >= 4 {
			token = fields[3]
		}
	case "rm":
		if len(fields) != 2 {
			return "", "", "", "", fmt.Errorf("config fleet rm: need NAME")
		}
		name = fields[1]
	default:
		return "", "", "", "", fmt.Errorf("config fleet: unknown op %q (have add, rm)", op)
	}
	return op, name, url, token, nil
}

// hasSecret reports whether the config carries a value that must stay
// readable by its owner only. The fleet token is the only such value today.
func (c Config) hasSecret() bool {
	for _, e := range c.Fleet {
		if e.Token != "" {
			return true
		}
	}
	return false
}
