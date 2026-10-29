package mcp

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/support/redact"
)

// Outcome is the result of a mutating tool (spec §13.5). The plan call
// returns Executed false with the plan and a token. The confirmed call
// returns Executed true with the CLI document in Result.
type Outcome[R any] struct {
	Executed     bool       `json:"executed"`
	Plan         any        `json:"plan,omitempty"`
	ConfirmToken string     `json:"confirm_token,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Note         string     `json:"note,omitempty"`
	Result       *R         `json:"result,omitempty"`
}

func (o Outcome[R]) confirmToken() string { return o.ConfirmToken }

var errBusy = errors.New("another operation is running; try again when it ends")

// mutation is one run of the two-step protocol.
type mutation[R any] struct {
	tool  string
	token string // the confirm_token argument; "" for the plan call
	args  any    // the normalized arguments, without the token
	// plan computes the plan the operator confirms. It is called once, after
	// the token is verified: a call that the gate refuses never computes a
	// plan, and never has the side effects of one.
	plan func() (any, error)
	lock *sync.Mutex
	// busy is the error of a busy lock. It is errBusy unless the tool has
	// its own sentence: a benchmark shares the busy answer of the other
	// front ends.
	busy error
	// run does the work. It runs only after a valid token.
	run func(ctx context.Context) (*R, error)
}

// busyError is the answer of a busy lock.
func (m *mutation[R]) busyError() error {
	if m.busy != nil {
		return m.busy
	}
	return errBusy
}

// execute plans, or verifies the token and runs. It never changes the host
// without a valid token: the plan call has no side effect, and a token is
// bound to the tool, the arguments, the plan digest and the host.
//
// The confirm call verifies the token before it computes the plan: a call the
// gate refuses never touches the host.
func execute[R any](s *Server, c *call, m mutation[R]) (any, error) {
	argsDigest, err := digest(m.args)
	if err != nil {
		return nil, err
	}
	c.digest = argsDigest[:16]

	if m.token == "" {
		// The plan call: the plan is computed and the token is bound to it.
		raw, err := m.plan()
		if err != nil {
			return nil, err
		}
		g, err := redact.JSON(raw)
		if err != nil {
			return nil, err
		}
		planDigest, err := digest(g)
		if err != nil {
			return nil, err
		}
		tok, exp, err := s.tokens.issue(m.tool, argsDigest, planDigest)
		if err != nil {
			c.result, c.class = "failed", "error"
			return nil, err
		}
		c.result, c.tokenID = "planned", tokenID(tok)
		return Outcome[R]{Plan: g, ConfirmToken: tok, ExpiresAt: &exp}, nil
	}

	c.tokenID = tokenID(m.token)
	if m.lock != nil {
		if !m.lock.TryLock() {
			c.result, c.class = "rejected", "busy"
			return nil, m.busyError()
		}
		defer m.lock.Unlock()
	}
	bound, err := s.tokens.verify(m.token, m.tool, argsDigest)
	if err != nil {
		c.result, c.class = "rejected", "token"
		return nil, err
	}
	raw, err := m.plan()
	if err != nil {
		return nil, err
	}
	g, err := redact.JSON(raw)
	if err != nil {
		return nil, err
	}
	planDigest, err := digest(g)
	if err != nil {
		return nil, err
	}
	if planDigest != bound {
		c.result, c.class = "rejected", "token"
		return nil, errTokenBinding
	}
	res, err := m.run(c.ctx)
	if err != nil {
		c.result, c.class = "failed", "error"
		return nil, err
	}
	c.result = "applied"
	return Outcome[R]{Executed: true, Result: res}, nil
}

// planOnly is the outcome of a call whose plan has nothing to run.
func planOnly[R any](c *call, plan any, note string) Outcome[R] {
	c.result = "planned"
	return Outcome[R]{Plan: plan, Note: note}
}
