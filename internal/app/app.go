package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zouxiaoliang/charles-mcp-go/internal/core"
)

type App struct {
	Config  Config
	Store   *core.Store
	Charles *core.Charles
	Live    *core.LiveManager
}

func New(c Config) (*App, error) {
	if err := os.MkdirAll(c.RecordingsDir, 0700); err != nil {
		return nil, err
	}
	s, err := core.OpenStore(filepath.Join(c.DataDir, "captures.db"))
	if err != nil {
		return nil, err
	}
	charles, err := core.NewCharles(c.CharlesURL, c.ProxyURL, c.User, c.Password, time.Duration(c.TimeoutSeconds)*time.Second)
	if err != nil {
		s.Close()
		return nil, err
	}
	charles.CLI = c.CharlesCLI
	return &App{Config: c, Store: s, Charles: charles, Live: core.NewLiveManager(charles, s, time.Duration(c.SessionTTLSeconds)*time.Second)}, nil
}
func (a *App) Close(ctx context.Context) error {
	err := a.Live.Close(ctx)
	a.Charles.Close()
	dbErr := a.Store.Close()
	if err != nil {
		return err
	}
	return dbErr
}

type Args struct {
	CaptureID      string     `json:"capture_id,omitempty"`
	LiveID         string     `json:"live_session_id,omitempty"`
	EntryID        string     `json:"entry_id,omitempty"`
	EntryIDs       []string   `json:"entry_ids,omitempty"`
	Path           string     `json:"path,omitempty"`
	Format         string     `json:"source_format,omitempty"`
	SnapshotFormat string     `json:"snapshot_format,omitempty"`
	Query          core.Query `json:"query,omitempty"`
	Limit          int        `json:"limit,omitempty"`
	Offset         int        `json:"offset,omitempty"`
	GroupBy        string     `json:"group_by,omitempty"`
	MaxChars       int        `json:"max_chars,omitempty"`
	IncludeRaw     bool       `json:"include_raw,omitempty"`
	Side           string     `json:"side,omitempty"`
	Descriptor     string     `json:"descriptor_path,omitempty"`
	Message        string     `json:"message_type,omitempty"`
	core.ReplayOptions
	Reset            bool               `json:"reset_session,omitempty"`
	StartRecording   *bool              `json:"start_recording_if_stopped,omitempty"`
	RestoreRecording *bool              `json:"restore_recording,omitempty"`
	Preset           string             `json:"preset,omitempty"`
	Action           string             `json:"action,omitempty"`
	SubjectID        string             `json:"subject_id,omitempty"`
	ArtifactKind     string             `json:"artifact_kind,omitempty"`
	PathKeywords     []string           `json:"path_keywords,omitempty"`
	SignatureHints   []string           `json:"signature_hints,omitempty"`
	Advance          *bool              `json:"advance,omitempty"`
	DecodeBodies     *bool              `json:"decode_bodies,omitempty"`
	RunReplay        bool               `json:"run_replay,omitempty"`
	Replay           core.ReplayOptions `json:"replay,omitempty"`
	BackupPath       string             `json:"backup_path,omitempty"`
	ConfigPath       string             `json:"config_path,omitempty"`
	ProfilesPath     string             `json:"profiles_path,omitempty"`
	DurationSeconds  int                `json:"duration_seconds,omitempty"`
}

func normalize(p *Args) error {
	if p.Limit == 0 {
		p.Limit = 20
	}
	if p.Limit < 1 || p.Limit > 200 {
		return fmt.Errorf("limit must be 1..200")
	}
	if p.Offset < 0 {
		return fmt.Errorf("offset must be nonnegative")
	}
	if p.MaxChars == 0 {
		p.MaxChars = 2048
	}
	if p.MaxChars < 32 || p.MaxChars > 1048576 {
		return fmt.Errorf("max_chars must be 32..1048576")
	}
	return nil
}
func yes(v *bool) bool { return v == nil || *v }
func (a *App) entries(ctx context.Context, p Args) ([]core.Entry, error) {
	id := p.CaptureID
	if p.LiveID != "" {
		var err error
		id, err = a.Live.Capture(ctx, p.LiveID, true)
		if err != nil {
			return nil, err
		}
	} else if p.Path != "" {
		c, err := a.Import(ctx, p.Path, p.Format)
		if err != nil {
			return nil, err
		}
		id = c.ID
	}
	if id == "" {
		return nil, fmt.Errorf("provide capture_id, live_session_id or path")
	}
	return a.Store.Entries(ctx, id)
}
func (a *App) Import(ctx context.Context, path, format string) (core.Capture, error) {
	// Content-addressed imported captures make repeated queries of a file idempotent.
	c, entries, err := core.ImportFile(ctx, path, format, a.Config.CharlesCLI)
	if err != nil {
		return c, err
	}
	err = a.Store.SaveCapture(ctx, c, entries)
	return c, err
}
func (a *App) Call(ctx context.Context, name string, p Args) (any, error) {
	if err := normalize(&p); err != nil {
		return nil, err
	}
	switch name {
	case "charles_status", "reverse_charles_recording_status":
		recording, err := a.Charles.Recording(ctx)
		if err != nil {
			return map[string]any{"connected": false, "error": err.Error(), "sessions": a.Live.Status()}, nil
		}
		return map[string]any{"connected": true, "is_recording": recording, "sessions": a.Live.Status()}, nil
	case "start_live_capture", "reverse_start_live_analysis":
		return a.Live.Start(ctx, p.SnapshotFormat, p.Reset, yes(p.StartRecording))
	case "peek_live_capture", "reverse_peek_live_entries":
		return a.Live.Read(ctx, p.LiveID, p.Query, p.Limit, false)
	case "read_live_capture", "reverse_read_live_entries":
		return a.Live.Read(ctx, p.LiveID, p.Query, p.Limit, true)
	case "stop_live_capture", "reverse_stop_live_analysis":
		return a.Live.Stop(ctx, p.LiveID, yes(p.RestoreRecording))
	case "query_live_capture_entries", "query_recorded_traffic", "reverse_query_entries":
		entries, err := a.entries(ctx, p)
		if err != nil {
			return nil, err
		}
		return core.QueryEntries(entries, p.Query, p.Limit, p.Offset)
	case "get_recording_snapshot":
		entries, err := a.entries(ctx, p)
		if err != nil {
			return nil, err
		}
		return core.QueryEntries(entries, p.Query, p.Limit, p.Offset)
	case "analyze_recorded_traffic", "group_capture_analysis", "get_capture_analysis_stats":
		entries, err := a.entries(ctx, p)
		if err != nil {
			return nil, err
		}
		return core.Analyze(entries, p.Query, p.GroupBy)
	case "get_traffic_entry_detail", "reverse_get_entry_detail":
		e, err := a.Store.Entry(ctx, p.EntryID)
		if err != nil {
			return nil, err
		}
		return core.Detail(e, p.MaxChars, p.IncludeRaw), nil
	case "reverse_import_session":
		return a.Import(ctx, p.Path, p.Format)
	case "reverse_list_captures":
		return a.Store.ListCaptures(ctx, p.Limit, p.Offset)
	case "list_recordings":
		return a.ListRecordings(p.Limit, p.Offset)
	case "reverse_decode_entry_body":
		return a.Store.DecodeEntry(ctx, p.EntryID, p.Side, p.Descriptor, p.Message, p.MaxChars)
	case "reverse_replay_entry":
		return core.Replay(ctx, a.Store, a.Charles, p.EntryID, p.ReplayOptions)
	case "reverse_discover_signature_candidates":
		return a.Store.Discover(ctx, p.EntryIDs)
	case "reverse_list_findings":
		kind := p.ArtifactKind
		if kind == "" {
			kind = "finding"
		}
		return a.Store.Artifacts(ctx, kind, p.SubjectID, p.Limit, p.Offset)
	case "reverse_analyze_live_login_flow", "reverse_analyze_live_api_flow", "reverse_analyze_live_signature_flow":
		kind := strings.TrimSuffix(strings.TrimPrefix(name, "reverse_analyze_live_"), "_flow")
		return core.Workflow(ctx, a.Store, a.Live, a.Charles, core.WorkflowOptions{Kind: kind, LiveID: p.LiveID, CaptureID: p.CaptureID, Query: p.Query, PathKeywords: p.PathKeywords, SignatureHints: p.SignatureHints, Limit: p.Limit, Advance: p.Advance, DecodeBodies: p.DecodeBodies, Descriptor: p.Descriptor, Message: p.Message, RunReplay: p.RunReplay, Replay: p.Replay})
	case "throttling":
		err := a.Charles.Throttle(ctx, p.Preset)
		return map[string]any{"preset": p.Preset, "success": err == nil}, err
	case "reset_environment":
		return a.Reset(ctx, p)
	case "delete_capture":
		err := a.Live.DeleteCapture(ctx, p.CaptureID)
		return map[string]any{"capture_id": p.CaptureID, "deleted": err == nil}, err
	case "filter_func":
		return a.Call(ctx, "query_recorded_traffic", p)
	case "list_sessions":
		return a.Call(ctx, "reverse_list_captures", p)
	case "proxy_by_time":
		if p.DurationSeconds == 0 {
			p.DurationSeconds = 30
		}
		if p.DurationSeconds < 1 || p.DurationSeconds > 300 {
			return nil, fmt.Errorf("duration_seconds must be 1..300")
		}
		session, err := a.Live.Start(ctx, p.SnapshotFormat, p.Reset, yes(p.StartRecording))
		if err != nil {
			return nil, err
		}
		timer := time.NewTimer(time.Duration(p.DurationSeconds) * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		cleanup, cancel := context.WithTimeout(context.Background(), time.Duration(a.Config.TimeoutSeconds)*time.Second)
		defer cancel()
		stopped, err := a.Live.Stop(cleanup, session.ID, true)
		if ctx.Err() != nil {
			return stopped, ctx.Err()
		}
		return stopped, err
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}
func (a *App) ListRecordings(limit, offset int) (map[string]any, error) {
	files, err := os.ReadDir(a.Config.RecordingsDir)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(f.Name())) {
		case ".xml", ".chls", ".chlz", ".chlsj", ".json":
		default:
			continue
		}
		info, err := f.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"path": filepath.Join(a.Config.RecordingsDir, f.Name()), "size_bytes": info.Size(), "modified_at": info.ModTime().UTC().Format(time.RFC3339Nano)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["modified_at"].(string) > out[j]["modified_at"].(string) })
	total := len(out)
	out = out[min(offset, total):min(offset+limit, total)]
	return map[string]any{"recordings": out, "total": total, "has_more": offset+len(out) < total}, nil
}
