package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// TokenTTL is how long a confirm token stays valid.
const TokenTTL = 5 * time.Minute

// tokenStore keeps the issued confirm tokens as files under <state>/mcp/tokens.
// A file is named by the SHA-256 of its token, so the token itself is never
// stored. Removing the file is the single use: of two concurrent consumers
// only one remove succeeds.
type tokenStore struct {
	dir string
	now func() time.Time // tests set it; nil is time.Now
}

// tokenRec is what a token is bound to.
type tokenRec struct {
	Tool    string    `json:"tool"`
	Args    string    `json:"args"` // digest of the normalized arguments
	Plan    string    `json:"plan"` // digest of the plan the operator saw
	Host    string    `json:"host"` // host name
	Expires time.Time `json:"expires"`
}

func (t tokenStore) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

func hashToken(token string) string {
	s := sha256.Sum256([]byte(token))
	return hex.EncodeToString(s[:])
}

// tokenID is the short id of a token for the audit log.
func tokenID(token string) string { return hashToken(token)[:12] }

// digest is the SHA-256 (hex) of the canonical JSON of v.
func digest(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

// issue creates a token for (tool, args, plan) and purges expired tokens.
func (t tokenStore) issue(tool, args, plan string) (string, time.Time, error) {
	if err := os.MkdirAll(t.dir, 0o700); err != nil {
		return "", time.Time{}, err
	}
	t.purge()
	var raw [32]byte // 256 random bits
	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	host, _ := os.Hostname()
	rec := tokenRec{Tool: tool, Args: args, Plan: plan, Host: host, Expires: t.clock().Add(TokenTTL).UTC()}
	b, err := json.Marshal(rec)
	if err != nil {
		return "", time.Time{}, err
	}
	if err := os.WriteFile(filepath.Join(t.dir, hashToken(token)), b, 0o600); err != nil {
		return "", time.Time{}, err
	}
	return token, rec.Expires, nil
}

// Token errors. A presented token is consumed whatever the verdict, so a
// wrong guess or a changed plan needs a fresh plan call.
var (
	errTokenUnknown = errors.New("confirm_token is unknown or was already used")
	errTokenExpired = errors.New("confirm_token expired; call the tool again without it to get a new plan")
	errTokenBinding = errors.New("confirm_token does not match this tool, these arguments or the current plan; call the tool again without it to get a new plan")
)

// verify checks a confirm token and burns it. It returns the digest of the
// plan the token is bound to, so the caller can check the plan it computed
// against it. A token is dead whatever the verdict: a token with changed
// arguments or a changed plan needs a new plan call.
//
// The plan is not part of the check: the caller verifies the token before it
// computes the plan, so a call that the gate refuses never has the side
// effects of a plan.
func (t tokenStore) verify(token, tool, args string) (string, error) {
	path := filepath.Join(t.dir, hashToken(token))
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errTokenUnknown
	}
	if err := os.Remove(path); err != nil {
		return "", errTokenUnknown // another call consumed it first
	}
	var rec tokenRec
	if err := json.Unmarshal(b, &rec); err != nil {
		return "", errTokenUnknown
	}
	if !t.clock().Before(rec.Expires) {
		return "", errTokenExpired
	}
	host, _ := os.Hostname()
	if rec.Tool != tool || rec.Args != args || rec.Host != host {
		return "", errTokenBinding
	}
	return rec.Plan, nil
}

// purge removes the files of expired tokens.
func (t tokenStore) purge() {
	entries, _ := os.ReadDir(t.dir)
	for _, e := range entries {
		p := filepath.Join(t.dir, e.Name())
		b, err := os.ReadFile(p)
		var rec tokenRec
		if err != nil || json.Unmarshal(b, &rec) != nil || !t.clock().Before(rec.Expires) {
			os.Remove(p)
		}
	}
}
