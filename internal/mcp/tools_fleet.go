package mcp

import (
	"context"
	"strings"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// fleetAddIn adds one peer to the fleet. The token is optional: a loopback
// host that prints no token has none.
type fleetAddIn struct {
	Name  string `json:"name" jsonschema:"unique name of the peer (shown on the Fleet tab)"`
	URL   string `json:"url" jsonschema:"dashboard URL of the peer (http or https)"`
	Token string `json:"token,omitempty" jsonschema:"dashboard token of the peer (optional; stored with 0600 and never returned)"`
	confirmToken
}

// fleetRmIn forgets one peer of the fleet.
type fleetRmIn struct {
	Name string `json:"name" jsonschema:"name of the peer to forget"`
	confirmToken
}

// fleetAddPlan is the plan the operator confirms: the entry that will be
// added (with the token).
type fleetAddPlan struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}

// fleetRmPlan is the plan the operator confirms: the name that will be
// forgotten.
type fleetRmPlan struct {
	Name string `json:"name"`
}

// fleetAdd adds one peer to the fleet. The two-call gate verifies the
// token before the config file is touched.
func (s *Server) fleetAdd(_ context.Context, c *call, in fleetAddIn) (any, error) {
	name, url, token := strings.TrimSpace(in.Name), strings.TrimSpace(in.URL), in.Token
	plan := fleetAddPlan{Name: name, URL: url, Token: token}
	tok := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[config.FleetEntry]{tool: "fleet_add", token: tok, args: in, plan: func() (any, error) { return plan, nil }, lock: &s.mutate,
		run: func(context.Context) (*config.FleetEntry, error) {
			e, err := app.FleetAdd(name, url, token)
			if err != nil {
				return nil, err
			}
			// The answer hides the token: the file holds it.
			e.Token = ""
			return &e, nil
		}})
}

// fleetRm forgets one peer. The two-call gate verifies the token before
// the config file is touched.
func (s *Server) fleetRm(_ context.Context, c *call, in fleetRmIn) (any, error) {
	name := strings.TrimSpace(in.Name)
	plan := fleetRmPlan{Name: name}
	tok := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[map[string]string]{tool: "fleet_rm", token: tok, args: in, plan: func() (any, error) { return plan, nil }, lock: &s.mutate,
		run: func(context.Context) (*map[string]string, error) {
			if err := app.FleetRemove(name); err != nil {
				return nil, err
			}
			out := map[string]string{"removed": name}
			return &out, nil
		}})
}
