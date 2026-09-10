package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
)

type Capture struct {
	ID      string    `json:"capture_id"`
	Source  string    `json:"source"`
	Format  string    `json:"format"`
	Created time.Time `json:"created_at"`
	Count   int       `json:"entry_count"`
}

type Body struct {
	Data         []byte `json:"base64,omitempty"`
	Preservation string `json:"preservation"`
	Charset      string `json:"charset,omitempty"`
}

type Message struct {
	Headers http.Header `json:"headers"`
	Body    Body        `json:"body"`
}

type Entry struct {
	ID            string  `json:"entry_id"`
	CaptureID     string  `json:"capture_id"`
	Sequence      int     `json:"sequence"`
	Method        string  `json:"method"`
	URL           string  `json:"url"`
	Host          string  `json:"host"`
	Path          string  `json:"path"`
	Status        int     `json:"status"`
	StartMS       int64   `json:"start_ms"`
	EndMS         int64   `json:"end_ms"`
	DurationMS    int64   `json:"duration_ms"`
	RequestBytes  int64   `json:"request_body_bytes"`
	ResponseBytes int64   `json:"response_body_bytes"`
	TotalSize     int64   `json:"total_size"`
	Request       Message `json:"request"`
	Response      Message `json:"response"`
	Class         string  `json:"resource_class"`
	Priority      int     `json:"priority"`
	Error         string  `json:"error,omitempty"`
}

func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b[:])
}

func digest(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }

func (e *Entry) Normalize(capture string, sequence int) {
	e.CaptureID = capture
	e.Sequence = sequence
	if e.Method == "" {
		e.Method = "GET"
	}
	e.Method = strings.ToUpper(e.Method)
	if u, err := url.Parse(e.URL); err == nil {
		e.Host = u.Hostname()
		e.Path = u.EscapedPath()
	}
	if e.Path == "" {
		e.Path = "/"
	}
	if e.Request.Headers == nil {
		e.Request.Headers = http.Header{}
	}
	if e.Response.Headers == nil {
		e.Response.Headers = http.Header{}
	}
	if e.Request.Body.Preservation == "" {
		e.Request.Body.Preservation = "missing"
	}
	if e.Response.Body.Preservation == "" {
		e.Response.Body.Preservation = "missing"
	}
	if e.EndMS >= e.StartMS && e.EndMS > 0 {
		e.DurationMS = e.EndMS - e.StartMS
	}
	if e.ID == "" {
		e.ID = digest(fmt.Sprintf("%s|%d|%s", capture, sequence, e.Identity()))[:32]
	}
	e.RequestBytes = max(e.RequestBytes, int64(len(e.Request.Body.Data)))
	e.ResponseBytes = max(e.ResponseBytes, int64(len(e.Response.Body.Data)))
	e.TotalSize = max(e.TotalSize, e.RequestBytes+e.ResponseBytes)
	e.Class, e.Priority = classify(*e)
}

// Identity uses only request attributes: a later response updates the same entry.
func (e Entry) Identity() string { return fmt.Sprintf("%s|%s|%d", e.Method, e.URL, e.StartMS) }

func classify(e Entry) (string, int) {
	ct := strings.ToLower(e.Response.Headers.Get("Content-Type"))
	p := strings.ToLower(e.Path)
	ext := path.Ext(p)
	c, priority := "other", 20
	switch {
	case e.Host == "control.charles":
		c, priority = "control", 0
	case e.Method == "CONNECT":
		c, priority = "tunnel", 0
	case strings.HasPrefix(ct, "image/") || slices.Contains([]string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico"}, ext):
		c, priority = "image", 5
	case strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "video/") || slices.Contains([]string{".mp3", ".mp4", ".wav", ".webm", ".mov", ".avi", ".m4a"}, ext):
		c, priority = "media", 5
	case strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript") || slices.Contains([]string{".js", ".mjs"}, ext):
		c, priority = "script", 15
	case strings.Contains(ct, "css") || strings.HasSuffix(p, ".css"):
		c, priority = "stylesheet", 10
	case strings.Contains(ct, "font") || slices.Contains([]string{".woff", ".woff2", ".ttf", ".otf", ".eot"}, ext):
		c, priority = "font", 5
	case strings.Contains(ct, "html"):
		c, priority = "document", 65
	case strings.Contains(ct, "json") || strings.Contains(e.Request.Headers.Get("Content-Type"), "json") || strings.Contains(ct, "protobuf") || anyContains(p, []string{"/api/", "/graphql", "/rpc", "/auth", "/login", "/token", "/session"}) || slices.Contains([]string{"POST", "PUT", "PATCH", "DELETE"}, e.Method):
		c, priority = "api", 90
	}
	if e.Status >= 400 || e.Error != "" {
		priority = 100
	}
	return c, priority
}
