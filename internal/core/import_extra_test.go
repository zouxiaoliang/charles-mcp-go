package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOversizedNativeDoesNotFallBackToConversion(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	if _, err := z.CreateRaw(&zip.FileHeader{Name: "1-meta.json", Method: zip.Store, UncompressedSize64: MaxInputBytes + 1}); err != nil {
		t.Fatal(err)
	}
	z.Close()
	path := filepath.Join(t.TempDir(), "oversized.chls")
	os.WriteFile(path, b.Bytes(), 0600)
	_, _, err := ImportFile(context.Background(), path, "native", os.Args[0])
	var sizeErr *SizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("expected original size error without invoking converter; got %v", err)
	}
}

func TestNativeExportLimitUses64BitSizes(t *testing.T) {
	metadata := []byte(`{"scheme":"https","host":"example.test","path":"/","request":{},"response":{"status":200}}`)
	for _, tc := range []struct {
		name           string
		total          uint64
		wantLimitError bool
	}{
		{"above_previous_limit", 129 << 20, false},
		{"at_4_GiB", 4 << 30, false},
		{"above_4_GiB", (4 << 30) + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := zip.NewWriter(&archive)
			member, err := writer.Create("1-meta.json")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = member.Write(metadata); err != nil {
				t.Fatal(err)
			}
			// An unconsumed opaque member exercises the ZIP size accounting and
			// 4 GiB boundary without allocating a multi-gigabyte body in tests.
			if _, err = writer.CreateRaw(&zip.FileHeader{Name: "opaque.bin", Method: zip.Store, UncompressedSize64: tc.total - uint64(len(metadata))}); err != nil {
				t.Fatal(err)
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			entries, err := ParseSession(archive.Bytes(), "native", "size-boundary")
			var limitError *SizeLimitError
			if tc.wantLimitError {
				if !errors.As(err, &limitError) || limitError.Limit != 4<<30 {
					t.Fatalf("expected 4 GiB size error, got %v", err)
				}
			} else if err != nil || len(entries) != 1 {
				t.Fatalf("expected archive within 4 GiB to be accepted, got %d entries: %v", len(entries), err)
			}
		})
	}
}

func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == "convert" {
		b, err := os.ReadFile(os.Args[2])
		if err != nil {
			os.Exit(3)
		}
		if string(b) == "fail-native" {
			fmt.Fprintln(os.Stderr, "conversion failure")
			os.Exit(4)
		}
		if err = os.WriteFile(os.Args[3], []byte(xmlFixture(2, "converted")), 0600); err != nil {
			os.Exit(5)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestNativeConversionFallbackAndErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session with spaces.chls")
	os.WriteFile(path, []byte("older-native-format"), 0600)
	c, entries, err := ImportFile(context.Background(), path, "native", os.Args[0])
	if err != nil || len(entries) != 2 || entries[0].CaptureID != c.ID {
		t.Fatalf("conversion: %+v %v", entries, err)
	}
	os.WriteFile(path, []byte("fail-native"), 0600)
	if _, _, err = ImportFile(context.Background(), path, "native", os.Args[0]); err == nil || !strings.Contains(err.Error(), "conversion failure") {
		t.Fatalf("conversion failure not propagated: %v", err)
	}
}
func TestJSONEncodedFlagAndMissingBodySizes(t *testing.T) {
	raw := fmt.Sprintf(`[{"scheme":"http","host":"test","path":"/","totalSize":1500,"request":{"body":{"encoded":true,"text":"%s"}},"response":{"sizes":{"body":1000},"status":200}}]`, base64.StdEncoding.EncodeToString([]byte{0, 255, 1}))
	es, err := ParseSession([]byte(raw), "json", "cap")
	if err != nil {
		t.Fatal(err)
	}
	if string(es[0].Request.Body.Data) != string([]byte{0, 255, 1}) || es[0].Request.Body.Preservation != "raw" || es[0].ResponseBytes != 1000 || es[0].TotalSize != 1500 {
		t.Fatalf("encoded/sizes: %+v", es[0])
	}
	result, err := QueryEntries(es, Query{MinSize: 1200}, 20, 0)
	if err != nil || result.Matched != 1 {
		t.Fatalf("missing body size query: %+v %v", result, err)
	}
}
