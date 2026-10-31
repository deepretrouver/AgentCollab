package runnerclient

import (
	"errors"
	"strings"
	"testing"
)

func TestCommandErrorSummaryDoesNotPublishRawDiagnostics(t *testing.T) {
	for _, diagnostic := range []string{
		"failed reading /private/example/config token=private-value",
		"unknown model /private/example/config token=private-value",
		"stream disconnected token=private-value",
	} {
		summary := commandErrorSummary(agentCommandResult{Stderr: diagnostic, Err: errors.New("failed"), ExitCode: 1})
		if strings.Contains(summary, "private-value") || strings.Contains(summary, "/private/") {
			t.Fatalf("leaked diagnostic: %s", summary)
		}
		if summary == "" {
			t.Fatal("missing actionable summary")
		}
	}
}
