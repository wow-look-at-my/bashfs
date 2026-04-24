package packager

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wow-look-at-my/testify/assert"
	"github.com/wow-look-at-my/testify/require"
)

func TestPackage(t *testing.T) {
	dir := t.TempDir()

	// Create test filesystem
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	script := `#!/bin/bash
echo "before"
eval $(bashfs gen ./myfiles)
echo "after"
bashfs_cat greeting.txt
`

	result, err := Package(script, dir)
	require.Nil(t, err)

	output := string(result.Data)

	// The eval line should be replaced
	assert.NotContains(t, output, "eval $(bashfs gen")

	// Embedded code should be present
	assert.Contains(t, output, "declare -A __bashfs_offset")
	assert.Contains(t, output, "bashfs_cat()")

	// Surrounding lines should be preserved
	assert.Contains(t, output, `echo "before"`)
	assert.Contains(t, output, `echo "after"`)

	// Exit guard should be present
	assert.Contains(t, output, "exit 0")

	// Binary payload should be appended (data is longer than just the text)
	textEnd := strings.Index(output, "exit 0\n") + len("exit 0\n")
	assert.Greater(t, len(result.Data), textEnd)
}

func TestPackageNoEval(t *testing.T) {
	_, err := Package("#!/bin/bash\necho hello\n", "/tmp")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "no 'eval $(bashfs gen ...)' line found")
}

func TestPackageMultipleEval(t *testing.T) {
	script := `#!/bin/bash
eval $(bashfs gen ./a)
eval $(bashfs gen ./b)
`
	_, err := Package(script, "/tmp")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "multiple")
}

func TestPackageQuotedPath(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "test.txt"), "data")

	script := `#!/bin/bash
eval $(bashfs gen "./myfiles")
`
	result, err := Package(script, dir)
	require.Nil(t, err)
	assert.Contains(t, string(result.Data), "declare -A __bashfs_offset")
}

func TestPackagePreservesIndentation(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "test.txt"), "data")

	script := "#!/bin/bash\n    eval $(bashfs gen ./myfiles)\n"
	result, err := Package(script, dir)
	require.Nil(t, err)

	for _, line := range strings.Split(string(result.Data), "\n") {
		if strings.Contains(line, "declare -A __bashfs_offset") {
			assert.True(t, strings.HasPrefix(line, "    "))
			break
		}
	}
}

func TestPackageRunsDirectAndPiped(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	script := `#!/bin/bash
eval $(bashfs gen ./myfiles)
bashfs_cat greeting.txt
`
	result, err := Package(script, dir)
	require.Nil(t, err)

	scriptPath := filepath.Join(dir, "packaged.sh")
	require.NoError(t, os.WriteFile(scriptPath, result.Data, 0755))

	// Direct execution: BASH_SOURCE[0] is a real path, trampoline skipped.
	out, err := exec.Command("bash", scriptPath).Output()
	require.Nil(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(string(out)))

	// Piped execution (simulates curl ... | bash): BASH_SOURCE[0]="main",
	// trampoline must spool stdin to a tempfile and re-exec.
	cmd := exec.Command("bash")
	cmd.Stdin = bytes.NewReader(result.Data)
	out, err = cmd.Output()
	require.Nil(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(string(out)))
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}
