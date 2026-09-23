package mail

import (
	"strings"
	"testing"
	"time"
)

func TestBuildStripsHeaderNewlines(t *testing.T) {
	msg := string(Build("ops@example.com", []string{"support@example.com"}, Notice{
		ID:            7,
		Product:       "PulseCheck",
		Component:     "API\r\nBcc: attacker@example.com",
		Category:      "Errors",
		Description:   "timeouts",
		ReporterEmail: "person@example.com",
		When:          time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
		AdminURL:      "https://pulse.example/admin",
	}))
	if strings.Contains(msg, "\r\nBcc:") || strings.Contains(msg, "\nBcc:") {
		t.Fatalf("header injection survived:\n%s", msg)
	}
	if !strings.Contains(msg, "This is one unverified customer report.") {
		t.Fatal(msg)
	}
	if !strings.Contains(msg, "Report ID: 7") || !strings.Contains(msg, "person@example.com") {
		t.Fatal(msg)
	}
}
