package core

import (
	"context"
	"os"
	"testing"
	"time"
)

// Opt-in because this reads the user's running Charles instance. It never starts,
// stops or clears recording, persists captures, or sends captured requests.
func TestRealCharlesReadOnlySmoke(t *testing.T) {
	if os.Getenv("CHARLES_MCP_LIVE_SMOKE") != "1" {
		t.Skip("set CHARLES_MCP_LIVE_SMOKE=1 to read a local Charles session")
	}
	env := func(key, fallback string) string {
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return fallback
	}
	c, err := NewCharles(env("CHARLES_BASE_URL", "http://control.charles"), env("CHARLES_PROXY_URL", "http://127.0.0.1:8888"), env("CHARLES_USER", "admin"), env("CHARLES_PASS", "123456"), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.CLI = os.Getenv("CHARLES_CLI_PATH")
	for _, format := range []string{"xml", "json", "native"} {
		t.Run(format, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			entries, err := c.Snapshot(ctx, format, "smoke")
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("parsed %d entries", len(entries))
			for _, e := range entries {
				if e.ID == "" || e.URL == "" || e.CaptureID != "smoke" {
					t.Fatal("invalid normalized entry")
				}
			}
		})
	}
}
