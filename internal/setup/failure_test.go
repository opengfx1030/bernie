package setup

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestDownloadFailureNamesTheNetwork(t *testing.T) {
	err := &net.DNSError{Err: "no such host", Name: "huggingface.co"}
	got := downloadFailure(fmt.Errorf("Get \"https://huggingface.co\": %w", err), "model.download")
	if !strings.HasPrefix(got, "no network:") {
		t.Errorf("the failure is %q, want it to start with 'no network:'", got)
	}
	if !strings.Contains(got, "bernie setup apply --step model.download") {
		t.Errorf("the failure is %q, want the command that runs it again", got)
	}
}

func TestDownloadFailureKeepsOtherCauses(t *testing.T) {
	got := downloadFailure(errors.New("404 Not Found"), "model.download")
	if strings.Contains(got, "no network") {
		t.Errorf("the failure is %q, want the cause as it is", got)
	}
	if got != "404 Not Found" {
		t.Errorf("the failure is %q, want the cause as it is", got)
	}
}

func TestDownloadFailureOfNoError(t *testing.T) {
	if got := downloadFailure(nil, "model.download"); got != "" {
		t.Errorf("the failure is %q, want an empty text", got)
	}
}
