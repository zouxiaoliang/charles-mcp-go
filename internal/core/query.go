package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jmespath/go-jmespath"
)

type Query struct {
	Preset              string   `json:"preset,omitempty"`
	Host                string   `json:"host_contains,omitempty"`
	Path                string   `json:"path_contains,omitempty"`
	Methods             []string `json:"method_in,omitempty"`
	Statuses            []int    `json:"status_in,omitempty"`
	HasError            *bool    `json:"has_error,omitempty"`
	Classes             []string `json:"resource_class_in,omitempty"`
	MinPriority         int      `json:"min_priority_score,omitempty"`
	RequestHeader       string   `json:"request_header_name,omitempty"`
	RequestHeaderValue  string   `json:"request_header_value_contains,omitempty"`
	ResponseHeader      string   `json:"response_header_name,omitempty"`
	ResponseHeaderValue string   `json:"response_header_value_contains,omitempty"`
	RequestType         string   `json:"request_content_type,omitempty"`
	ResponseType        string   `json:"response_content_type,omitempty"`
	RequestBody         string   `json:"request_body_contains,omitempty"`
	ResponseBody        string   `json:"response_body_contains,omitempty"`
	RequestJSON         string   `json:"request_json_query,omitempty"`
	ResponseJSON        string   `json:"response_json_query,omitempty"`
	MinSize             int      `json:"min_total_size,omitempty"`
	MaxSize             int      `json:"max_total_size,omitempty"`
	SinceSeconds        int      `json:"since_seconds,omitempty"`
}
type matcher struct {
	q        Query
	req, res *jmespath.JMESPath
	now      int64
}

func newMatcher(q Query) (matcher, error) {
	m := matcher{q: q, now: time.Now().UnixMilli()}
	if q.SinceSeconds < 0 || q.MinSize < 0 || q.MaxSize < 0 || q.MinPriority < 0 || q.MaxSize > 0 && q.MaxSize < q.MinSize {
		return m, fmt.Errorf("query sizes, priority and time window must be nonnegative; max size must not be smaller than min size")
	}
	var err error
	if q.RequestJSON != "" {
		m.req, err = jmespath.Compile(q.RequestJSON)
		if err != nil {
			return m, err
		}
	}
	if q.ResponseJSON != "" {
		m.res, err = jmespath.Compile(q.ResponseJSON)
		if err != nil {
			return m, err
		}
	}
	switch q.Preset {
	case "", "all_http", "api_focus", "errors_only", "page_bootstrap":
	default:
		return m, fmt.Errorf("unknown preset %q", q.Preset)
	}
	return m, nil
}
func contains(s, sub string) bool { return strings.Contains(strings.ToLower(s), strings.ToLower(sub)) }
func headerMatch(h http.Header, name, value string) bool {
	if name != "" {
		vs, ok := h[http.CanonicalHeaderKey(name)]
		if !ok {
			return false
		}
		return contains(strings.Join(vs, "\n"), value)
	}
	if value != "" {
		for _, vs := range h {
			if contains(strings.Join(vs, "\n"), value) {
				return true
			}
		}
		return false
	}
	return true
}
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	return true
}
func (m matcher) match(e Entry) bool {
	q := m.q
	hasError := e.Status >= 400 || e.Error != ""
	if !contains(e.Host, q.Host) || !contains(e.Path, q.Path) || q.HasError != nil && hasError != *q.HasError {
		return false
	}
	if len(q.Methods) > 0 && !slices.ContainsFunc(q.Methods, func(s string) bool { return strings.EqualFold(s, e.Method) }) {
		return false
	}
	if len(q.Statuses) > 0 && !slices.Contains(q.Statuses, e.Status) {
		return false
	}
	if len(q.Classes) > 0 && !slices.Contains(q.Classes, e.Class) || e.Priority < q.MinPriority {
		return false
	}
	switch q.Preset {
	case "errors_only":
		if !hasError {
			return false
		}
	case "api_focus":
		if e.Class != "api" && !hasError {
			return false
		}
	case "page_bootstrap":
		if !slices.Contains([]string{"api", "document", "script", "stylesheet"}, e.Class) {
			return false
		}
	}
	if q.SinceSeconds > 0 && e.StartMS < m.now-int64(q.SinceSeconds)*1000 {
		return false
	}
	size := e.TotalSize
	if size < int64(q.MinSize) || q.MaxSize > 0 && size > int64(q.MaxSize) {
		return false
	}
	if !headerMatch(e.Request.Headers, q.RequestHeader, q.RequestHeaderValue) || !headerMatch(e.Response.Headers, q.ResponseHeader, q.ResponseHeaderValue) {
		return false
	}
	if !contains(e.Request.Headers.Get("Content-Type"), q.RequestType) || !contains(e.Response.Headers.Get("Content-Type"), q.ResponseType) {
		return false
	}
	for _, s := range []struct {
		msg  Message
		body string
		j    *jmespath.JMESPath
	}{{e.Request, q.RequestBody, m.req}, {e.Response, q.ResponseBody, m.res}} {
		if s.body == "" && s.j == nil {
			continue
		}
		b, _, err := bodyBytes(s.msg)
		if err != nil {
			return false
		}
		text := bodyText(s.msg, b)
		if !contains(text, s.body) {
			return false
		}
		if s.j != nil {
			var data any
			if json.Unmarshal([]byte(text), &data) != nil {
				return false
			}
			v, err := s.j.Search(data)
			if err != nil || !truthy(v) {
				return false
			}
		}
	}
	return true
}

type QueryResult struct {
	Entries    []map[string]any `json:"entries"`
	Total      int              `json:"total_entries"`
	Matched    int              `json:"matched_entries"`
	NextOffset *int             `json:"next_offset"`
	Cursor     int              `json:"cursor,omitempty"`
	HasMore    bool             `json:"has_more"`
}

func Summary(e Entry) map[string]any {
	return map[string]any{"entry_id": e.ID, "capture_id": e.CaptureID, "sequence": e.Sequence, "method": e.Method, "url": e.URL, "status": e.Status, "start_ms": e.StartMS, "duration_ms": e.DurationMS, "resource_class": e.Class, "priority": e.Priority, "request_bytes": e.RequestBytes, "response_bytes": e.ResponseBytes, "total_size": e.TotalSize}
}
func QueryEntries(entries []Entry, q Query, limit, offset int) (QueryResult, error) {
	out := QueryResult{Entries: []map[string]any{}, Total: len(entries)}
	m, err := newMatcher(q)
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		if !m.match(e) {
			continue
		}
		if out.Matched >= offset && len(out.Entries) < limit {
			out.Entries = append(out.Entries, Summary(e))
		}
		out.Matched++
	}
	if offset+len(out.Entries) < out.Matched {
		n := offset + len(out.Entries)
		out.NextOffset = &n
		out.HasMore = true
	}
	return out, nil
}
func Detail(e Entry, maxChars int, raw bool) map[string]any {
	out := Summary(e)
	for side, msg := range map[string]Message{"request": e.Request, "response": e.Response} {
		b, w, decodeErr := bodyBytes(msg)
		if decodeErr != nil {
			w = append(w, decodeErr.Error())
		}
		text := bodyText(msg, b)
		body := map[string]any{"preservation": msg.Body.Preservation, "byte_length": len(msg.Body.Data), "preview": preview(text, maxChars), "truncated": len([]rune(text)) > maxChars, "warnings": w}
		if raw {
			body["base64"] = msg.Body.Data
		}
		part := map[string]any{"headers": msg.Headers, "body": body, "content_type": msg.Headers.Get("Content-Type"), "content_encoding": msg.Headers.Get("Content-Encoding")}
		if side == "request" {
			r := http.Request{Header: msg.Headers}
			part["cookies"] = r.Cookies()
		} else {
			r := http.Response{Header: msg.Headers}
			part["set_cookies"] = r.Cookies()
			part["redirect_location"] = msg.Headers.Get("Location")
		}
		out[side] = part
	}
	return out
}
func Analyze(entries []Entry, q Query, groupBy string) (map[string]any, error) {
	m, err := newMatcher(q)
	if err != nil {
		return nil, err
	}
	if groupBy == "" {
		groupBy = "host"
	}
	if !slices.Contains([]string{"host", "path", "route", "method", "status", "resource_class", "content_type"}, groupBy) {
		return nil, fmt.Errorf("unsupported group_by %q", groupBy)
	}
	groups := map[string]int{}
	statuses := map[int]int{}
	classes := map[string]int{}
	var count, errors int
	var bytes int64
	var durations []int64
	var total int64
	for _, e := range entries {
		if !m.match(e) {
			continue
		}
		count++
		if e.Status >= 400 || e.Error != "" {
			errors++
		}
		bytes += e.RequestBytes + e.ResponseBytes
		total += e.DurationMS
		durations = append(durations, e.DurationMS)
		statuses[e.Status]++
		classes[e.Class]++
		key := e.Host
		switch groupBy {
		case "path":
			key = e.Path
		case "route":
			key = e.Method + " " + e.Host + e.Path
		case "method":
			key = e.Method
		case "status":
			key = fmt.Sprint(e.Status)
		case "resource_class":
			key = e.Class
		case "content_type":
			key = e.Response.Headers.Get("Content-Type")
		}
		groups[key]++
	}
	list := []map[string]any{}
	for k, v := range groups {
		list = append(list, map[string]any{"key": k, "count": v})
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i]["count"].(int), list[j]["count"].(int)
		if a == b {
			return list[i]["key"].(string) < list[j]["key"].(string)
		}
		return a > b
	})
	slices.Sort(durations)
	var avg, p95 int64
	if count > 0 {
		avg = total / int64(count)
		p95 = durations[(count-1)*95/100]
	}
	return map[string]any{"total_entries": len(entries), "matched_entries": count, "error_count": errors, "total_body_bytes": bytes, "average_duration_ms": avg, "p95_duration_ms": p95, "status_counts": statuses, "resource_class_counts": classes, "group_by": groupBy, "groups": list}, nil
}
