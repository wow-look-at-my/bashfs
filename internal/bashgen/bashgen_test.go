package bashgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bashfs/internal/fswalker"
)

func TestGenerateDevMode(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)
	mustWriteFile(t, filepath.Join(dir, "sub", "data.txt"), "hello")

	files, err := fswalker.Walk(dir)
	if err != nil {
		t.Fatal(err)
	}

	output, err := GenerateDevMode(files, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify key functions are present
	for _, fn := range []string{"bashfs_cat()", "bashfs_extract()", "bashfs_list()", "bashfs_jq()"} {
		if !strings.Contains(output, fn) {
			t.Errorf("output missing function %s", fn)
		}
	}

	// Verify file paths are listed
	if !strings.Contains(output, "config.json") {
		t.Error("output missing config.json")
	}
	if !strings.Contains(output, "sub/data.txt") {
		t.Error("output missing sub/data.txt")
	}

	// Verify absolute path is embedded
	absDir, _ := filepath.Abs(dir)
	if !strings.Contains(output, absDir) {
		t.Errorf("output missing absolute dir %s", absDir)
	}
}

func TestGenerateEmbedded(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "hello.txt"), "hello world")
	mustWriteFile(t, filepath.Join(dir, "sub", "data.json"), `{"key":"value"}`)

	files, err := fswalker.Walk(dir)
	if err != nil {
		t.Fatal(err)
	}

	output, err := GenerateEmbedded(files)
	if err != nil {
		t.Fatal(err)
	}

	// Verify associative array declaration
	if !strings.Contains(output, "declare -A __bashfs_data") {
		t.Error("output missing declare -A __bashfs_data")
	}

	// Verify functions
	for _, fn := range []string{"bashfs_cat()", "bashfs_extract()", "bashfs_list()", "bashfs_jq()"} {
		if !strings.Contains(output, fn) {
			t.Errorf("output missing function %s", fn)
		}
	}

	// Verify file keys are present
	if !strings.Contains(output, `["hello.txt"]`) {
		t.Error("output missing hello.txt key")
	}
	if !strings.Contains(output, `["sub/data.json"]`) {
		t.Error("output missing sub/data.json key")
	}

	// Verify base64 data is present (should contain = padding or alphanumeric)
	if !strings.Contains(output, "=") {
		t.Error("output missing base64 encoded data")
	}
}

func TestGenerateEmbeddedEmpty(t *testing.T) {
	output, err := GenerateEmbedded(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "declare -A __bashfs_data") {
		t.Error("empty output should still have array declaration")
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
