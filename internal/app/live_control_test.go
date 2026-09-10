package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zouxiaoliang/charles-mcp-go/internal/core"
)

func TestLiveToolsControlAndWorkflowDispatch(t *testing.T) {
	var mu sync.Mutex
	recording := false
	throttle := ""
	clears := 0
	var address string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/recording") || strings.HasPrefix(r.URL.Path, "/session") || strings.HasPrefix(r.URL.Path, "/throttling") {
			user, pass, ok := r.BasicAuth()
			if !ok || user != "test" || pass != "secret" {
				http.Error(w, "auth", 401)
				return
			}
		}
		switch r.URL.Path {
		case "/recording/":
			if recording {
				fmt.Fprint(w, "Status: Recording")
			} else {
				fmt.Fprint(w, "Status: Recording Stopped")
			}
		case "/recording/start":
			recording = true
		case "/recording/stop":
			recording = false
		case "/session/clear":
			clears++
		case "/throttling/activate":
			throttle = r.URL.Query().Get("preset")
		case "/throttling/deactivate":
			throttle = "off"
		case "/session/export-xml":
			u, _ := url.Parse(address)
			fmt.Fprint(w, `<charles-session>`)
			for i, path := range []string{"/auth/login", "/api/orders", "/sign/verify"} {
				fmt.Fprintf(w, `<transaction method="POST" protocol="http" host="%s" port="%s" path="%s" query="sign=%d" startTimeMillis="%d"><request><headers><header><name>Content-Type</name><value>application/json</value></header></headers><body>{"name":"test","sign":"sig%d"}</body></request><response status="200"><body>{"ok":true}</body></response></transaction>`, u.Hostname(), u.Port(), path, i, i+1, i)
			}
			fmt.Fprint(w, `</charles-session>`)
		default:
			fmt.Fprint(w, `{"ok":true}`)
		}
	}))
	defer fake.Close()
	address = fake.URL
	dir := t.TempDir()
	a, err := New(Config{DataDir: dir, RecordingsDir: filepath.Join(dir, "recordings"), CharlesURL: fake.URL, User: "test", Password: "secret", TimeoutSeconds: 3, SessionTTLSeconds: 900})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	ctx := context.Background()
	call := func(name string, p Args) any {
		t.Helper()
		r, err := a.Call(ctx, name, p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return r
	}
	for _, name := range []string{"charles_status", "reverse_charles_recording_status"} {
		if !call(name, Args{}).(map[string]any)["connected"].(bool) {
			t.Fatal("not connected")
		}
	}
	for _, preset := range []string{"3g", "off", "custom"} {
		call("throttling", Args{Preset: preset})
		mu.Lock()
		got := throttle
		mu.Unlock()
		want := preset
		if preset == "3g" {
			want = "3G"
		}
		if got != want {
			t.Fatalf("throttle %q != %q", got, want)
		}
	}
	for _, name := range []string{"start_live_capture", "reverse_start_live_analysis"} {
		live := call(name, Args{Reset: true}).(core.LiveSession)
		if _, err = a.Call(ctx, "delete_capture", Args{CaptureID: live.CaptureID}); err == nil {
			t.Fatal("deleted active live capture")
		}
		for _, peek := range []string{"peek_live_capture", "reverse_peek_live_entries"} {
			r := call(peek, Args{LiveID: live.ID, Limit: 1}).(core.QueryResult)
			if len(r.Entries) != 1 || !r.HasMore {
				t.Fatalf("peek %+v", r)
			}
		}
		full := call("query_live_capture_entries", Args{LiveID: live.ID}).(core.QueryResult)
		if full.Matched != 3 {
			t.Fatal(full)
		}
		advance := false
		for _, workflow := range []string{"reverse_analyze_live_login_flow", "reverse_analyze_live_api_flow", "reverse_analyze_live_signature_flow"} {
			r := call(workflow, Args{LiveID: live.ID, Advance: &advance}).(map[string]any)
			if r["status"] != "ok" {
				t.Fatal(r)
			}
		}
		for _, read := range []string{"read_live_capture", "reverse_read_live_entries"} {
			call(read, Args{LiveID: live.ID, Limit: 1})
		}
		call("reverse_replay_entry", Args{EntryID: full.Entries[0]["entry_id"].(string)})
		call("reverse_discover_signature_candidates", Args{EntryIDs: []string{full.Entries[0]["entry_id"].(string), full.Entries[1]["entry_id"].(string)}})
		call("reverse_list_findings", Args{})
		stop := "stop_live_capture"
		if name == "reverse_start_live_analysis" {
			stop = "reverse_stop_live_analysis"
		}
		call(stop, Args{LiveID: live.ID})
		call("delete_capture", Args{CaptureID: live.CaptureID})
	}
	for _, action := range []string{"start_recording", "stop_recording", "clear_session"} {
		call("reset_environment", Args{Action: action})
	}
	mu.Lock()
	if recording || clears != 3 {
		t.Fatalf("control state recording=%v clears=%d", recording, clears)
	}
	mu.Unlock()
	call("proxy_by_time", Args{DurationSeconds: 1})
	call("list_sessions", Args{})
}
