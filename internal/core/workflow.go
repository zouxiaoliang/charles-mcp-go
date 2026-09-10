package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type WorkflowOptions struct {
	Kind           string        `json:"kind"`
	LiveID         string        `json:"live_session_id,omitempty"`
	CaptureID      string        `json:"capture_id,omitempty"`
	Query          Query         `json:"query,omitempty"`
	PathKeywords   []string      `json:"path_keywords,omitempty"`
	SignatureHints []string      `json:"signature_hints,omitempty"`
	Limit          int           `json:"limit,omitempty"`
	Advance        *bool         `json:"advance,omitempty"`
	DecodeBodies   *bool         `json:"decode_bodies,omitempty"`
	Descriptor     string        `json:"descriptor_path,omitempty"`
	Message        string        `json:"message_type,omitempty"`
	RunReplay      bool          `json:"run_replay,omitempty"`
	Replay         ReplayOptions `json:"replay,omitempty"`
}
type FlowCandidate struct {
	Entry   map[string]any `json:"entry"`
	Score   int            `json:"score"`
	Reasons []string       `json:"reasons"`
	entry   Entry
}

func enabled(v *bool) bool { return v == nil || *v }
func anyContains(s string, words []string) bool {
	for _, w := range words {
		if w != "" && contains(s, w) {
			return true
		}
	}
	return false
}
func scoreEntry(e Entry, o WorkflowOptions) FlowCandidate {
	c := FlowCandidate{Entry: Summary(e), Reasons: []string{}, entry: e}
	add := func(n int, r string) { c.Score += n; c.Reasons = append(c.Reasons, r) }
	keywords := o.PathKeywords
	if len(keywords) == 0 {
		switch o.Kind {
		case "login":
			keywords = []string{"login", "signin", "sign-in", "auth", "oauth", "token", "session"}
		case "api":
			keywords = []string{"api", "graphql", "rpc", "v1", "v2"}
		case "signature":
			keywords = []string{"sign", "verify", "auth", "token"}
		}
	}
	switch o.Kind {
	case "login":
		if anyContains(e.Path, keywords) {
			add(5, "login/auth route")
		}
		if e.Method == "POST" {
			add(3, "POST request")
		}
		if e.Status == 200 || e.Status == 201 || e.Status == 302 || e.Status == 401 || e.Status == 403 {
			add(2, "authentication-relevant status")
		}
		if len(e.Request.Body.Data) > 0 {
			add(1, "request body")
		}
	case "api":
		if anyContains(e.Method, []string{"POST", "PUT", "PATCH", "DELETE"}) {
			add(4, "state-changing API method")
		} else if e.Method == "GET" {
			add(1, "GET request")
		}
		if anyContains(e.Path, keywords) {
			add(3, "API route")
		}
		if e.Status >= 200 && e.Status < 300 {
			add(3, "successful response")
		} else if e.Status >= 400 && e.Status < 500 {
			add(1, "client error response")
		}
		if len(e.Request.Body.Data) > 0 {
			add(2, "request body")
		}
		if len(e.Response.Body.Data) > 0 {
			add(1, "response body")
		}
		if anyContains(e.Path, []string{"sign", "login", "auth"}) {
			add(-3, "authentication-specific route")
		}
	case "signature":
		if anyContains(e.Method, []string{"POST", "PUT", "PATCH"}) {
			add(4, "signed-method candidate")
		}
		if anyContains(e.Path, keywords) {
			add(2, "workflow route")
		}
		hints := o.SignatureHints
		if len(hints) == 0 {
			hints = []string{"sign", "verify", "nonce", "token"}
		}
		if anyContains(e.Path, hints) {
			add(3, "signature route hint")
		}
		if e.Status == 401 || e.Status == 403 {
			add(4, "authorization rejection")
		} else if e.Status == 200 || e.Status == 302 {
			add(1, "successful candidate")
		}
		if len(e.Request.Body.Data) > 0 {
			add(2, "request body")
		}
		for _, f := range requestFields(e) {
			if signatureName(f.Name) || anyContains(f.Name, hints) {
				add(3, "signature-like request field")
				break
			}
		}
	}
	return c
}

type MutationTarget struct {
	Surface  string   `json:"surface"`
	Field    string   `json:"field"`
	Priority int      `json:"priority"`
	Reason   string   `json:"reason"`
	Observed []string `json:"observed_values"`
	Expected string   `json:"expected_signal"`
}
type MutationVariant struct {
	ID        string        `json:"variant_id"`
	Target    string        `json:"target"`
	Operation string        `json:"operation"`
	Recipe    ReplayOptions `json:"recipe"`
}

func mutationPlan(e Entry, kind string) map[string]any {
	targets := []MutationTarget{}
	for _, f := range requestFields(e) {
		if f.Surface == "header" && !signatureName(f.Name) && !strings.EqualFold(f.Name, "Cookie") {
			continue
		}
		priority := 40
		reason := "business field"
		if signatureName(f.Name) {
			reason = "signature/authentication candidate"
			if kind == "signature" || kind == "login" {
				priority = 100
			} else {
				priority = 20
			}
		}
		if f.Surface == "header" {
			priority -= 5
		}
		targets = append(targets, MutationTarget{f.Surface, f.Name, priority, reason, f.Values, "Compare status, response body and authentication outcome against baseline."})
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Priority == targets[j].Priority {
			return targets[i].Surface+targets[i].Field < targets[j].Surface+targets[j].Field
		}
		return targets[i].Priority > targets[j].Priority
	})
	n := 4
	if kind == "login" {
		n = 3
	}
	targets = targets[:min(n, len(targets))]
	variants := []MutationVariant{}
	order := []string{}
	for _, t := range targets {
		for _, op := range []string{"remove", "empty", "tamper"} {
			var value any
			switch op {
			case "empty":
				value = ""
			case "tamper":
				value = "invalid-test-value"
			}
			recipe := ReplayOptions{}
			switch t.Surface {
			case "query":
				recipe.Query = map[string]any{t.Field: value}
			case "json":
				recipe.JSON = map[string]any{t.Field: value}
			case "form":
				recipe.Form = map[string]any{t.Field: value}
			case "header":
				recipe.Headers = map[string]*string{t.Field: nil}
				if value != nil {
					s := fmt.Sprint(value)
					recipe.Headers[t.Field] = &s
				}
			}
			id := fmt.Sprintf("variant_%d", len(variants)+1)
			variants = append(variants, MutationVariant{id, t.Surface + ":" + t.Field, op, recipe})
			order = append(order, id)
		}
	}
	if len(targets) == 0 && len(e.Request.Body.Data) > 0 {
		targets = append(targets, MutationTarget{"body", "text", 10, "opaque request body", nil, "Compare response to an empty request body."})
		empty := ""
		variants = append(variants, MutationVariant{"variant_1", "body:text", "empty", ReplayOptions{BodyText: &empty}})
		order = append(order, "variant_1")
	}
	maxVariants := 12
	if kind == "login" {
		maxVariants = 6
	} else if kind == "api" {
		maxVariants = 8
	}
	variants = variants[:min(len(variants), maxVariants)]
	order = order[:min(len(order), maxVariants)]
	return map[string]any{"targets": targets, "variants": variants, "execution_order": order, "executed": false}
}
func Workflow(ctx context.Context, s *Store, l *LiveManager, c *Charles, o WorkflowOptions) (map[string]any, error) {
	if o.Kind != "login" && o.Kind != "api" && o.Kind != "signature" {
		return nil, fmt.Errorf("workflow kind must be login, api or signature")
	}
	if o.Limit == 0 {
		o.Limit = 20
	}
	if o.Limit < 1 || o.Limit > 200 {
		return nil, fmt.Errorf("limit must be 1..200")
	}
	var entries []Entry
	var window QueryResult
	var err error
	if o.LiveID != "" {
		window, err = l.Read(ctx, o.LiveID, o.Query, o.Limit, enabled(o.Advance))
		if err != nil {
			return nil, err
		}
		for _, v := range window.Entries {
			e, err := s.Entry(ctx, v["entry_id"].(string))
			if err != nil {
				return nil, err
			}
			entries = append(entries, e)
		}
	} else if o.CaptureID != "" {
		all, err := s.Entries(ctx, o.CaptureID)
		if err != nil {
			return nil, err
		}
		window, err = QueryEntries(all, o.Query, o.Limit, 0)
		if err != nil {
			return nil, err
		}
		for _, v := range window.Entries {
			e, err := s.Entry(ctx, v["entry_id"].(string))
			if err != nil {
				return nil, err
			}
			entries = append(entries, e)
		}
	} else {
		return nil, fmt.Errorf("live_session_id or capture_id is required")
	}
	report := map[string]any{"kind": o.Kind, "status": "ok", "window": window, "replay_executed": false}
	if len(entries) == 0 {
		report["status"] = "no_new_entries"
		return report, nil
	}
	candidates := []FlowCandidate{}
	for _, e := range entries {
		candidate := scoreEntry(e, o)
		if candidate.Score > 0 {
			candidates = append(candidates, candidate)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].entry.Sequence > candidates[j].entry.Sequence
		}
		return candidates[i].Score > candidates[j].Score
	})
	report["candidates"] = candidates
	if len(candidates) == 0 {
		report["status"] = "no_" + o.Kind + "_candidates"
		return report, nil
	}
	selected := candidates[0].entry
	report["selected_entry"] = Detail(selected, 2048, false)
	if enabled(o.DecodeBodies) {
		decoded := map[string]any{}
		for _, side := range []string{"request", "response"} {
			d, err := s.DecodeEntry(ctx, selected.ID, side, o.Descriptor, o.Message, 2048)
			if err != nil {
				decoded[side] = map[string]any{"error": err.Error()}
			} else {
				decoded[side] = d
			}
		}
		report["decoded"] = decoded
	}
	if len(candidates) >= 2 {
		ids := []string{}
		for _, candidate := range candidates[:min(3, len(candidates))] {
			ids = append(ids, candidate.entry.ID)
		}
		discovery, err := s.Discover(ctx, ids)
		if err != nil {
			return nil, err
		}
		report["signature_candidates"] = discovery
	}
	report["mutation_plan"] = mutationPlan(selected, o.Kind)
	report["next_steps"] = []string{"Inspect selected evidence and candidate reasons.", "Use reverse_replay_entry with a mutation recipe when you intend to send a request.", "Compare findings; signature scores are hypotheses, not proof."}
	if o.RunReplay {
		r, err := Replay(ctx, s, c, selected.ID, o.Replay)
		if err != nil {
			return nil, err
		}
		report["replay"] = r
		report["replay_executed"] = true
	}
	findings, err := s.Artifacts(ctx, "finding", selected.ID, 20, 0)
	if err != nil {
		return nil, err
	}
	report["findings"] = findings
	artifact, err := s.SaveArtifact(ctx, "workflow", selected.ID, report)
	report["artifact_id"] = artifact
	return report, err
}
