package fswalker

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Nil(t, err)

	// Should find a.txt, sub/b.txt, sub/deep/c.json but skip hidden files/dirs
	want := []string{"a.txt", "sub/b.txt", "sub/deep/c.json"}
	require.Equal(t, len(want), len(entries))

	for i, w := range want {
		assert.Equal(t, w, entries[i].RelPath)

		assert.True(t, filepath.IsAbs(entries[i].AbsPath))

	}
}

func TestWalkEmpty(t *testing.T) {
	dir := t.TempDir()
	entries, err := Walk(dir)
	require.Nil(t, err)

	require.Equal(t, 0, len(entries))

}

func TestWalkNonexistent(t *testing.T) {
	_, err := Walk("/nonexistent/path")
	require.NotNil(t, err)

}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))

	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

}
