package packager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackage(t *testing.T) {
	dir := t.TempDir()

	// Create test filesystem
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	// Create test script
	script := `#!/bin/bash
echo "before"
eval $(bashfs gen ./myfiles)
echo "after"
bashfs_cat greeting.txt
`

	result, err := Package(script, dir)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}

	// The eval line should be replaced
	if strings.Contains(result, "eval $(bashfs gen") {
		t.Error("eval line was not replaced")
	}

	// Embedded code should be present
	if !strings.Contains(result, "declare -A __bashfs_data") {
		t.Error("missing embedded data declaration")
	}
	if !strings.Contains(result, "bashfs_cat()") {
		t.Error("missing bashfs_cat function")
	}

	// Surrounding lines should be preserved
	if !strings.Contains(result, `echo "before"`) {
		t.Error("missing line before eval")
	}
	if !strings.Contains(result, `echo "after"`) {
		t.Error("missing line after eval")
	}
}

func TestPackageNoEval(t *testing.T) {
	_, err := Package("#!/bin/bash\necho hello\n", "/tmp")
	if err == nil {
		t.Fatal("expected error for missing eval line")
	}
	if !strings.Contains(err.Error(), "no 'eval $(bashfs gen ...)' line found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPackageMultipleEval(t *testing.T) {
	script := `#!/bin/bash
eval $(bashfs gen ./a)
eval $(bashfs gen ./b)
`
	_, err := Package(script, "/tmp")
	if err == nil {
		t.Fatal("expected error for multiple eval lines")
	}
	if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPackageQuotedPath(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "test.txt"), "data")

	script := `#!/bin/bash
eval $(bashfs gen "./myfiles")
`
	result, err := Package(script, dir)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}
	if !strings.Contains(result, "declare -A __bashfs_data") {
		t.Error("missing embedded data for quoted path")
	}
}

func TestPackagePreservesIndentation(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "test.txt"), "data")

	script := "#!/bin/bash\n    eval $(bashfs gen ./myfiles)\n"
	result, err := Package(script, dir)
	if err != nil {
		t.Fatalf("Package() error: %v", err)
	}

	// Check that indented lines exist
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(line, "declare -A __bashfs_data") {
			if !strings.HasPrefix(line, "    ") {
				t.Errorf("embedded code not indented: %q", line)
			}
			break
		}
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
