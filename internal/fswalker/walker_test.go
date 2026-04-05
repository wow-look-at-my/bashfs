package fswalker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalk(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "aaa")
	mustWriteFile(t, filepath.Join(dir, "sub", "b.txt"), "bbb")
	mustWriteFile(t, filepath.Join(dir, "sub", "deep", "c.json"), `{"key":"val"}`)
	mustWriteFile(t, filepath.Join(dir, ".hidden"), "hidden")
	mustWriteFile(t, filepath.Join(dir, ".hiddendir", "d.txt"), "ddd")

	entries, err := Walk(dir)
	if err != nil {
		t.Fatalf("Walk() error: %v", err)
	}

	// Should find a.txt, sub/b.txt, sub/deep/c.json but skip hidden files/dirs
	want := []string{"a.txt", "sub/b.txt", "sub/deep/c.json"}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(entries), len(want), entries)
	}
	for i, w := range want {
		if entries[i].RelPath != w {
			t.Errorf("entry[%d].RelPath = %q, want %q", i, entries[i].RelPath, w)
		}
		if !filepath.IsAbs(entries[i].AbsPath) {
			t.Errorf("entry[%d].AbsPath = %q, not absolute", i, entries[i].AbsPath)
		}
	}
}

func TestWalkEmpty(t *testing.T) {
	dir := t.TempDir()
	entries, err := Walk(dir)
	if err != nil {
		t.Fatalf("Walk() error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries for empty dir, want 0", len(entries))
	}
}

func TestWalkNonexistent(t *testing.T) {
	_, err := Walk("/nonexistent/path")
	if err == nil {
		t.Fatal("Walk() expected error for nonexistent dir")
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
