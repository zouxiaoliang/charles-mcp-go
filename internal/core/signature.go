package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type Field struct {
	Surface string   `json:"surface"`
	Name    string   `json:"field"`
	Values  []string `json:"values"`
}

func requestFields(e Entry) []Field {
	fields := []Field{}
	u, _ := url.Parse(e.URL)
	if u != nil {
		for k, v := range u.Query() {
			fields = append(fields, Field{"query", k, v})
		}
	}
	for k, v := range e.Request.Headers {
		fields = append(fields, Field{"header", k, v})
	}
	b, _, _ := bodyBytes(e.Request)
	ct := e.Request.Headers.Get("Content-Type")
	if strings.Contains(ct, "x-www-form-urlencoded") {
		v, _ := url.ParseQuery(string(b))
		for k, vs := range v {
			fields = append(fields, Field{"form", k, vs})
		}
	}
	var obj map[string]any
	if json.Unmarshal(b, &obj) == nil {
		var walk func(map[string]any, string)
		walk = func(m map[string]any, path string) {
			for k, v := range m {
				p := path + "/" + strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
				if sub, ok := v.(map[string]any); ok {
					walk(sub, p)
				} else {
					raw, _ := json.Marshal(v)
					fields = append(fields, Field{"json", p, []string{string(raw)}})
				}
			}
		}
		walk(obj, "")
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Surface+fields[i].Name < fields[j].Surface+fields[j].Name })
	return fields
}
func signatureName(s string) bool {
	for _, hint := range []string{"sign", "sig", "token", "nonce", "timestamp", "auth"} {
		if contains(s, hint) {
			return true
		}
	}
	return strings.EqualFold(s, "ts") || strings.HasSuffix(s, "/ts")
}

type SignatureCandidate struct {
	Surface  string   `json:"surface"`
	Field    string   `json:"field"`
	Score    float64  `json:"score"`
	Distinct []string `json:"distinct_values"`
	Observed int      `json:"observed_count"`
}

func Discover(entries []Entry) ([]SignatureCandidate, error) {
	if len(entries) < 2 {
		return nil, fmt.Errorf("signature comparison requires at least two distinct entries")
	}
	ids := map[string]bool{}
	fields := map[string][]string{}
	definitions := map[string]Field{}
	for _, e := range entries {
		if ids[e.ID] {
			return nil, fmt.Errorf("duplicate entry_id %q", e.ID)
		}
		ids[e.ID] = true
		for _, f := range requestFields(e) {
			k := f.Surface + ":" + f.Name
			definitions[k] = f
			b, _ := json.Marshal(f.Values)
			fields[k] = append(fields[k], string(b))
		}
	}
	out := []SignatureCandidate{}
	for k, values := range fields {
		unique := map[string]bool{}
		long := true
		for _, v := range values {
			unique[v] = true
			if len(v) < 12 {
				long = false
			}
		}
		if len(unique) < 2 {
			continue
		}
		score := 0.4
		f := definitions[k]
		if signatureName(f.Name) {
			score += 0.35
		}
		if long {
			score += 0.15
		}
		if len(unique) == len(entries) {
			score += 0.1
		}
		distinct := []string{}
		for v := range unique {
			distinct = append(distinct, v)
		}
		sort.Strings(distinct)
		out = append(out, SignatureCandidate{f.Surface, f.Name, min(score, 0.99), distinct[:min(5, len(distinct))], len(values)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Surface+out[i].Field < out[j].Surface+out[j].Field
		}
		return out[i].Score > out[j].Score
	})
	return out, nil
}
func (s *Store) Discover(ctx context.Context, ids []string) (map[string]any, error) {
	entries := []Entry{}
	for _, id := range ids {
		e, err := s.Entry(ctx, id)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	candidates, err := Discover(entries)
	if err != nil {
		return nil, err
	}
	for _, c := range candidates[:min(5, len(candidates))] {
		if _, err = s.SaveArtifact(ctx, "finding", entries[0].CaptureID, map[string]any{"type": "signature_candidate", "candidate": c, "entry_ids": ids}); err != nil {
			return nil, err
		}
	}
	warnings := []string{"Heuristic candidates only; this does not identify or prove a signature algorithm."}
	for _, e := range entries[1:] {
		if e.Method != entries[0].Method || e.Host != entries[0].Host || e.Path != entries[0].Path {
			warnings = append(warnings, "Mixed routes compared; narrow to one method and route to reduce false positives.")
			break
		}
	}
	return map[string]any{"candidates": candidates, "warnings": warnings}, nil
}
