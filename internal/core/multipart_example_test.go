package core

import (
	"os"
	"testing"
)

func TestMultipartRequestExample(t *testing.T) {
	raw, err := os.ReadFile("../../examples/multipart-request.json")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ParseSession(raw, "json", "multipart_example")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want one entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Method != "POST" || e.Status != 200 || e.URL != "https://api.example.test/api/upload" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	result, err := Decode(e.Request, "", "", 4096)
	if err != nil {
		t.Fatal(err)
	}
	parts, ok := result.Value.([]map[string]any)
	if result.Format != "multipart" || !ok || len(parts) != 2 || len(result.Warnings) != 0 {
		t.Fatalf("unexpected multipart result: %+v", result)
	}
	for i, want := range []struct{ name, filename, text string }{
		{"description", "", "Offline upload demo"},
		{"attachment", "hello.txt", "hello from a fake file\n"},
	} {
		part := parts[i]
		if part["name"] != want.name || part["filename"] != want.filename || part["preview"] != want.text || part["byte_length"] != len(want.text) || part["truncated"] != false {
			t.Fatalf("part %d: got %+v, want %+v", i, part, want)
		}
	}
}
