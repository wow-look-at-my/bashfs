package bashgen

import (
	"os"
	"path/filepath"
	"testing"

	"bashfs/internal/fswalker"
	"github.com/wow-look-at-my/testify/assert"
	"github.com/wow-look-at-my/testify/require"
)

func TestGenerateDevMode(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)
	mustWriteFile(t, filepath.Join(dir, "sub", "data.txt"), "hello")

	files, err := fswalker.Walk(dir)
	require.Nil(t, err)

	output, err := GenerateDevMode(files, dir)
	require.Nil(t, err)

	// Verify key functions are present
	for _, fn := range []string{"bashfs_cat()", "bashfs_extract()", "bashfs_list()", "bashfs_jq()"} {
		assert.Contains(t, output, fn)

	}

	// Verify file paths are listed
	assert.Contains(t, output, "config.json")

	assert.Contains(t, output, "sub/data.txt")

	// Verify absolute path is embedded
	absDir, _ := filepath.Abs(dir)
	assert.Contains(t, output, absDir)

}

func TestGenerateEmbedded(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "hello.txt"), "hello world")
	mustWriteFile(t, filepath.Join(dir, "sub", "data.json"), `{"key":"value"}`)

	files, err := fswalker.Walk(dir)
	require.Nil(t, err)

	output, err := GenerateEmbedded(files)
	require.Nil(t, err)

	// Verify associative array declaration
	assert.Contains(t, output, "declare -A __bashfs_data")

	// Verify functions
	for _, fn := range []string{"bashfs_cat()", "bashfs_extract()", "bashfs_list()", "bashfs_jq()"} {
		assert.Contains(t, output, fn)

	}

	// Verify file keys are present
	assert.Contains(t, output, `["hello.txt"]`)

	assert.Contains(t, output, `["sub/data.json"]`)

	// Verify base64 data is present (should contain = padding or alphanumeric)
	assert.Contains(t, output, "=")

}

func TestGenerateEmbeddedEmpty(t *testing.T) {
	output, err := GenerateEmbedded(nil)
	require.Nil(t, err)

	assert.Contains(t, output, "declare -A __bashfs_data")

}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))

	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

}
