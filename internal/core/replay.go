package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ReplayOptions struct {
	Query           map[string]any     `json:"query_overrides,omitempty"`
	Headers         map[string]*string `json:"header_overrides,omitempty"`
	JSON            map[string]any     `json:"json_overrides,omitempty"`
	Form            map[string]any     `json:"form_overrides,omitempty"`
	BodyText        *string            `json:"body_text_override,omitempty"`
	FollowRedirects *bool              `json:"follow_redirects,omitempty"`
	UseProxy        bool               `json:"use_proxy,omitempty"`
}

func mutateValues(v url.Values, overrides map[string]any) {
	for k, x := range overrides {
		v.Del(k)
		if x == nil {
			continue
		}
		switch vs := x.(type) {
		case []any:
			for _, a := range vs {
				v.Add(k, fmt.Sprint(a))
			}
		case []string:
			for _, a := range vs {
				v.Add(k, a)
			}
		default:
			v.Set(k, fmt.Sprint(x))
		}
	}
}

// JSON overrides accept JSON Pointers for unambiguous nested fields, or top-level keys.
func mutateJSON(root map[string]any, key string, value any) error {
	parts := []string{key}
	if strings.HasPrefix(key, "/") {
		parts = strings.Split(key[1:], "/")
		for i := range parts {
			parts[i] = strings.ReplaceAll(strings.ReplaceAll(parts[i], "~1", "/"), "~0", "~")
		}
	}
	current := root
	for _, p := range parts[:len(parts)-1] {
		child, exists := current[p]
		if !exists {
			if value == nil {
				return nil
			}
			next := map[string]any{}
			current[p] = next
			current = next
			continue
		}
		next, ok := child.(map[string]any)
		if !ok {
			return fmt.Errorf("JSON override %q crosses non-object field %q", key, p)
		}
		current = next
	}
	last := parts[len(parts)-1]
	if value == nil {
		delete(current, last)
	} else {
		current[last] = value
	}
	return nil
}
func BuildReplay(ctx context.Context, e Entry, o ReplayOptions) (*http.Request, error) {
	u, err := url.Parse(e.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("replay URL must be http or https")
	}
	count := 0
	for _, set := range []bool{o.JSON != nil, o.Form != nil, o.BodyText != nil} {
		if set {
			count++
		}
	}
	if count > 1 {
		return nil, fmt.Errorf("choose exactly one body mutation: json_overrides, form_overrides or body_text_override")
	}
	query := u.Query()
	mutateValues(query, o.Query)
	u.RawQuery = query.Encode()
	headers := e.Request.Headers.Clone()
	body := e.Request.Body.Data
	if count > 0 {
		headers.Del("Content-Encoding")
		headers.Del("Content-MD5")
		headers.Del("Digest")
	}
	switch {
	case o.BodyText != nil:
		body = []byte(*o.BodyText)
	case o.JSON != nil:
		b, _, err := bodyBytes(e.Request)
		if err != nil {
			return nil, err
		}
		var obj map[string]any
		if err = json.Unmarshal(b, &obj); err != nil || obj == nil {
			return nil, fmt.Errorf("JSON overrides require an object body")
		}
		for k, v := range o.JSON {
			if err = mutateJSON(obj, k, v); err != nil {
				return nil, err
			}
		}
		body, err = json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		headers.Set("Content-Type", "application/json")
	case o.Form != nil:
		if !strings.HasPrefix(e.Request.Headers.Get("Content-Type"), "application/x-www-form-urlencoded") {
			return nil, fmt.Errorf("form overrides require URL-encoded form content type")
		}
		b, _, err := bodyBytes(e.Request)
		if err != nil {
			return nil, err
		}
		v, err := url.ParseQuery(string(b))
		if err != nil {
			return nil, err
		}
		mutateValues(v, o.Form)
		body = []byte(v.Encode())
		headers.Set("Content-Type", "application/x-www-form-urlencoded")
	default:
		if e.Request.Body.Preservation == "missing" && headers.Get("Content-Length") != "" && headers.Get("Content-Length") != "0" {
			return nil, fmt.Errorf("original request body is missing; provide a body override")
		}
		if e.Request.Body.Preservation == "text_only" {
			headers.Del("Content-Encoding")
		}
	}
	for k, v := range o.Headers {
		if v == nil {
			headers.Del(k)
		} else {
			headers.Set(k, *v)
		}
	}
	for _, k := range strings.Split(headers.Get("Connection"), ",") {
		headers.Del(strings.TrimSpace(k))
	}
	for _, k := range []string{"Host", "Content-Length", "Transfer-Encoding", "Connection", "Proxy-Connection", "Proxy-Authorization", "Keep-Alive", "TE", "Trailer", "Upgrade"} {
		headers.Del(k)
	}
	req, err := http.NewRequestWithContext(ctx, e.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = headers
	return req, nil
}

type ReplayResult struct {
	ExperimentID    string         `json:"experiment_id"`
	ExecutionStatus string         `json:"execution_status"`
	Error           string         `json:"error,omitempty"`
	URL             string         `json:"url"`
	Status          int            `json:"status"`
	LatencyMS       int64          `json:"latency_ms"`
	BaselineStatus  int            `json:"baseline_status"`
	StatusChanged   bool           `json:"status_changed"`
	BodySizeDelta   int            `json:"body_size_delta"`
	Response        map[string]any `json:"response,omitempty"`
}

func Replay(ctx context.Context, s *Store, c *Charles, id string, o ReplayOptions) (ReplayResult, error) {
	e, err := s.Entry(ctx, id)
	if err != nil {
		return ReplayResult{}, err
	}
	req, err := BuildReplay(ctx, e, o)
	if err != nil {
		return ReplayResult{}, err
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.DisableCompression = true
	defer tr.CloseIdleConnections()
	if o.UseProxy {
		if c.ProxyURL == "" {
			return ReplayResult{}, fmt.Errorf("Charles proxy URL is not configured")
		}
		u, err := url.Parse(c.ProxyURL)
		if err != nil {
			return ReplayResult{}, err
		}
		tr.Proxy = http.ProxyURL(u)
	}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	if o.FollowRedirects != nil && !*o.FollowRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	result := ReplayResult{ExecutionStatus: "completed", URL: req.URL.String(), BaselineStatus: e.Status}
	start := time.Now()
	res, sendErr := client.Do(req)
	var raw []byte
	var msg Message
	if sendErr == nil {
		defer res.Body.Close()
		result.Status = res.StatusCode
		raw, sendErr = readBounded(res.Body, MaxBodyBytes)
		msg = Message{Headers: res.Header, Body: Body{Data: raw, Preservation: "raw"}}
	}
	result.LatencyMS = time.Since(start).Milliseconds()
	if sendErr != nil {
		result.ExecutionStatus = "failed"
		result.Error = sendErr.Error()
	}
	result.StatusChanged = result.Status != e.Status
	result.BodySizeDelta = len(raw) - len(e.Response.Body.Data)
	if res != nil {
		b, w, _ := bodyBytes(msg)
		result.Response = map[string]any{"headers": res.Header, "byte_length": len(raw), "preview": preview(bodyText(msg, b), 2048), "truncated": len([]rune(bodyText(msg, b))) > 2048, "warnings": w}
	}
	result.ExperimentID, err = s.SaveArtifact(ctx, "experiment", id, map[string]any{"result": result, "request": map[string]any{"method": req.Method, "url": req.URL.String(), "headers": req.Header, "mutations": o}, "response": msg})
	if err != nil {
		return result, err
	}
	_, err = s.SaveArtifact(ctx, "finding", id, map[string]any{"type": "replay_comparison", "experiment_id": result.ExperimentID, "execution_status": result.ExecutionStatus, "status_changed": result.StatusChanged, "baseline_status": e.Status, "replay_status": result.Status, "body_size_delta": result.BodySizeDelta})
	return result, err
}
