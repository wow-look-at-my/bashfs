package bashgen

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

	result, err := GenerateEmbedded(files)
	require.Nil(t, err)

	// Verify offset array declaration
	assert.Contains(t, result.Script, "declare -A __bashfs_offset")

	// Verify functions
	for _, fn := range []string{"bashfs_cat()", "bashfs_extract()", "bashfs_list()", "bashfs_jq()"} {
		assert.Contains(t, result.Script, fn)
	}

	// Verify file keys are present
	assert.Contains(t, result.Script, `["hello.txt"]`)
	assert.Contains(t, result.Script, `["sub/data.json"]`)

	// Verify payload size is embedded (used for runtime offset calc)
	assert.Contains(t, result.Script, "__bashfs_payload_size=")

	// Verify binary payload is non-empty
	assert.NotEmpty(t, result.Payload)
}

func TestGenerateEmbeddedEmpty(t *testing.T) {
	result, err := GenerateEmbedded(nil)
	require.Nil(t, err)

	assert.Contains(t, result.Script, "declare -A __bashfs_offset")
	assert.Empty(t, result.Payload)
}

func TestGenerateEmbeddedBase64(t *testing.T) {
	dir := t.TempDir()
	helloContent := []byte("hello world")
	jsonContent := []byte(`{"key":"value"}`)
	mustWriteFile(t, filepath.Join(dir, "hello.txt"), string(helloContent))
	mustWriteFile(t, filepath.Join(dir, "sub", "data.json"), string(jsonContent))

	files, err := fswalker.Walk(dir)
	require.Nil(t, err)

	result, err := GenerateEmbeddedBase64(files)
	require.Nil(t, err)

	// Header advertises base64 mode.
	assert.Contains(t, result.Script, "(base64 mode)")
	// Pipeline includes the base64 -d step.
	assert.Contains(t, result.Script, "| base64 -d | gzip -d")
	// Standard scaffolding is identical to raw mode.
	assert.Contains(t, result.Script, "declare -A __bashfs_offset")
	assert.Contains(t, result.Script, "__bashfs_payload_size=")
	for _, fn := range []string{"bashfs_cat()", "bashfs_extract()", "bashfs_list()", "bashfs_jq()"} {
		assert.Contains(t, result.Script, fn)
	}

	// Payload must be pure base64 alphabet - that's what makes it
	// copy-paste safe through text-only channels.
	require.NotEmpty(t, result.Payload)
	b64Alpha := regexp.MustCompile(`^[A-Za-z0-9+/=]+$`)
	require.Truef(t, b64Alpha.Match(result.Payload),
		"payload contains non-base64 byte; got %q", string(result.Payload))

	// Load-bearing round-trip: for every file in the offset map, slice
	// the payload using just that file's offset+length and decode it in
	// isolation. If per-file alignment is wrong, base64.DecodeString here
	// will fail. This is the test that guards the per-file-encoding choice.
	offsets := parseOffsetMap(t, result.Script)
	want := map[string][]byte{
		"hello.txt":     helloContent,
		"sub/data.json": jsonContent,
	}
	require.Len(t, offsets, len(want))
	for path, expected := range want {
		off, ok := offsets[path]
		require.Truef(t, ok, "missing %q in offset map", path)
		require.LessOrEqualf(t, off.start+off.length, len(result.Payload),
			"offset %d+%d for %q runs past payload end %d", off.start, off.length, path, len(result.Payload))

		chunk := result.Payload[off.start : off.start+off.length]
		gz, err := base64.StdEncoding.DecodeString(string(chunk))
		require.NoErrorf(t, err, "isolated base64 decode failed for %q (chunk=%q)", path, string(chunk))

		r, err := gzip.NewReader(bytes.NewReader(gz))
		require.NoErrorf(t, err, "gzip header invalid for %q", path)
		got, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, expected, got, "round-trip mismatch for %q", path)
	}
}

func TestGenerateEmbeddedBase64Empty(t *testing.T) {
	result, err := GenerateEmbeddedBase64(nil)
	require.Nil(t, err)

	assert.Contains(t, result.Script, "(base64 mode)")
	assert.Contains(t, result.Script, "| base64 -d | gzip -d")
	assert.Empty(t, result.Payload)
}

type offsetEntry struct {
	start  int
	length int
}

// parseOffsetMap extracts the ["path"]="offset:length" entries from the
// generated script's declare -A __bashfs_offset block. Used by the round-trip
// test to drive base64 slicing exactly the way the bash runtime would.
func parseOffsetMap(t *testing.T, script string) map[string]offsetEntry {
	t.Helper()
	re := regexp.MustCompile(`\["([^"]+)"\]="(\d+):(\d+)"`)
	out := map[string]offsetEntry{}
	for _, m := range re.FindAllStringSubmatch(script, -1) {
		off, err := strconv.Atoi(m[2])
		require.NoError(t, err)
		ln, err := strconv.Atoi(m[3])
		require.NoError(t, err)
		out[m[1]] = offsetEntry{start: off, length: ln}
	}
	return out
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))

	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

}
