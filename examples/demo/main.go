// Demo runs real MCP tool calls against a local HTTP fixture, without Charles.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type stage struct {
	Title string   `json:"title"`
	Tool  string   `json:"tool"`
	Lines []string `json:"lines"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	binary := flag.String("server", "./bin/charles-mcp", "path to the MCP executable")
	fixture := flag.String("fixture", "examples/session.json", "sample session JSON")
	transcript := flag.String("transcript", "", "optional JSON recording of verified results")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var requests atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/login":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid JSON", 400)
				return
			}
			if body["password"] != "demo-valid" {
				w.WriteHeader(401)
				fmt.Fprint(w, `{"error":"invalid_credentials"}`)
				return
			}
			fmt.Fprint(w, `{"ok":true,"token":"demo-token"}`)
		case "/api/products":
			fmt.Fprint(w, `{"products":[{"id":1,"name":"Demo product"}]}`)
		case "/api/orders":
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"missing_demo_token"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	dir, err := os.MkdirTemp("", "charles-mcp-demo-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	b, err := os.ReadFile(*fixture)
	if err != nil {
		return err
	}
	var entries []map[string]any
	if err := json.Unmarshal(b, &entries); err != nil {
		return err
	}
	// Produce the recording from actual loopback requests using the sample inputs.
	for _, entry := range entries {
		entry["url"] = backend.URL + entry["path"].(string)
		body := ""
		if v, ok := entry["request"].(map[string]any)["body"].(map[string]any); ok {
			body, _ = v["text"].(string)
		}
		req, err := http.NewRequestWithContext(ctx, entry["method"].(string), entry["url"].(string), strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := backend.Client().Do(req)
		if err != nil {
			return err
		}
		raw, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return err
		}
		response := entry["response"].(map[string]any)
		if res.StatusCode != int(response["status"].(float64)) {
			return fmt.Errorf("unexpected baseline status: %d", res.StatusCode)
		}
		response["body"] = map[string]any{"text": string(raw)}
	}
	b, err = json.Marshal(entries)
	if err != nil {
		return err
	}
	sessionPath := filepath.Join(dir, "demo-session.json")
	if err := os.WriteFile(sessionPath, b, 0600); err != nil {
		return err
	}
	cmd := exec.Command(*binary, "--data-dir", filepath.Join(dir, "data"))
	cmd.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "charles-mcp-demo", Version: "1"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return err
	}
	defer cs.Close()
	var stages []stage
	emit := func(title, tool string, lines ...string) {
		stages = append(stages, stage{title, tool, lines})
		fmt.Printf("\n%s\n> %s\n%s\n", title, tool, strings.Join(lines, "\n"))
	}
	var capture struct {
		ID string `json:"capture_id"`
	}
	if err := call(ctx, cs, "reverse_import_session", map[string]any{"path": sessionPath, "source_format": "json"}, &capture); err != nil {
		return err
	}
	if capture.ID == "" {
		return fmt.Errorf("missing capture ID")
	}
	var all struct {
		Total int `json:"total_entries"`
	}
	if err := call(ctx, cs, "reverse_query_entries", map[string]any{"capture_id": capture.ID}, &all); err != nil {
		return err
	}
	if all.Total != 3 {
		return fmt.Errorf("expected 3 imported requests, got %d", all.Total)
	}
	emit("01  IMPORT A RECORDING", "reverse_import_session", fmt.Sprintf("%d requests imported from demo-session.json", all.Total), "Actual HTTP responses from a local demo API")
	var found struct {
		Entries []struct {
			ID     string `json:"entry_id"`
			Status int    `json:"status"`
		} `json:"entries"`
	}
	if err := call(ctx, cs, "reverse_query_entries", map[string]any{"capture_id": capture.ID, "query": map[string]any{"preset": "errors_only", "path_contains": "login"}}, &found); err != nil {
		return err
	}
	if len(found.Entries) != 1 || found.Entries[0].Status != 401 {
		return fmt.Errorf("expected one failed login: %+v", found)
	}
	id := found.Entries[0].ID
	emit("02  FIND THE FAILED LOGIN", "reverse_query_entries", "Filter: errors_only + path contains login", fmt.Sprintf("POST /api/login  ->  %d", found.Entries[0].Status))
	var decoded map[string]any
	if err := call(ctx, cs, "reverse_decode_entry_body", map[string]any{"entry_id": id, "side": "response"}, &decoded); err != nil {
		return err
	}
	b, err = json.Marshal(decoded)
	if err != nil || !strings.Contains(string(b), "invalid_credentials") {
		return fmt.Errorf("expected invalid_credentials in decoded response: %s", b)
	}
	emit("03  INSPECT THE RESPONSE", "reverse_decode_entry_body", `{"error":"invalid_credentials"}`, "Demo API accepts the fixture password: demo-valid")
	var replay struct {
		Baseline int    `json:"baseline_status"`
		Status   int    `json:"status"`
		Changed  bool   `json:"status_changed"`
		ID       string `json:"experiment_id"`
	}
	if err := call(ctx, cs, "reverse_replay_entry", map[string]any{"entry_id": id, "json_overrides": map[string]any{"/password": "demo-valid"}, "use_proxy": false}, &replay); err != nil {
		return err
	}
	if replay.Baseline != 401 || replay.Status != 200 || !replay.Changed || replay.ID == "" || requests.Load() != 4 {
		return fmt.Errorf("unexpected replay: %+v, requests=%d", replay, requests.Load())
	}
	emit("04  REPLAY WITH ONE CHANGE", "reverse_replay_entry", `json_overrides: {"/password":"demo-valid"}`, fmt.Sprintf("Baseline %d  ->  Replay %d", replay.Baseline, replay.Status), "Real loopback request sent; comparison persisted")
	if *transcript != "" {
		b, err := json.MarshalIndent(stages, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(*transcript, append(b, '\n'), 0644)
	}
	return nil
}

func call(ctx context.Context, cs *mcp.ClientSession, name string, args map[string]any, out any) error {
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("%s failed: %+v", name, res.Content)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return err
	}
	var wrapper struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return err
	}
	return json.Unmarshal(wrapper.Result, out)
}
