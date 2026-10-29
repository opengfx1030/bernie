package hostroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootIsSlashWithoutTheVariable(t *testing.T) {
	t.Setenv(EnvVar, "")
	if got := Root(); got != "/" {
		t.Errorf("the root is %q, want /", got)
	}
}

func TestRootFollowsTheVariable(t *testing.T) {
	t.Setenv(EnvVar, "/srv/host/")
	if got := Root(); got != "/srv/host" {
		t.Errorf("the root is %q, want /srv/host", got)
	}
}

func TestSetPointsAtAnotherTree(t *testing.T) {
	t.Setenv(EnvVar, "")
	Set("/srv/other")
	if got := Root(); got != "/srv/other" {
		t.Errorf("the root is %q, want /srv/other", got)
	}
	if os.Getenv(EnvVar) != "/srv/other" {
		t.Errorf("the variable is %q, want /srv/other", os.Getenv(EnvVar))
	}
	Set("")
	if got := Root(); got != "/" {
		t.Errorf("the root is %q, want /", got)
	}
}

func TestSetCleansThePath(t *testing.T) {
	t.Setenv(EnvVar, "")
	Set("/srv/host/../host")
	if got := Root(); got != filepath.Clean("/srv/host") {
		t.Errorf("the root is %q, want %q", got, filepath.Clean("/srv/host"))
	}
}
