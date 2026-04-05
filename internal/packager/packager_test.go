package packager

import (
	"os"
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

	// Create test script
	script := `#!/bin/bash
echo "before"
eval $(bashfs gen ./myfiles)
echo "after"
bashfs_cat greeting.txt
`

	result, err := Package(script, dir)
	require.Nil(t, err)

	// The eval line should be replaced
	assert.NotContains(t, result, "eval $(bashfs gen")

	// Embedded code should be present
	assert.Contains(t, result, "declare -A __bashfs_data")

	assert.Contains(t, result, "bashfs_cat()")

	// Surrounding lines should be preserved
	assert.Contains(t, result, `echo "before"`)

	assert.Contains(t, result, `echo "after"`)

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

	assert.Contains(t, result, "declare -A __bashfs_data")

}

func TestPackagePreservesIndentation(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "test.txt"), "data")

	script := "#!/bin/bash\n    eval $(bashfs gen ./myfiles)\n"
	result, err := Package(script, dir)
	require.Nil(t, err)

	// Check that indented lines exist
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(line, "declare -A __bashfs_data") {
			assert.True(t, strings.HasPrefix(line, "    "))

			break
		}
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))

	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

}
