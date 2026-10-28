package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// auditEntry is one line of <state>/mcp/audit.jsonl (spec §13.9). It never
// holds a bearer token, a confirm token, an environment map or a log body:
// Args is the redacted argument summary, TokenID the hash prefix of the token.
type auditEntry struct {
	Time       time.Time `json:"time"`
	Transport  string    `json:"transport"`
	Client     string    `json:"client,omitempty"`
	Tool       string    `json:"tool"`
	Digest     string    `json:"digest,omitempty"`
	Args       string    `json:"args,omitempty"`
	TokenID    string    `json:"token_id,omitempty"`
	Result     string    `json:"result"` // ok, planned, applied, rejected, failed
	DurationMS int64     `json:"duration_ms"`
	ErrorClass string    `json:"error_class,omitempty"`
}

type auditLog struct {
	path string
	mu   sync.Mutex
}

func (a *auditLog) write(e auditEntry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(a.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
