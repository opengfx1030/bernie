// Package app holds the operations the three front ends share: the CLI
// calls them, the MCP server calls them and the dashboard calls them.
package app

import (
	"context"
	"fmt"
	"os"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/support/config"
	"github.com/opengfx1030/bernie/internal/support/fleet"
)

// FleetList is the list of the bernie hosts this box knows: this box
// first, then every configured peer. The token of a peer stays in the
// config file: it is not part of the document.
type FleetList struct {
	// Path is the config file the peers come from.
	Path string `json:"path"`
	// Hosts is one row per host: this box first, then the peers.
	Hosts []fleet.Summary `json:"hosts"`
}

// Fleet reads the fleet of this box: the local host and every configured
// peer. A peer that does not answer keeps its row and says why.
func Fleet(ctx context.Context) (FleetList, error) {
	cfg, err := config.Load()
	if err != nil {
		return FleetList{}, err
	}
	return FleetList{
		Path:  config.Path(),
		Hosts: fleet.Fetch(ctx, fleet.Source{Entries: cfg.Fleet, Self: selfSummary}),
	}, nil
}

// FleetAdd adds one peer to the fleet and saves the config file. The token
// stays in the file (0600 when a token is set) and is not part of the
// result. A name that already exists is an error.
func FleetAdd(name, url, token string) (config.FleetEntry, error) {
	entry := config.FleetEntry{Name: name, URL: url, Token: token}
	if err := config.ValidateFleetEntry(entry); err != nil {
		return config.FleetEntry{}, err
	}
	cfg, err := config.Load()
	if err != nil {
		return config.FleetEntry{}, err
	}
	for _, e := range cfg.Fleet {
		if e.Name == name {
			return config.FleetEntry{}, fmt.Errorf("a fleet entry named %q already exists", name)
		}
	}
	cfg.UpsertFleet(entry)
	if err := config.Save(cfg); err != nil {
		return config.FleetEntry{}, err
	}
	return entry, nil
}

// FleetRemove forgets one peer and saves the config file. The token goes
// with it. A name that is not in the list is an error.
func FleetRemove(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.RemoveFleet(name) {
		return fmt.Errorf("no fleet entry named %q", name)
	}
	return config.Save(cfg)
}

// selfSummary is the local row of the fleet: the cards and the run the
// Dashboard tab shows. The live generation speed (gen_tok_s) comes from
// the telemetry stream of the dashboard, which adds it to the row before
// it paints.
func selfSummary(ctx context.Context) fleet.Summary {
	row := fleet.Summary{Name: hostName(), Self: true, Reachable: true}
	rep, err := HW(ctx)
	if err == nil {
		for _, g := range rep.GPUs {
			card := fleet.GPUDetail{Index: g.HIPIndex, Name: g.Name}
			if g.VRAMBytes > 0 {
				gb := float64(g.VRAMBytes) / (1 << 30)
				card.VRAMGiB = &gb
			}
			if g.PowerCapW > 0 {
				w := g.PowerCapW
				card.PowerCapW = &w
			}
			row.GPU = append(row.GPU, card)
		}
	}
	if runs, err := runner.List(ctx); err == nil {
		for _, s := range runs {
			st := string(s.State)
			if st == "serving" || st == "running" {
				row.State = st
				row.Recipe = s.Recipe
				break
			}
		}
	}
	if row.State == "" {
		row.State = "stopped"
	}
	return row
}

// hostName reads the name of this host.
func hostName() string {
	if name, err := os.Hostname(); err == nil {
		return name
	}
	return "this box"
}
