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

	result, err := Package(script, dir, Options{})
	require.Nil(t, err)

	output := string(result.Data)

	// The eval line should be replaced
	assert.NotContains(t, output, "eval $(bashfs gen")

	// Embedded code should be present
	assert.Contains(t, output, "declare -A __bashfs_offset")
	assert.Contains(t, output, "bashfs_cat()")
	assert.Contains(t, output, "__bashfs_payload_sha256=")

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
	_, err := Package("#!/bin/bash\necho hello\n", "/tmp", Options{})
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "no 'eval $(bashfs gen ...)' line found")
}

func TestPackageMultipleEval(t *testing.T) {
	script := `#!/bin/bash
eval $(bashfs gen ./a)
eval $(bashfs gen ./b)
`
	_, err := Package(script, "/tmp", Options{})
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
	result, err := Package(script, dir, Options{})
	require.Nil(t, err)
	assert.Contains(t, string(result.Data), "declare -A __bashfs_offset")
}

// TestPackageQuotedEval covers the recommended shell idiom
// `eval "$(bashfs gen ...)"` (quoted command substitution). The README has
// always shown this form, but historically the matcher only accepted the
// unquoted form -- which forced users into a choice between source-mode
// correctness (quoted) and packageability (unquoted). Both must work.
func TestPackageQuotedEval(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	script := `#!/bin/bash
eval "$(bashfs gen ./myfiles)"
bashfs_cat greeting.txt
`
	result, err := Package(script, dir, Options{})
	require.NoError(t, err)
	output := string(result.Data)

	// The eval line should be replaced by the embedded block.
	assert.NotContains(t, output, `eval "$(bashfs gen`)
	assert.NotContains(t, output, "eval $(bashfs gen")
	assert.Contains(t, output, "declare -A __bashfs_offset")

	// And the packaged script must actually run.
	scriptPath := filepath.Join(dir, "packaged.sh")
	require.NoError(t, os.WriteFile(scriptPath, result.Data, 0755))
	out, err := exec.Command("bash", scriptPath).Output()
	require.NoError(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(string(out)))
}

// TestPackageQuotedEvalWithQuotedDir covers the fully quoted variant where
// both the command substitution and the directory argument are quoted.
func TestPackageQuotedEvalWithQuotedDir(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "test.txt"), "data")

	script := `#!/bin/bash
eval "$(bashfs gen "./myfiles")"
`
	result, err := Package(script, dir, Options{})
	require.NoError(t, err)
	assert.Contains(t, string(result.Data), "declare -A __bashfs_offset")
}

// TestPackageMultipleEvalMixedForms makes sure the matcher counts both
// quoted and unquoted forms when detecting "multiple eval lines" -- a user
// who has one of each in the same script still gets the conflict error
// rather than silently packaging only one of them.
func TestPackageMultipleEvalMixedForms(t *testing.T) {
	script := `#!/bin/bash
eval $(bashfs gen ./a)
eval "$(bashfs gen ./b)"
`
	_, err := Package(script, "/tmp", Options{})
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "multiple")
}

func TestPackagePreservesIndentation(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "test.txt"), "data")

	script := "#!/bin/bash\n    eval $(bashfs gen ./myfiles)\n"
	result, err := Package(script, dir, Options{})
	require.Nil(t, err)

	for _, line := range strings.Split(string(result.Data), "\n") {
		if strings.Contains(line, "declare -A __bashfs_offset") {
			assert.True(t, strings.HasPrefix(line, "    "))
			break
		}
	}
}

func TestPackageRunsDirect(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	script := `#!/bin/bash
eval $(bashfs gen ./myfiles)
bashfs_cat greeting.txt
`
	result, err := Package(script, dir, Options{})
	require.Nil(t, err)

	output := string(result.Data)
	assert.NotContains(t, output, "__bfs_self")

	scriptPath := filepath.Join(dir, "packaged.sh")
	require.NoError(t, os.WriteFile(scriptPath, result.Data, 0755))

	out, err := exec.Command("bash", scriptPath).Output()
	require.Nil(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(string(out)))
}

func TestPackageStreamablePiped(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	script := `#!/bin/bash
eval $(bashfs gen ./myfiles)
bashfs_cat greeting.txt
`
	result, err := Package(script, dir, Options{Streamable: true})
	require.Nil(t, err)

	output := string(result.Data)
	assert.Contains(t, output, "__bfs_self")

	scriptPath := filepath.Join(dir, "packaged.sh")
	require.NoError(t, os.WriteFile(scriptPath, result.Data, 0755))

	// Direct execution: BASH_SOURCE[0] is a real path, stream shim skipped.
	out, err := exec.Command("bash", scriptPath).Output()
	require.Nil(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(string(out)))

	// Piped execution (simulates curl ... | bash): BASH_SOURCE[0]="main",
	// stream shim must spool stdin to a tempfile and re-exec.
	cmd := exec.Command("bash")
	cmd.Stdin = bytes.NewReader(result.Data)
	out, err = cmd.Output()
	require.Nil(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(string(out)))
}

func TestPackageBase64Runs(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")
	mustWriteFile(t, filepath.Join(fsDir, "sub", "data.json"), `{"port":8080}`)

	script := `#!/bin/bash
eval $(bashfs gen ./myfiles)
bashfs_cat greeting.txt
bashfs_cat sub/data.json
`
	result, err := Package(script, dir, Options{Encoding: EncodingBase64})
	require.Nil(t, err)

	output := string(result.Data)

	// Sanity: base64 mode should advertise itself in the generated header.
	assert.Contains(t, output, "base64 mode")
	// Pipeline must include the base64 -d step before gzip -d.
	assert.Contains(t, output, "| base64 -d | gzip -d")
	assert.NotContains(t, output, "__bfs_self")

	// The bytes after `exit 0\n` are the trailing payload - in base64 mode
	// they MUST be printable ASCII for copy-paste through text channels to
	// work. This is the load-bearing guarantee of this mode.
	exitIdx := strings.Index(output, "\nexit 0\n")
	require.GreaterOrEqual(t, exitIdx, 0)
	payloadStart := exitIdx + len("\nexit 0\n")
	for i := payloadStart; i < len(result.Data); i++ {
		b := result.Data[i]
		ok := b == 0x09 || b == 0x0A || b == 0x0D || (b >= 0x20 && b <= 0x7E)
		require.Truef(t, ok, "non-printable byte 0x%02x at offset %d in base64-mode payload", b, i)
	}

	scriptPath := filepath.Join(dir, "packaged.sh")
	require.NoError(t, os.WriteFile(scriptPath, result.Data, 0755))

	// Direct execution.
	out, err := exec.Command("bash", scriptPath).Output()
	require.Nil(t, err)
	assert.Equal(t, `hello world{"port":8080}`, strings.TrimSpace(string(out)))
}

func TestPackageBase64StreamablePiped(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	script := `#!/bin/bash
eval $(bashfs gen ./myfiles)
bashfs_cat greeting.txt
`
	result, err := Package(script, dir, Options{Encoding: EncodingBase64, Streamable: true})
	require.Nil(t, err)

	assert.Contains(t, string(result.Data), "__bfs_self")

	scriptPath := filepath.Join(dir, "packaged.sh")
	require.NoError(t, os.WriteFile(scriptPath, result.Data, 0755))

	// Piped execution.
	cmd := exec.Command("bash")
	cmd.Stdin = bytes.NewReader(result.Data)
	out, err := cmd.Output()
	require.Nil(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(string(out)))
}

func TestPackageBase64SurvivesTextRoundTrip(t *testing.T) {
	// This is the whole point of base64 mode: take the packaged script,
	// shove it through a string-typed channel (bytes -> string -> bytes),
	// write the result back out, and confirm it still runs identically.
	// In raw mode this would corrupt the binary payload; in base64 mode
	// it must be lossless because every byte is printable ASCII.
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	script := `#!/bin/bash
eval $(bashfs gen ./myfiles)
bashfs_cat greeting.txt
`
	result, err := Package(script, dir, Options{Encoding: EncodingBase64})
	require.Nil(t, err)

	roundTripped := []byte(string(result.Data))
	assert.Equal(t, result.Data, roundTripped, "base64-mode payload must survive a string round-trip byte-for-byte")

	scriptPath := filepath.Join(dir, "pasted.sh")
	require.NoError(t, os.WriteFile(scriptPath, roundTripped, 0755))
	out, err := exec.Command("bash", scriptPath).Output()
	require.Nil(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(string(out)))
}

func TestPackageIntegrityCheckCatchesTruncation(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	script := `#!/bin/bash
eval $(bashfs gen ./myfiles)
bashfs_cat greeting.txt
`
	for _, enc := range []struct {
		name string
		opts Options
	}{
		{"raw", Options{Encoding: EncodingRaw}},
		{"base64", Options{Encoding: EncodingBase64}},
	} {
		t.Run(enc.name, func(t *testing.T) {
			result, err := Package(script, dir, enc.opts)
			require.NoError(t, err)

			truncated := result.Data[:len(result.Data)-10]
			scriptPath := filepath.Join(t.TempDir(), "truncated.sh")
			require.NoError(t, os.WriteFile(scriptPath, truncated, 0755))

			cmd := exec.Command("bash", scriptPath)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			require.Error(t, err, "truncated script should fail")
			assert.Empty(t, strings.TrimSpace(string(out)), "truncated script should produce no stdout")
			assert.Contains(t, stderr.String(), "integrity check failed")
		})
	}
}

func TestPackageIntegrityCheckCatchesCorruption(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "myfiles")
	mustWriteFile(t, filepath.Join(fsDir, "greeting.txt"), "hello world")

	script := `#!/bin/bash
eval $(bashfs gen ./myfiles)
bashfs_cat greeting.txt
`
	result, err := Package(script, dir, Options{Encoding: EncodingBase64})
	require.NoError(t, err)

	corrupted := make([]byte, len(result.Data))
	copy(corrupted, result.Data)
	corrupted[len(corrupted)-5] ^= 0xFF

	scriptPath := filepath.Join(t.TempDir(), "corrupted.sh")
	require.NoError(t, os.WriteFile(scriptPath, corrupted, 0755))

	cmd := exec.Command("bash", scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_, err = cmd.Output()
	require.Error(t, err, "corrupted script should fail")
	assert.Contains(t, stderr.String(), "integrity check failed")
}

func TestEncodingString(t *testing.T) {
	assert.Equal(t, "raw", EncodingRaw.String())
	assert.Equal(t, "base64", EncodingBase64.String())
	// Out-of-range values should self-describe rather than panic - useful
	// when an Encoding value shows up in an error message somewhere.
	assert.Equal(t, "encoding(99)", Encoding(99).String())
}

func TestEncodingSet(t *testing.T) {
	var e Encoding
	require.NoError(t, e.Set("raw"))
	assert.Equal(t, EncodingRaw, e)

	require.NoError(t, e.Set("base64"))
	assert.Equal(t, EncodingBase64, e)

	err := e.Set("hex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "raw")
	assert.Contains(t, err.Error(), "base64")
	// Failed Set must leave the value unchanged so cobra reports the bad
	// flag without silently switching the encoding to a default.
	assert.Equal(t, EncodingBase64, e)
}

func TestEncodingType(t *testing.T) {
	var e Encoding
	assert.Equal(t, "encoding", e.Type())
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}
