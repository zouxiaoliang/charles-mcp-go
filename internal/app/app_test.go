package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zouxiaoliang/charles-mcp-go/internal/core"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	a, err := New(Config{DataDir: dir, RecordingsDir: filepath.Join(dir, "recordings"), CharlesURL: "http://127.0.0.1:1", TimeoutSeconds: 1, SessionTTLSeconds: 900})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close(context.Background()) })
	return a
}
func TestMCPProtocolToolsSchemasAndCalls(t *testing.T) {
	a := newTestApp(t)
	server, err := a.Server("test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, err := cs.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 32 {
		t.Fatalf("tool registration: %d %v", len(list.Tools), err)
	}
	call := func(name string, args any) *mcp.CallToolResult {
		t.Helper()
		r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := call("reverse_list_captures", map[string]any{}); r.IsError {
		t.Fatalf("list: %+v", r)
	}
	for _, args := range []map[string]any{{}, {"entry_id": ""}, {"entry_id": "missing", "unknown": true}, {"entry_id": "missing", "max_chars": -1}} {
		if r := call("reverse_get_entry_detail", args); !r.IsError {
			t.Fatalf("accepted bad arguments: %+v", args)
		}
	}
	path := filepath.Join(a.Config.RecordingsDir, "session.xml")
	os.WriteFile(path, []byte(`<charles-session><transaction method="POST" protocol="http" host="example.test" path="/api/login"><request><headers><header><name>Content-Type</name><value>application/json</value></header></headers><body>{"name":"alice"}</body></request><response status="200"><body>{"ok":true}</body></response></transaction></charles-session>`), 0600)
	imported := call("reverse_import_session", map[string]any{"path": path})
	if imported.IsError {
		t.Fatalf("import: %+v", imported)
	}
	var wrapper struct {
		Result core.Capture `json:"result"`
	}
	json.Unmarshal([]byte(imported.Content[0].(*mcp.TextContent).Text), &wrapper)
	id := wrapper.Result.ID
	r := call("reverse_query_entries", map[string]any{"capture_id": id, "query": map[string]any{"request_json_query": "name == 'alice'"}})
	if r.IsError {
		t.Fatalf("query: %+v", r)
	}
	entries, err := a.Store.Entries(ctx, id)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries: %v %v", entries, err)
	}
	entry := entries[0].ID
	for _, name := range []string{"get_traffic_entry_detail", "reverse_get_entry_detail"} {
		if r := call(name, map[string]any{"entry_id": entry}); r.IsError {
			t.Fatalf("%s: %+v", name, r)
		}
	}
	if r := call("reverse_decode_entry_body", map[string]any{"entry_id": entry, "side": "request"}); r.IsError {
		t.Fatalf("decode: %+v", r)
	}
	// A null header override must validate and reach the replay builder. Deliberately
	// invalid body mutation fails before networking, with the builder's actual error.
	r = call("reverse_replay_entry", map[string]any{"entry_id": entry, "header_overrides": map[string]any{"Authorization": nil}, "form_overrides": map[string]any{"a": "b"}})
	if !r.IsError || r.Content[0].(*mcp.TextContent).Text != "form overrides require URL-encoded form content type" {
		t.Fatalf("replay schema: %+v", r)
	}
	for _, name := range []string{"analyze_recorded_traffic", "group_capture_analysis", "get_capture_analysis_stats", "get_recording_snapshot", "query_recorded_traffic"} {
		if r := call(name, map[string]any{"capture_id": id}); r.IsError {
			t.Fatalf("%s: %+v", name, r)
		}
	}
	for _, name := range []string{"reverse_analyze_live_login_flow", "reverse_analyze_live_api_flow", "reverse_analyze_live_signature_flow"} {
		if r := call(name, map[string]any{"capture_id": id}); r.IsError {
			t.Fatalf("%s: %+v", name, r)
		}
	}
	if r := call("delete_capture", map[string]any{"capture_id": id}); r.IsError {
		t.Fatalf("delete: %+v", r)
	}
}
func TestHistoryDedupAndResetConfig(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	path := filepath.Join(a.Config.RecordingsDir, "recording.json")
	os.WriteFile(path, []byte(`[{"host":"example.test","path":"/","request":{},"response":{"status":200}}]`), 0600)
	c1, err := a.Import(ctx, path, "")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := a.Import(ctx, path, "")
	if err != nil || c1.ID != c2.ID {
		t.Fatalf("duplicate imports: %s %s %v", c1.ID, c2.ID, err)
	}
	list, err := a.ListRecordings(20, 0)
	if err != nil || list["total"] != 1 {
		t.Fatalf("recordings: %+v %v", list, err)
	}
	cfg := filepath.Join(t.TempDir(), "charles.config")
	os.WriteFile(cfg, []byte("original"), 0600)
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err = a.Reset(ctx, Args{Action: "backup_config", ConfigPath: cfg, BackupPath: backup}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(cfg, []byte("changed"), 0600)
	if _, err = a.Reset(ctx, Args{Action: "restore_config", ConfigPath: cfg, BackupPath: backup}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cfg)
	if string(b) != "original" {
		t.Fatal("config was not restored")
	}
	if _, err = a.Reset(ctx, Args{}); err == nil {
		t.Fatal("implicit reset accepted")
	}
}
func TestConfigAndQueryValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"timeout_seconds":0}`), 0600)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("invalid timeout accepted")
	}
	os.WriteFile(path, []byte(`{"user":"from-file"}`), 0600)
	t.Setenv("CHARLES_USER", "from-env")
	cfg, err := LoadConfig(path)
	if err != nil || cfg.User != "from-env" {
		t.Fatalf("env precedence: %+v %v", cfg, err)
	}
}
