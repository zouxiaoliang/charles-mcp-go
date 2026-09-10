package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeCharles struct {
	mu            sync.Mutex
	recording     bool
	snapshot      string
	starts, stops int
	server        *httptest.Server
}

func newFakeCharles(t *testing.T) *fakeCharles {
	t.Helper()
	f := &fakeCharles{snapshot: xmlFixture(5, "one")}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/recording/":
			if f.recording {
				fmt.Fprint(w, "Status: Recording")
			} else {
				fmt.Fprint(w, "Status: Recording Stopped")
			}
		case "/recording/start":
			f.recording = true
			f.starts++
		case "/recording/stop":
			f.recording = false
			f.stops++
		case "/session/export-xml":
			fmt.Fprint(w, f.snapshot)
		case "/session/clear":
			f.snapshot = xmlFixture(0, "")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}
func TestLivePaginationUpdatesResetAndOwnership(t *testing.T) {
	ctx := context.Background()
	f := newFakeCharles(t)
	c, err := NewCharles(f.server.URL, "", "", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s := testStore(t)
	l := NewLiveManager(c, s, time.Minute)
	first, err := l.Start(ctx, "xml", false, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := l.Start(ctx, "xml", false, true)
	if err != nil {
		t.Fatal(err)
	}
	peek, err := l.Read(ctx, first.ID, Query{}, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	read, err := l.Read(ctx, first.ID, Query{}, 2, true)
	if err != nil || len(read.Entries) != 2 || read.Entries[0]["entry_id"] != peek.Entries[0]["entry_id"] {
		t.Fatalf("peek moved cursor: %+v %+v %v", peek, read, err)
	}
	ids := []string{}
	for _, e := range read.Entries {
		ids = append(ids, e["entry_id"].(string))
	}
	for read.HasMore {
		read, err = l.Read(ctx, first.ID, Query{}, 2, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range read.Entries {
			ids = append(ids, e["entry_id"].(string))
		}
	}
	if len(ids) != 5 {
		t.Fatalf("lost entries: %v", ids)
	}
	read, err = l.Read(ctx, first.ID, Query{}, 2, true)
	if err != nil || len(read.Entries) != 0 {
		t.Fatalf("duplicate snapshot: %+v %v", read, err)
	}
	f.mu.Lock()
	f.snapshot = xmlFixture(5, "two")
	f.mu.Unlock()
	read, err = l.Read(ctx, first.ID, Query{}, 10, true)
	if err != nil || len(read.Entries) != 5 || read.Entries[0]["entry_id"] != ids[0] {
		t.Fatalf("response update identity: %+v %v", read, err)
	}
	stored, _ := s.Entries(ctx, first.CaptureID)
	if len(stored) != 5 {
		t.Fatalf("updates duplicated storage: %d", len(stored))
	}
	f.mu.Lock()
	f.snapshot = xmlFixture(1, "after-clear")
	f.mu.Unlock()
	read, err = l.Read(ctx, first.ID, Query{}, 10, true)
	if err != nil || len(read.Entries) != 1 || read.Entries[0]["entry_id"] == ids[0] {
		t.Fatalf("reset: %+v %v", read, err)
	}
	if _, err = l.Stop(ctx, first.ID, true); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	stops := f.stops
	f.mu.Unlock()
	if stops != 0 {
		t.Fatal("stopped recording while second session active")
	}
	if _, err = l.Stop(ctx, second.ID, true); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.starts != 1 || f.stops != 1 || f.recording {
		t.Fatalf("recording ownership: %+v", f)
	}
}
func TestLiveTTLAndPreexistingRecording(t *testing.T) {
	ctx := context.Background()
	f := newFakeCharles(t)
	c, _ := NewCharles(f.server.URL, "", "", "", time.Second)
	defer c.Close()
	s := testStore(t)
	l := NewLiveManager(c, s, time.Hour)
	session, err := l.Start(ctx, "xml", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.Expire(ctx); err != nil {
		t.Fatal(err)
	}
	if status := l.Status(); len(status) != 1 || !status[0].Active {
		t.Fatal("unexpired session removed or stopped")
	}
	// Age the session explicitly; elapsed nanoseconds depend on OS clock resolution.
	l.mu.Lock()
	l.sessions[session.ID].Updated = time.Now().Add(-2 * l.ttl)
	l.mu.Unlock()
	if err = l.Expire(ctx); err != nil {
		t.Fatal(err)
	}
	if len(l.Status()) != 0 {
		t.Fatal("expired session retained")
	}
	f.mu.Lock()
	if f.recording {
		t.Fatal("TTL did not restore recording")
	}
	f.recording = true
	before := f.stops
	f.mu.Unlock()
	session, err = l.Start(ctx, "xml", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.Stop(ctx, session.ID, true); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.recording || f.stops != before {
		t.Fatal("changed user-owned recording")
	}
}
func TestReplayActuallySendsMutationsAndStoresFailures(t *testing.T) {
	ctx := context.Background()
	var received atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		if r.Method != "POST" || r.URL.Query().Has("remove") || strings.Join(r.URL.Query()["multi"], ",") != "x,y" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Remove") != "" {
			t.Errorf("bad outgoing request: %s %s %v", r.Method, r.URL, r.Header)
		}
		var v map[string]any
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			t.Error(err)
		}
		if v["user"].(map[string]any)["name"] != "bob" {
			t.Error(v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":"denied"}`)
	}))
	defer target.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	s := testStore(t)
	e := testEntry("replay")
	e.URL = target.URL + "/api?remove=1"
	e.Request.Headers.Set("X-Remove", "bad")
	saveEntries(t, s, e)
	c, _ := NewCharles(target.URL, "", "", "", time.Second)
	defer c.Close()
	result, err := Replay(ctx, s, c, e.ID, ReplayOptions{Query: map[string]any{"remove": nil, "multi": []any{"x", "y"}}, Headers: map[string]*string{"x-remove": nil}, JSON: map[string]any{"/user/name": "bob"}})
	if err != nil {
		t.Fatal(err)
	}
	if received.Load() != 1 || result.Status != 401 || result.ExecutionStatus != "completed" || !result.StatusChanged || result.ExperimentID == "" {
		t.Fatalf("replay result: %+v", result)
	}
	e.ID = "failed"
	e.URL = "http://127.0.0.1:1/"
	saveEntries(t, s, e)
	result, err = Replay(ctx, s, c, e.ID, ReplayOptions{})
	if err != nil || result.ExecutionStatus != "failed" || result.ExperimentID == "" {
		t.Fatalf("failure not persisted: %+v %v", result, err)
	}
	experiments, err := s.Artifacts(ctx, "experiment", "", 20, 0)
	if err != nil || len(experiments) != 2 {
		t.Fatalf("experiments: %+v %v", experiments, err)
	}
}
func TestReplayFormAndExplicitProxy(t *testing.T) {
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Host != "target.invalid:8080" {
			t.Error(r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != "a=2&a=3" {
			t.Errorf("form %s", b)
		}
		fmt.Fprint(w, "ok")
	}))
	defer proxy.Close()
	s := testStore(t)
	e := testEntry("form")
	e.URL = "http://target.invalid:8080/form"
	e.Request.Headers.Set("Content-Type", "application/x-www-form-urlencoded")
	e.Request.Body.Data = []byte("a=1&remove=yes")
	saveEntries(t, s, e)
	c, _ := NewCharles("http://control.charles", proxy.URL, "", "", time.Second)
	defer c.Close()
	r, err := Replay(context.Background(), s, c, e.ID, ReplayOptions{UseProxy: true, Form: map[string]any{"a": []any{2, 3}, "remove": nil}})
	if err != nil || r.Status != 200 || calls.Load() != 1 {
		t.Fatalf("proxy: %+v %v", r, err)
	}
}
func TestWorkflowsArePassiveAndProduceExecutableRecipes(t *testing.T) {
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "ok") }))
	defer target.Close()
	s := testStore(t)
	c, _ := NewCharles(target.URL, "", "", "", time.Second)
	defer c.Close()
	l := NewLiveManager(c, s, time.Minute)
	a, b, d := testEntry("login"), testEntry("api"), testEntry("signature")
	a.URL = target.URL + "/auth/login"
	b.URL = target.URL + "/api/orders"
	b.Request.Body.Data = []byte(`{"order_id":123}`)
	d.URL = target.URL + "/sign/verify?sign=bbbbbbbb&ts=2"
	d.Status = 403
	d.Request.Body.Data = []byte(`{"signature":"bbbbbbbb"}`)
	saveEntries(t, s, a, b, d)
	for _, kind := range []string{"login", "api", "signature"} {
		r, err := Workflow(context.Background(), s, l, c, WorkflowOptions{Kind: kind, CaptureID: "cap_test"})
		if err != nil {
			t.Fatal(err)
		}
		if r["status"] != "ok" || r["replay_executed"] != false || r["selected_entry"].(map[string]any)["entry_id"] != kind {
			t.Fatalf("%s: %+v", kind, r)
		}
		plan := r["mutation_plan"].(map[string]any)
		if len(plan["variants"].([]MutationVariant)) == 0 {
			t.Fatal("no recipes")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("passive workflow sent a request")
	}
	r, err := Workflow(context.Background(), s, l, c, WorkflowOptions{Kind: "api", CaptureID: "cap_test", RunReplay: true})
	if err != nil || r["replay_executed"] != true || calls.Load() != 1 {
		t.Fatalf("explicit replay: %+v %v", r, err)
	}
}
