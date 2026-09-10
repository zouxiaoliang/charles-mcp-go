package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileReplacementRemovesStaleFiles(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "source"), filepath.Join(dir, "destination")
	os.Mkdir(src, 0700)
	os.Mkdir(dst, 0700)
	os.WriteFile(filepath.Join(src, "new.xml"), []byte("new"), 0600)
	os.WriteFile(filepath.Join(dst, "stale.xml"), []byte("old"), 0600)
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "stale.xml")); !os.IsNotExist(err) {
		t.Fatal("stale profile survived restore")
	}
	if b, err := os.ReadFile(filepath.Join(dst, "new.xml")); err != nil || string(b) != "new" {
		t.Fatalf("new profile %s %v", b, err)
	}
	if err := copyTree(src, filepath.Join(src, "inside")); err == nil {
		t.Fatal("recursive backup accepted")
	}
	if err := copyTree(src, dir); err == nil {
		t.Fatal("ancestor replacement accepted")
	}
}
