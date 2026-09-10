package core

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "captures.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func testEntry(id string) Entry {
	e := Entry{Method: "POST", URL: "http://example.test:8080/api/login?ts=1&sign=aaaaaaaa", Status: 200, StartMS: time.Now().UnixMilli(), EndMS: time.Now().UnixMilli() + 10, Request: Message{Headers: http.Header{"Content-Type": {"application/json"}, "Authorization": {"Bearer secret"}}, Body: Body{Data: []byte(`{"user":{"name":"alice"},"sign":"aaaaaaaa"}`), Preservation: "raw"}}, Response: Message{Headers: http.Header{"Content-Type": {"application/json"}, "Set-Cookie": {"session=one", "csrf=two"}}, Body: Body{Data: []byte(`{"ok":true}`), Preservation: "raw"}}}
	e.ID = id
	e.Normalize("cap_test", 1)
	return e
}
func saveEntries(t *testing.T, s *Store, es ...Entry) {
	t.Helper()
	for i := range es {
		es[i].Normalize("cap_test", i+1)
	}
	if err := s.SaveCapture(context.Background(), Capture{ID: "cap_test", Source: "test", Format: "xml"}, es); err != nil {
		t.Fatal(err)
	}
}
func xmlFixture(n int, revision string) string {
	var b strings.Builder
	b.WriteString(`<charles-session>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<transaction method="POST" protocol="http" host="example.test" port="8080" path="/api/%d" startTimeMillis="%d" endTimeMillis="%d"><request><headers><header><name>Content-Type</name><value>application/json</value></header></headers><body encoding="base64">%s</body></request><response status="200"><headers><header><name>Content-Type</name><value>application/json</value></header><header><name>Set-Cookie</name><value>a=1</value></header><header><name>Set-Cookie</name><value>b=2</value></header></headers><body>{"revision":"%s"}</body></response></transaction>`, i, 1000+i, 1020+i, base64.StdEncoding.EncodeToString([]byte(`{"a":1}`)), revision)
	}
	b.WriteString(`</charles-session>`)
	return b.String()
}

func TestImportFormatsAndPreservation(t *testing.T) {
	es, err := ParseSession([]byte(xmlFixture(2, "one")), "xml", "cap")
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[0].URL != "http://example.test:8080/api/0" || es[0].Request.Body.Preservation != "raw" || len(es[0].Response.Headers.Values("Set-Cookie")) != 2 {
		t.Fatalf("bad XML: %+v", es)
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	meta := `{"method":"POST","scheme":"https","host":"api.test","port":8443,"path":"/login","times":{"startMillis":1000},"request":{"header":{"headers":[{"name":"Content-Type","value":"application/json"}]}},"response":{"status":201}}`
	for _, name := range []string{"10-meta.json", "2-meta.json"} {
		w, _ := z.Create(name)
		w.Write([]byte(meta))
		w, _ = z.Create(strings.TrimSuffix(name, "-meta.json") + "-req.dat")
		w.Write([]byte{0, 255, 1})
	}
	z.Close()
	es, err = ParseSession(buf.Bytes(), "native", "native")
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[0].URL != "https://api.test:8443/login" || !bytes.Equal(es[0].Request.Body.Data, []byte{0, 255, 1}) {
		t.Fatalf("bad native: %+v", es)
	}
	jsonRaw := `[{"method":"GET","scheme":"https","host":"api.test","path":"/api","request":{},"response":{"status":200,"body":{"text":"中文"}}}]`
	es, err = ParseSession([]byte(jsonRaw), "json", "json")
	if err != nil {
		t.Fatal(err)
	}
	if string(es[0].Response.Body.Data) != "中文" || es[0].Response.Body.Preservation != "text_only" {
		t.Fatal(es)
	}
	for _, raw := range []string{`<wrong/>`, `<charles-session><transaction/></charles-session>`, `<charles-session><transaction><request><body encoding="base64">!</body></request><response/></transaction></charles-session>`} {
		if _, err = ParseSession([]byte(raw), "xml", "bad"); err == nil {
			t.Fatalf("accepted invalid XML %s", raw)
		}
	}
}

func TestStorePersistenceAndCascade(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	e := testEntry("entry")
	saveEntries(t, s, e)
	s.SaveArtifact(ctx, "decoded", e.ID, map[string]any{"ok": true})
	e.Status = 401
	saveEntries(t, s, e)
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	es, err := s.Entries(ctx, "cap_test")
	if err != nil || len(es) != 1 || es[0].Status != 401 {
		t.Fatalf("upsert/reopen: %v %v", es, err)
	}
	if err = s.DeleteCapture(ctx, "cap_test"); err != nil {
		t.Fatal(err)
	}
	artifacts, err := s.Artifacts(ctx, "", "", 20, 0)
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("cascade: %v %v", artifacts, err)
	}
}

func TestDecodeCompressionFormatsAndProtobuf(t *testing.T) {
	payload := []byte(`{"message":"中文","ok":true}`)
	compressors := map[string]func([]byte) []byte{
		"gzip": func(p []byte) []byte {
			var b bytes.Buffer
			w := gzip.NewWriter(&b)
			w.Write(p)
			w.Close()
			return b.Bytes()
		},
		"deflate": func(p []byte) []byte {
			var b bytes.Buffer
			w := zlib.NewWriter(&b)
			w.Write(p)
			w.Close()
			return b.Bytes()
		},
		"br": func(p []byte) []byte {
			var b bytes.Buffer
			w := brotli.NewWriter(&b)
			w.Write(p)
			w.Close()
			return b.Bytes()
		},
		"zstd": func(p []byte) []byte { w, _ := zstd.NewWriter(nil); defer w.Close(); return w.EncodeAll(p, nil) },
	}
	for encoding, compress := range compressors {
		t.Run(encoding, func(t *testing.T) {
			m := Message{Headers: http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {encoding}}, Body: Body{Data: compress(payload), Preservation: "raw"}}
			r, err := Decode(m, "", "", 2048)
			if err != nil || r.Format != "json" || r.Value.(map[string]any)["message"] != "中文" {
				t.Fatalf("decode: %+v %v", r, err)
			}
			m.Body.Data = payload
			r, err = Decode(m, "", "", 2048)
			if err != nil || r.Format != "json" || len(r.Warnings) == 0 {
				t.Fatalf("already decoded fallback: %+v %v", r, err)
			}
		})
	}
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("test.proto"), Package: proto.String("test"), Syntax: proto.String("proto3"), MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Hello"), Field: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("user_name"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()}}}}}}}
	raw, _ := proto.Marshal(set)
	path := filepath.Join(t.TempDir(), "test.pb")
	os.WriteFile(path, raw, 0600)
	m := Message{Headers: http.Header{"Content-Type": {"application/x-protobuf"}}, Body: Body{Data: []byte{10, 3, 'B', 'o', 'b'}, Preservation: "raw"}}
	r, err := Decode(m, path, "test.Hello", 2048)
	if err != nil || r.Value.(map[string]any)["user_name"] != "Bob" {
		t.Fatalf("protobuf: %+v %v", r, err)
	}
	r, err = Decode(m, "", "", 2048)
	if err != nil || r.Format != "binary" || len(r.Warnings) == 0 {
		t.Fatalf("missing descriptor: %+v %v", r, err)
	}
	for _, tc := range []struct{ ct, body, format string }{{"application/json", "{bad", "text"}, {"application/x-www-form-urlencoded", "a=1&a=2&empty=", "form"}, {"multipart/form-data; boundary=x", "--x\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\nhello\r\n--x--\r\n", "multipart"}, {"text/plain", "", "text"}} {
		m := Message{Headers: http.Header{"Content-Type": {tc.ct}}, Body: Body{Data: []byte(tc.body), Preservation: "raw"}}
		r, err := Decode(m, "", "", 2048)
		if err != nil || r.Format != tc.format {
			t.Fatalf("%s: %+v %v", tc.ct, r, err)
		}
	}
	if _, err = Decode(Message{Body: Body{Preservation: "missing"}}, "", "", 2048); err == nil {
		t.Fatal("missing body accepted")
	}
}

func TestQueryAndStats(t *testing.T) {
	e := testEntry("one")
	e2 := testEntry("two")
	e2.Status = 404
	e2.Normalize("cap_test", 2)
	q := Query{Host: "example", Methods: []string{"post"}, RequestHeader: "authorization", RequestHeaderValue: "secret", RequestJSON: "user.name == 'alice'", ResponseBody: "true", MinSize: 1}
	r, err := QueryEntries([]Entry{e, e2}, q, 1, 0)
	if err != nil || r.Matched != 2 || r.NextOffset == nil || *r.NextOffset != 1 {
		t.Fatalf("query: %+v %v", r, err)
	}
	r, err = QueryEntries([]Entry{e, e2}, Query{Preset: "errors_only"}, 20, 0)
	if err != nil || r.Matched != 1 || r.Entries[0]["entry_id"] != "two" {
		t.Fatalf("error filter: %+v %v", r, err)
	}
	if _, err = QueryEntries([]Entry{e}, Query{RequestJSON: "["}, 20, 0); err == nil {
		t.Fatal("invalid JMESPath accepted")
	}
	stats, err := Analyze([]Entry{e, e2}, Query{}, "status")
	if err != nil || stats["error_count"] != 1 || len(stats["groups"].([]map[string]any)) != 2 {
		t.Fatalf("stats: %+v %v", stats, err)
	}
}

func TestSignatureAndNestedMutationPlan(t *testing.T) {
	a := testEntry("one")
	b := testEntry("two")
	b.URL = "http://example.test:8080/api/login?ts=2&sign=bbbbbbbb"
	b.Request.Body.Data = []byte(`{"user":{"name":"bob"},"sign":"bbbbbbbb"}`)
	b.Normalize("cap_test", 2)
	candidates, err := Discover([]Entry{a, b})
	if err != nil || len(candidates) == 0 || candidates[0].Score < 0.9 {
		t.Fatalf("signature: %+v %v", candidates, err)
	}
	plan := mutationPlan(a, "api")
	targets := plan["targets"].([]MutationTarget)
	if signatureName(targets[0].Field) {
		t.Fatalf("API prioritizes signature over business fields: %+v", targets)
	}
	for _, variant := range plan["variants"].([]MutationVariant) {
		req, err := BuildReplay(context.Background(), a, variant.Recipe)
		if err != nil {
			t.Fatal(err)
		}
		if variant.Target == "json:/user/name" && variant.Operation == "remove" {
			var v map[string]any
			json.NewDecoder(req.Body).Decode(&v)
			if _, ok := v["user"].(map[string]any)["name"]; ok {
				t.Fatal("nested mutation did not remove nested key")
			}
		}
	}
}
