package core

import (
	"os"
	"testing"
)

func TestGzipResponseExample(t *testing.T) {
	raw, err := os.ReadFile("../../examples/gzip-response.xml")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ParseSession(raw, "xml", "gzip_example")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want one entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Method != "GET" || e.Status != 200 || e.URL != "https://api.example.test/api/greeting" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if e.Response.Headers.Get("Content-Encoding") != "gzip" || e.Response.Body.Preservation != "raw" {
		t.Fatalf("compressed response was not preserved: %+v", e.Response)
	}
	result, err := Decode(e.Response, "", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := result.Value.(map[string]any)
	if result.Format != "json" || !ok || value["message"] != "hello from gzip" || value["ok"] != true || len(value) != 2 || len(result.Warnings) != 0 {
		t.Fatalf("unexpected decoded response: %+v", result)
	}
}
