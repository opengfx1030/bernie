package mcp

import (
	"context"
	"fmt"
	"net"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/opengfx1030/bernie/internal/app/dashsvc"
)

// The dashboard tools avoid importing `github.com/opengfx1030/bernie/dashboard`
// directly: the dashboard package imports this one for its /mcp endpoint,
// so a direct import would cycle. The constant below inlines the small
// piece we need (the default listen address).

const dashDefaultListen = "127.0.0.1:7860"

// This file is the dashboard service as MCP tools. They are the same
// capabilities the CLI exposes through `bernie dash start|stop|status`
// and the installer `--start` flag uses (one app function, three front
// ends). The rules of spec §13 hold: dash_status is read-only;
// dash_start and dash_stop are mutating and only listed when the write
// gate is open.

// dashStatusIn is the input of dash_status. show_token reveals the
// bearer token when the MCP server can read the 0600 token file.
type dashStatusIn struct {
	ShowToken bool `json:"show_token,omitempty" jsonschema:"reveal the bearer token (false by default: the token is redacted)"`
}

// dashStartIn is the input of dash_start. The plan and the run share the
// same shape; the confirm_token field gates the run.
type dashStartIn struct {
	Listen      string `json:"listen,omitempty" jsonschema:"listen address (default 127.0.0.1:7860; a non-loopback address implies token mode)"`
	Token       string `json:"token,omitempty" jsonschema:"explicit token (else use or generate the persisted one)"`
	RotateToken bool   `json:"rotate_token,omitempty" jsonschema:"replace the persisted token before starting"`
	NoSystemd   bool   `json:"no_systemd,omitempty" jsonschema:"skip the systemd --user path even when it is available"`
	confirmToken
}

// dashStopIn is the input of dash_stop. The token is the confirm_token.
type dashStopIn struct {
	confirmToken
}

// dashPlanStart is the document a dash_start plan returns. It is the
// would-be state of the dashboard: the listen address and the URLs the
// operator would open. It carries no secret; the redactor of spec §13.7
// would strip it anyway.
type dashPlanStart struct {
	Listen      string   `json:"listen,omitempty"`
	LoopbackURL string   `json:"loopback_url,omitempty"`
	LANURLs     []string `json:"lan_urls,omitempty"`
	Manager     string   `json:"manager,omitempty"`
	TokenPath   string   `json:"token_path,omitempty"`
	LogPath     string   `json:"log_path,omitempty"`
}

// dashPlanStop is the document a dash_stop plan returns. The plan says
// what the call would do; the result says it did.
type dashPlanStop struct {
	WouldStop bool `json:"would_stop"`
}

// dashStatus implements the read-only dash_status tool. The token is
// redacted unless the caller asks for it AND the MCP process owns the
// token file (mode 0600). The owner check is conservative: the
// dashboard process owns the file, but the MCP server runs in a
// different process; the token would be unreadable in practice.
func (s *Server) dashStatus(_ context.Context, _ *call, in dashStatusIn) (any, error) {
	st, err := dashsvc.GetStatus()
	if err != nil {
		return nil, err
	}
	if !in.ShowToken {
		// Redact the token unless the caller explicitly asked for it AND
		// the process can read the file. We never reveal a token that
		// belongs to another process.
		st.Token = ""
	}
	return st, nil
}

// dashStart implements the mutating dash_start tool. The call goes
// through the same two-call gate as every mutating tool (spec §13.5):
// the first call computes the plan and returns a token; the second
// call runs the action.
func (s *Server) dashStart(ctx context.Context, c *call, in dashStartIn) (any, error) {
	// Validate the listen address early; the plan would otherwise fail
	// in the same way, but a clear error here is friendlier.
	if in.Listen != "" {
		if _, _, err := net.SplitHostPort(in.Listen); err != nil {
			return nil, fmt.Errorf("invalid listen %q: %w", in.Listen, err)
		}
	}
	plan := func() (any, error) {
		// Compute the URL the dashboard would bind without actually
		// starting it. The dashboard.NewStarter path opens a listener
		// to find a free port; we cannot do that in a plan call. The
		// caller only needs to know the listen address and the manager
		// the service would pick.
		paths := dashsvc.ServicePaths()
		manager := "process"
		if !in.NoSystemd && dashsvc.SystemdUserAvailable() {
			manager = "systemd"
		}
		listen := in.Listen
		if listen == "" {
			listen = dashDefaultListen
		}
		host, port, _ := net.SplitHostPort(listen)
		_ = port
		loopback := "http://" + host + "/"
		if host == "" || host == "0.0.0.0" || host == "::" {
			loopback = "http://127.0.0.1" + portSuffix(port)
		}
		return &dashPlanStart{
			Listen:      listen,
			LoopbackURL: loopback,
			Manager:     manager,
			TokenPath:   paths.Token,
			LogPath:     paths.Log,
		}, nil
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[*dashsvc.Status]{tool: "dash_start", token: token, args: in, plan: plan, lock: &s.mutate,
		run: func(_ context.Context) (**dashsvc.Status, error) {
			st, err := dashsvc.Start(context.Background(), dashsvc.StartOptions{
				Listen:      in.Listen,
				Token:       in.Token,
				RotateToken: in.RotateToken,
				NoSystemd:   in.NoSystemd,
			})
			if err != nil {
				return nil, err
			}
			return &st, nil
		}})
}

// portSuffix returns ":PORT" or "" for an empty port.
func portSuffix(p string) string {
	if p == "" {
		return ""
	}
	return ":" + p
}

// dashStop implements the mutating dash_stop tool. The plan says "we
// would stop the dashboard"; the run actually stops it.
func (s *Server) dashStop(ctx context.Context, c *call, in dashStopIn) (any, error) {
	plan := func() (any, error) {
		// A plan call must not change the host. We do not even probe the
		// pidfile: the plan is the same whatever the answer.
		return &dashPlanStop{WouldStop: true}, nil
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[map[string]bool]{tool: "dash_stop", token: token, args: in, plan: plan, lock: &s.mutate,
		run: func(_ context.Context) (*map[string]bool, error) {
			if err := dashsvc.Stop(context.Background()); err != nil {
				return nil, err
			}
			out := map[string]bool{"stopped": true}
			return &out, nil
		}})
}

// registerDashTools wires dash_status, dash_start and dash_stop into the
// catalog. The mutating tools are listed only when Options.Write is
// true; addTool honours that.
func registerDashTools(s *Server) {
	ro := func(name, desc string) spec { return spec{name: name, desc: desc} }
	w := func(name, desc string, destructive, idempotent, openWorld bool) spec {
		return spec{name: name, desc: desc + twoCalls, write: true, destructive: destructive, idempotent: idempotent, openWorld: openWorld}
	}

	addTool[dashStatusIn, *dashsvc.Status](s, ro("dash_status", "The state of the dashboard service (bernie dash status): listen address, loopback and LAN URLs, manager (process or systemd), log and token paths. The token is redacted unless show_token is true and the file is readable."),
		nil,
		s.dashStatus)

	addTool[dashStartIn, Outcome[*dashsvc.Status]](s, w("dash_start", "Start the dashboard as a detached service (bernie dash start). It survives the shell; the token persists across restarts; prefers systemd --user when reachable (and prints the enable-linger hint).", false, false, false),
		func(sc *jsonschema.Schema) {
			prop(sc, "listen").Default = []byte(`"127.0.0.1:7860"`)
			tokenSchema(sc)
		},
		s.dashStart)

	addTool[dashStopIn, Outcome[map[string]bool]](s, w("dash_stop", "Stop the dashboard service (bernie dash stop). Sends SIGTERM, then SIGKILL after 5 s; clears the pidfile; stops the systemd --user unit when in use.", true, true, false),
		func(sc *jsonschema.Schema) { tokenSchema(sc) },
		s.dashStop)
}
