package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRequestPacingDefaults(t *testing.T) {
	for _, name := range []string{
		"MFL_YEAR", "MFL_LEAGUE_ID", "MFL_API_KEY", "MFL_USER_COOKIE",
		"MFL_BASE_URL", "MFL_USER_AGENT", "MFL_TIMEOUT", "MFL_MIN_INTERVAL",
	} {
		t.Setenv(name, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinInterval != time.Second {
		t.Fatalf("MinInterval = %s, want 1s", cfg.MinInterval)
	}
	if cfg.UserAgent != "mfl-mcp/0.2.0" {
		t.Fatalf("UserAgent = %q, want mfl-mcp/0.2.0", cfg.UserAgent)
	}
}

func TestLoadRejectsNegativeRequestInterval(t *testing.T) {
	t.Setenv("MFL_MIN_INTERVAL", "-1s")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "non-negative duration") {
		t.Fatalf("Load error = %v, want non-negative duration error", err)
	}
}
