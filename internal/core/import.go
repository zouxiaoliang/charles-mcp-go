package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const MaxInputBytes = 4 << 30 // 4 GiB, shared by exports and session imports.
const MaxBodyBytes = 32 << 20

type SizeLimitError struct{ Limit int64 }

func (e *SizeLimitError) Error() string {
	return fmt.Sprintf("content exceeds %d bytes; export or select a smaller Charles session", e.Limit)
}

func readBounded(r io.Reader, n int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, n+1))
	if err == nil && int64(len(b)) > n {
		err = &SizeLimitError{Limit: n}
	}
	return b, err
}
func readFile(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readBounded(f, n)
}

func ImportFile(ctx context.Context, path, format, cli string) (Capture, []Entry, error) {
	c := Capture{ID: NewID("cap_"), Source: path, Created: time.Now().UTC()}
	b, err := readFile(path, MaxInputBytes)
	if err != nil {
		return c, nil, err
	}
	if format == "" || format == "auto" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".chls", ".chlz":
			format = "native"
		case ".chlsj", ".json":
			format = "json"
		default:
			format = "xml"
		}
	}
	c.Format = format
	c.ID = "cap_" + digest(format + "|" + string(b))[:32]
	entries, err := ParseSession(b, format, c.ID)
	var sizeErr *SizeLimitError
	if errors.As(err, &sizeErr) {
		return c, nil, err
	}
	if err != nil && format == "native" {
		parseErr := err
		cli, findErr := FindCharlesCLI(cli)
		if findErr != nil {
			return c, nil, fmt.Errorf("native parser: %v; conversion fallback: %w", err, findErr)
		}
		dir, e := os.MkdirTemp("", "charles-convert-")
		if e != nil {
			return c, nil, e
		}
		defer os.RemoveAll(dir)
		out := filepath.Join(dir, "session.xml")
		conversionSource := path
		if bytes.HasPrefix(b, []byte{'P', 'K', 3, 4}) && !strings.EqualFold(filepath.Ext(path), ".chlz") {
			conversionSource = filepath.Join(dir, "source.chlz")
			if err = os.WriteFile(conversionSource, b, 0600); err != nil {
				return c, nil, err
			}
		}
		convertCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		if output, e := exec.CommandContext(convertCtx, cli, "convert", conversionSource, out).CombinedOutput(); e != nil {
			return c, nil, fmt.Errorf("native parsing: %v; Charles conversion: %w: %s", parseErr, e, preview(string(output), 2048))
		}
		b, err = readFile(out, MaxInputBytes)
		if err == nil {
			entries, err = ParseSession(b, "xml", c.ID)
		}
	}
	c.Count = len(entries)
	return c, entries, err
}

func FindCharlesCLI(configured string) (string, error) {
	if configured != "" {
		p, err := exec.LookPath(configured)
		if err != nil {
			return "", fmt.Errorf("CHARLES_CLI_PATH: %w", err)
		}
		return p, nil
	}
	names := []string{"charles"}
	switch runtime.GOOS {
	case "darwin":
		names = append(names, "/Applications/Charles.app/Contents/MacOS/Charles")
	case "windows":
		names = append(names, filepath.Join(os.Getenv("ProgramFiles"), "Charles", "Charles.exe"), filepath.Join(os.Getenv("ProgramFiles(x86)"), "Charles", "Charles.exe"))
	case "linux":
		names = append(names, "/usr/bin/charles", "/opt/charles/bin/charles")
	}
	for _, p := range names {
		if found, err := exec.LookPath(p); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("Charles CLI not found; set CHARLES_CLI_PATH for native conversion")
}

func ParseSession(b []byte, format, capture string) ([]Entry, error) {
	var out []Entry
	var err error
	switch format {
	case "xml":
		out, err = parseXML(b)
	case "json", "legacy_json":
		out, err = parseJSON(b)
	case "native":
		out, err = parseNative(b)
	default:
		return nil, fmt.Errorf("unsupported format %q (xml, native, json)", format)
	}
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Normalize(capture, i+1)
	}
	return out, nil
}

type xmlBody struct {
	Encoding string `xml:"encoding,attr"`
	Text     string `xml:",chardata"`
}
type xmlMessage struct {
	Status  int    `xml:"status,attr"`
	Charset string `xml:"charset,attr"`
	Mime    string `xml:"mime-type,attr"`
	Headers []struct {
		Name  string `xml:"name"`
		Value string `xml:"value"`
	} `xml:"headers>header"`
	Body *xmlBody `xml:"body"`
}
type xmlTransaction struct {
	Method     string      `xml:"method,attr"`
	Protocol   string      `xml:"protocol,attr"`
	Host       string      `xml:"host,attr"`
	Path       string      `xml:"path,attr"`
	Query      string      `xml:"query,attr"`
	Port       int         `xml:"port,attr"`
	ActualPort int         `xml:"actualPort,attr"`
	Start      int64       `xml:"startTimeMillis,attr"`
	End        int64       `xml:"endTimeMillis,attr"`
	StartText  string      `xml:"startTime,attr"`
	EndText    string      `xml:"endTime,attr"`
	Request    *xmlMessage `xml:"request"`
	Response   *xmlMessage `xml:"response"`
}

func parseXML(b []byte) ([]Entry, error) {
	var session struct {
		XMLName      xml.Name         `xml:"charles-session"`
		Transactions []xmlTransaction `xml:"transaction"`
	}
	if err := xml.Unmarshal(b, &session); err != nil {
		return nil, err
	}
	out := []Entry{}
	for i, t := range session.Transactions {
		if t.Request == nil || t.Response == nil {
			return nil, fmt.Errorf("transaction %d lacks request/response", i+1)
		}
		req, err := fromXMLMessage(*t.Request)
		if err != nil {
			return nil, err
		}
		res, err := fromXMLMessage(*t.Response)
		if err != nil {
			return nil, err
		}
		if t.ActualPort > 0 {
			t.Port = t.ActualPort
		}
		if t.Start == 0 {
			t.Start = parseTime(t.StartText)
		}
		if t.End == 0 {
			t.End = parseTime(t.EndText)
		}
		out = append(out, Entry{Method: t.Method, URL: makeURL(t.Protocol, t.Host, t.Port, t.Path, t.Query), StartMS: t.Start, EndMS: t.End, Status: t.Response.Status, Request: req, Response: res})
	}
	return out, nil
}
func fromXMLMessage(m xmlMessage) (Message, error) {
	out := Message{Headers: http.Header{}, Body: Body{Preservation: "missing", Charset: m.Charset}}
	for _, h := range m.Headers {
		out.Headers.Add(h.Name, h.Value)
	}
	if m.Mime != "" && out.Headers.Get("Content-Type") == "" {
		out.Headers.Set("Content-Type", m.Mime)
	}
	if m.Body != nil {
		out.Body.Preservation = "text_only"
		out.Body.Data = []byte(m.Body.Text)
		if strings.EqualFold(m.Body.Encoding, "base64") {
			b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(m.Body.Text), ""))
			if err != nil {
				return out, fmt.Errorf("invalid base64 body: %w", err)
			}
			out.Body.Data = b
			out.Body.Preservation = "raw"
		} else if m.Body.Encoding != "" && !strings.EqualFold(m.Body.Encoding, "text") {
			return out, fmt.Errorf("unsupported XML body encoding %q", m.Body.Encoding)
		}
	}
	return out, nil
}
func makeURL(scheme, host string, port int, path, query string) string {
	if scheme == "" {
		scheme = "http"
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	authority := host
	if port > 0 && !(scheme == "http" && port == 80) && !(scheme == "https" && port == 443) {
		authority = net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
	} else if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		authority = "[" + host + "]"
	}
	result := scheme + "://" + authority + path
	if query != "" {
		result += "?" + strings.TrimPrefix(query, "?")
	}
	return result
}
func parseTime(s string) int64 {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700", "2006-01-02 15:04:05"} {
		if t, e := time.Parse(layout, s); e == nil {
			return t.UnixMilli()
		}
	}
	return 0
}
func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}
func str(v any) string { s, _ := v.(string); return s }
func num(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}
func parseJSON(b []byte) ([]Entry, error) {
	var raw []map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	out := []Entry{}
	for _, m := range raw {
		e, err := fromJSON(m)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
func fromJSON(m map[string]any) (Entry, error) {
	req, err := jsonMessage(object(m["request"]))
	if err != nil {
		return Entry{}, err
	}
	res, err := jsonMessage(object(m["response"]))
	if err != nil {
		return Entry{}, err
	}
	times := object(m["times"])
	start, end := num(times["startMillis"]), num(times["endMillis"])
	if start == 0 {
		start = parseTime(str(times["start"]))
	}
	if end == 0 {
		end = parseTime(str(times["end"]))
	}
	port := num(m["actualPort"])
	if port == 0 {
		port = num(m["port"])
	}
	u := str(m["url"])
	if u == "" {
		u = makeURL(str(m["scheme"]), str(m["host"]), int(port), str(m["path"]), str(m["query"]))
	}
	if _, err = url.ParseRequestURI(u); err != nil {
		return Entry{}, err
	}
	return Entry{Method: str(m["method"]), URL: u, StartMS: start, EndMS: end, DurationMS: num(object(m["durations"])["total"]), RequestBytes: num(object(object(m["request"])["sizes"])["body"]), ResponseBytes: num(object(object(m["response"])["sizes"])["body"]), TotalSize: num(m["totalSize"]), Status: int(num(object(m["response"])["status"])), Request: req, Response: res, Error: str(m["error"])}, nil
}
func jsonMessage(m map[string]any) (Message, error) {
	out := Message{Headers: http.Header{}, Body: Body{Preservation: "missing", Charset: str(m["charset"])}}
	hs, _ := object(m["header"])["headers"].([]any)
	for _, v := range hs {
		h := object(v)
		if name := str(h["name"]); name != "" {
			out.Headers.Add(name, str(h["value"]))
		}
	}
	for k, v := range map[string]string{"Content-Type": str(m["mimeType"]), "Content-Encoding": str(m["contentEncoding"])} {
		if out.Headers.Get(k) == "" && v != "" {
			out.Headers.Set(k, v)
		}
	}
	body := object(m["body"])
	if v, ok := body["text"].(string); ok {
		out.Body.Data = []byte(v)
		out.Body.Preservation = "text_only"
		if encoded, ok := body["encoded"].(bool); ok && encoded {
			b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(v), ""))
			if err != nil {
				return out, fmt.Errorf("invalid encoded JSON body: %w", err)
			}
			out.Body.Data = b
			out.Body.Preservation = "raw"
		}
	}
	for _, k := range []string{"encoded", "base64"} {
		if v, ok := body[k].(string); ok {
			b, err := base64.StdEncoding.DecodeString(v)
			if err != nil {
				return out, err
			}
			out.Body.Data = b
			out.Body.Preservation = "raw"
			break
		}
	}
	return out, nil
}
func parseNative(b []byte) ([]Entry, error) {
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	files := map[string]*zip.File{}
	metas := []string{}
	var total uint64
	for _, f := range z.File {
		total += f.UncompressedSize64
		if total > MaxInputBytes {
			return nil, &SizeLimitError{Limit: MaxInputBytes}
		}
		if _, ok := files[f.Name]; ok {
			return nil, fmt.Errorf("duplicate archive member %s", f.Name)
		}
		files[f.Name] = f
		if strings.HasSuffix(f.Name, "-meta.json") {
			metas = append(metas, f.Name)
		}
	}
	if len(metas) == 0 {
		return nil, fmt.Errorf("native archive contains no metadata")
	}
	sort.Slice(metas, func(i, j int) bool {
		a, _ := strconv.Atoi(strings.Split(filepath.Base(metas[i]), "-")[0])
		b, _ := strconv.Atoi(strings.Split(filepath.Base(metas[j]), "-")[0])
		if a == b {
			return metas[i] < metas[j]
		}
		return a < b
	})
	read := func(f *zip.File) ([]byte, error) {
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return readBounded(r, MaxInputBytes)
	}
	out := []Entry{}
	for _, name := range metas {
		raw, err := read(files[name])
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err = json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		e, err := fromJSON(m)
		if err != nil {
			return nil, err
		}
		prefix := strings.TrimSuffix(name, "-meta.json")
		for side, msg := range map[string]*Message{"req": &e.Request, "res": &e.Response} {
			for _, ext := range []string{".json", ".txt", ".html", ".dat", ".xml", ".csv"} {
				if f := files[prefix+"-"+side+ext]; f != nil {
					data, err := read(f)
					if err != nil {
						return nil, err
					}
					msg.Body.Data = data
					msg.Body.Preservation = "raw"
					break
				}
			}
		}
		out = append(out, e)
	}
	return out, nil
}
