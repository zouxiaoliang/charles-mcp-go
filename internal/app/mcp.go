package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zouxiaoliang/charles-mcp-go/internal/core"
)

type ToolSpec struct {
	Name, Description, Fields, Required string
	ReadOnly                            bool
}

// Keep the familiar 31 tool names, but use one ID model and a nested query object.
var ToolSpecs = []ToolSpec{
	{"start_live_capture", "Start a live session; returns live_session_id and persistent capture_id. Existing Charles traffic is preserved unless reset_session is true.", "snapshot_format reset_session start_recording_if_stopped", "", false},
	{"read_live_capture", "Read new or updated entries and advance the cursor only through this page. Filters consume nonmatching entries. Reuse live_session_id.", "live_session_id query limit", "live_session_id", false},
	{"peek_live_capture", "Inspect new or updated entries without moving the live cursor.", "live_session_id query limit", "live_session_id", false},
	{"stop_live_capture", "Stop a live session. Restore recording if this server started it and no other sessions need it. Captured data remains queryable.", "live_session_id restore_recording", "live_session_id", false},
	{"query_live_capture_entries", "Query the full live capture without consuming its incremental cursor.", "live_session_id query limit offset", "live_session_id", false},
	{"list_recordings", "List saved XML, native and JSON files in recordings_dir.", "limit offset", "", true},
	{"get_recording_snapshot", "Import a recording path or inspect an existing capture_id; returns a paginated summary.", "path source_format capture_id query limit offset", "", false},
	{"query_recorded_traffic", "Filter recorded traffic by route, method, status, class, headers, decoded body text and JMESPath. Provide capture_id or path.", "capture_id path source_format query limit offset", "", false},
	{"analyze_recorded_traffic", "Analyze recorded traffic: groups, status/class counts, body bytes, errors and latency. Provide capture_id or path.", "capture_id path source_format query group_by", "", false},
	{"group_capture_analysis", "Group filtered entries by host, path, route, method, status, resource_class or content_type.", "capture_id live_session_id path source_format query group_by", "", false},
	{"get_capture_analysis_stats", "Compute traffic counts, errors, sizes and latency for a capture or live session.", "capture_id live_session_id path source_format query group_by", "", false},
	{"get_traffic_entry_detail", "Expand one entry. Headers are preserved; body preview is bounded. include_raw returns stored base64 bytes.", "entry_id max_chars include_raw", "entry_id", true},
	{"charles_status", "Check Charles connectivity, recording state and local live sessions. Does not change Charles.", "", "", true},
	{"throttling", "Set Charles throttling: 3g, 4g, 5g, fibre, 56k, 256k, custom preset name, or off.", "preset", "preset", false},
	{"reset_environment", "Explicit control action only: clear_session, quit_charles, start_recording, stop_recording, backup_config, restore_config. Stop live sessions first; restore requires Charles closed.", "action config_path backup_path profiles_path", "action", false},
	{"reverse_import_session", "Import Charles XML, native chls/chlz, or JSON/chlsj. Auto-detect by extension. Native fallback uses Charles CLI. Returns capture_id.", "path source_format", "path", false},
	{"reverse_list_captures", "List persisted captures, including previous live sessions, with pagination.", "limit offset", "", true},
	{"reverse_query_entries", "Find candidate entries in a stored capture; use entry_id to expand, decode or replay.", "capture_id query limit offset", "capture_id", true},
	{"reverse_get_entry_detail", "Expand a stored entry; same ID and detail model as live captures.", "entry_id max_chars include_raw", "entry_id", true},
	{"reverse_decode_entry_body", "Decode request or response: gzip, Brotli, zstd, deflate, charset, JSON, form, multipart, protobuf. Protobuf uses binary FileDescriptorSet and fully-qualified message_type.", "entry_id side descriptor_path message_type max_chars", "entry_id side", false},
	{"reverse_replay_entry", "SEND a stored request to its original HTTP(S) destination with optional mutations. Preserves cookies/auth. null deletes fields. JSON overrides accept top-level keys or JSON Pointers. Only one body mutation kind per call. Persists experiment and comparison.", "entry_id query_overrides header_overrides json_overrides form_overrides body_text_override follow_redirects use_proxy", "entry_id", false},
	{"reverse_discover_signature_candidates", "Compare at least two distinct requests and rank changing signature-like fields. Heuristics only; findings are persisted.", "entry_ids", "entry_ids", false},
	{"reverse_list_findings", "List persisted findings. artifact_kind can also be decoded, experiment, or workflow; filter by subject_id and paginate.", "artifact_kind subject_id limit offset", "", true},
	{"reverse_start_live_analysis", "Start live analysis using the shared live capture model. Defaults to official XML export.", "snapshot_format reset_session start_recording_if_stopped", "", false},
	{"reverse_peek_live_entries", "Peek incremental live entries without advancing the cursor.", "live_session_id query limit", "live_session_id", false},
	{"reverse_read_live_entries", "Read incremental live entries with lossless pagination; updates to completed responses retain entry_id.", "live_session_id query limit", "live_session_id", false},
	{"reverse_stop_live_analysis", "Stop live analysis and optionally restore the recording state this server changed.", "live_session_id restore_recording", "live_session_id", false},
	{"reverse_charles_recording_status", "Check recording state and all live sessions without changing them.", "", "", true},
	{"reverse_analyze_live_login_flow", "Rank login/auth candidates, decode selected evidence, discover fields and propose mutation recipes. No requests are sent unless run_replay=true. Provide live_session_id or capture_id.", "live_session_id capture_id query path_keywords limit advance decode_bodies descriptor_path message_type run_replay replay", "", false},
	{"reverse_analyze_live_api_flow", "Rank API candidates and prioritize business-field mutation recipes. No requests are sent unless run_replay=true. Provide live_session_id or capture_id.", "live_session_id capture_id query path_keywords limit advance decode_bodies descriptor_path message_type run_replay replay", "", false},
	{"reverse_analyze_live_signature_flow", "Rank signed-request candidates, compare changing fields and propose signature mutation recipes. No requests are sent unless run_replay=true. Provide live_session_id or capture_id.", "live_session_id capture_id query path_keywords signature_hints limit advance decode_bodies descriptor_path message_type run_replay replay", "", false},
	{"delete_capture", "Delete one capture and its entries and related artifacts. Active live captures must be stopped first.", "capture_id", "capture_id", false},
}

func specs(legacy bool) []ToolSpec {
	out := append([]ToolSpec{}, ToolSpecs...)
	if legacy {
		out = append(out, ToolSpec{"filter_func", "Deprecated alias for query_recorded_traffic.", "capture_id path source_format query limit offset", "", false}, ToolSpec{"list_sessions", "Deprecated alias for reverse_list_captures.", "limit offset", "", true}, ToolSpec{"proxy_by_time", "Capture for duration_seconds (default 30, max 300), save the capture and restore recording. Returns capture_id.", "duration_seconds snapshot_format reset_session start_recording_if_stopped", "", false})
	}
	return out
}
func ToolDefinitions(legacy bool) ([]*mcp.Tool, error) {
	tools := []*mcp.Tool{}
	for _, spec := range specs(legacy) {
		full, err := jsonschema.For[Args](nil)
		if err != nil {
			return nil, err
		}
		props := map[string]*jsonschema.Schema{}
		replaySchema, err := jsonschema.For[core.ReplayOptions](nil)
		if err != nil {
			return nil, err
		}
		for _, key := range strings.Fields(spec.Fields) {
			v := full.Properties[key]
			if v == nil {
				v = replaySchema.Properties[key]
			}
			if v == nil {
				return nil, fmt.Errorf("missing schema property %s", key)
			}
			props[key] = v
		}
		full.Properties = props
		full.Required = strings.Fields(spec.Required)
		full.AdditionalProperties = &jsonschema.Schema{Not: &jsonschema.Schema{}}
		for _, key := range full.Required {
			if p := props[key]; p != nil && p.Type == "string" {
				n := 1
				p.MinLength = &n
			}
		}
		for key, bounds := range map[string][2]float64{"limit": {1, 200}, "offset": {0, 1e9}, "max_chars": {32, 1048576}} {
			if p := props[key]; p != nil {
				low, high := bounds[0], bounds[1]
				p.Minimum = &low
				p.Maximum = &high
			}
		}
		for key, values := range map[string][]any{"side": {"request", "response"}, "source_format": {"auto", "xml", "native", "json", "legacy_json"}, "snapshot_format": {"xml", "native", "json"}, "artifact_kind": {"finding", "decoded", "experiment", "workflow"}, "group_by": {"host", "path", "route", "method", "status", "resource_class", "content_type"}} {
			if p := props[key]; p != nil {
				p.Enum = values
			}
		}
		tools = append(tools, &mcp.Tool{Name: spec.Name, Description: spec.Description, InputSchema: full, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: spec.ReadOnly}})
	}
	return tools, nil
}
func (a *App) Server(version string) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "charles-mcp-go", Version: version}, &mcp.ServerOptions{Instructions: "Start with charles_status and start_live_capture, or reverse_import_session. Query summaries, then expand selected entry_id. All capture tools share one persistent data model. Replay tools send real HTTP requests; analysis is passive unless run_replay is explicitly true."})
	defs, err := ToolDefinitions(a.Config.LegacyAliases)
	if err != nil {
		return nil, err
	}
	for _, tool := range defs {
		schema := tool.InputSchema.(*jsonschema.Schema)
		resolved, err := schema.Resolve(nil)
		if err != nil {
			return nil, err
		}
		name := tool.Name
		server.AddTool(tool, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			fail := func(err error) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
			}
			raw := request.Params.Arguments
			if len(raw) == 0 {
				raw = []byte("{}")
			}
			var data any
			if err := json.Unmarshal(raw, &data); err != nil {
				return fail(err)
			}
			if err := resolved.Validate(data); err != nil {
				return fail(fmt.Errorf("invalid arguments: %w", err))
			}
			var p Args
			if err := json.Unmarshal(raw, &p); err != nil {
				return fail(err)
			}
			result, err := a.Call(ctx, name, p)
			if err != nil {
				return fail(err)
			}
			wrapped := map[string]any{"result": result}
			b, err := json.Marshal(wrapped)
			if err != nil {
				return fail(err)
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}, StructuredContent: wrapped}, nil
		})
	}
	return server, nil
}
