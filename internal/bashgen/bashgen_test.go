package bashgen

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

	// Word-split safety: each function definition must end with `;` so
	// that adjacent defs don't fuse together when bash word-splits the
	// command substitution in the unquoted `eval $(bashfs gen ...)` form
	// and re-joins the words with spaces. Also: no `#` may appear before
	// the first function, or it would swallow every following word.
	hashIdx := strings.Index(output, "#")
	if hashIdx >= 0 {
		firstFnIdx := strings.Index(output, "bashfs_cat()")
		require.Greaterf(t, hashIdx, firstFnIdx,
			"a `#` appears before the first function def at offset %d (firstFn=%d); the unquoted eval form would silently no-op",
			hashIdx, firstFnIdx)
	}
	for _, fn := range []string{"bashfs_cat", "bashfs_extract", "bashfs_list", "bashfs_jq"} {
		// Each function's body opens with `{` and the closing brace must
		// be followed by `;` so the next word after newline-collapse is
		// a separator, not glued onto the next function name.
		fnStart := strings.Index(output, fn+"()")
		require.GreaterOrEqualf(t, fnStart, 0, "%s missing", fn)
		// Find the matching `};` (closing brace + statement separator)
		// somewhere after the function name on the same logical line.
		tail := output[fnStart:]
		nl := strings.Index(tail, "\n")
		require.GreaterOrEqualf(t, nl, 0, "%s has no terminating newline", fn)
		line := tail[:nl]
		require.Truef(t, strings.HasSuffix(line, "};"),
			"%s line does not end with `};` (got %q); adjacent def would fuse after word-split",
			fn, line)
	}
}

// TestGenerateDevModeRunsUnderEvalBothForms is the load-bearing regression
// test for the multi-line-eval bug: the dev-mode output must work whether
// the caller writes `eval "$(...)"` (quoted, recommended) or the unquoted
// `eval $(...)` form. Historically the unquoted form silently no-op'd
// because bash word-split the multi-line output and the leading `#` comment
// swallowed the rest, leaving no functions defined. This test runs a real
// bash and asserts the helpers actually produce output in both forms.
func TestGenerateDevModeRunsUnderEvalBothForms(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "greeting.txt"), "hello world")
	mustWriteFile(t, filepath.Join(dir, "sub", "data.json"), `{"port":8080}`)

	files, err := fswalker.Walk(dir)
	require.NoError(t, err)

	output, err := GenerateDevMode(files, dir)
	require.NoError(t, err)

	cases := []struct {
		name   string
		evalIn string
	}{
		// Quoted: command substitution preserves whitespace, so even if
		// the generator emitted multi-line code this would Just Work.
		{"quoted", `eval "$GEN"`},
		// Unquoted: bash word-splits $GEN before eval sees it. This is
		// the form the bug report fired on.
		{"unquoted", `eval $GEN`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Trailing `echo` after each bashfs_cat forces a newline
			// since the test fixtures don't end in one. bashfs_extract
			// is exercised separately so the helpers are all hit.
			extractDest := filepath.Join(t.TempDir(), "extracted.txt")
			script := fmt.Sprintf(`set -euo pipefail
GEN=%s
%s
bashfs_cat greeting.txt; echo
bashfs_cat sub/data.json; echo
bashfs_list
bashfs_extract greeting.txt %s
cat %s; echo
`, shellSingleQuote(output), tc.evalIn,
				shellSingleQuote(extractDest), shellSingleQuote(extractDest))

			cmd := exec.Command("bash", "-c", script)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			require.NoErrorf(t, err, "bash failed (stderr=%q)", stderr.String())

			got := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
			want := []string{
				"hello world",
				`{"port":8080}`,
				"greeting.txt",
				"sub/data.json",
				"hello world",
			}
			assert.Equal(t, want, got, "stderr=%q", stderr.String())
		})
	}
}

// shellSingleQuote wraps s in single quotes for safe inclusion in a bash
// script literal, escaping any embedded single quotes.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
