package profiling

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupportString(t *testing.T) {
	assert.Equal(t, "web", SupportWeb.String())
	assert.Equal(t, "local", SupportLocal.String())
	assert.Equal(t, "none", SupportNone.String())
	// Out-of-range values self-describe rather than panic.
	assert.Equal(t, "profiling-support(99)", Support(99).String())
}

func TestSupportSet(t *testing.T) {
	var s Support
	require.NoError(t, s.Set("local"))
	assert.Equal(t, SupportLocal, s)

	require.NoError(t, s.Set("none"))
	assert.Equal(t, SupportNone, s)

	require.NoError(t, s.Set("web"))
	assert.Equal(t, SupportWeb, s)

	err := s.Set("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "web")
	assert.Contains(t, err.Error(), "local")
	assert.Contains(t, err.Error(), "none")
	// A failed Set must leave the value untouched so cobra reports the bad
	// flag instead of silently switching modes.
	assert.Equal(t, SupportWeb, s)
}

func TestSupportType(t *testing.T) {
	var s Support
	assert.Equal(t, "profiling-support", s.Type())
}

// TestSupportZeroValueIsWeb pins the default: an unset Support (and thus a
// packager.Options{} with no Profiling field) means web mode, matching the CLI
// default.
func TestSupportZeroValueIsWeb(t *testing.T) {
	var s Support
	assert.Equal(t, SupportWeb, s)
}

func TestBlockNone(t *testing.T) {
	assert.Equal(t, "", Block(SupportNone))
}

func TestBlockWeb(t *testing.T) {
	b := Block(SupportWeb)
	// Guarded by the runtime env var.
	assert.Contains(t, b, `[ "${BASHFS_PROFILE_SCRIPT:-}" = "1" ]`)
	// Fetches from the public harness URL and eval's it.
	assert.Contains(t, b, HarnessURL)
	assert.Contains(t, b, "curl -fsSL")
	assert.Contains(t, b, `eval "$__bashfs_profile_src"`)
	// Web mode must NOT inline the harness body - that's the whole point.
	assert.NotContains(t, b, "__bashfs_profile_main")
	// Always stops before the user's body.
	assert.Contains(t, b, "exit 0")
}

func TestBlockLocal(t *testing.T) {
	b := Block(SupportLocal)
	assert.Contains(t, b, `[ "${BASHFS_PROFILE_SCRIPT:-}" = "1" ]`)
	// The full harness is inlined.
	assert.Contains(t, b, "__bashfs_profile_main")
	assert.Contains(t, b, "hyperfine")
	// No network/URL in local mode.
	assert.NotContains(t, b, HarnessURL)
	assert.NotContains(t, b, "curl")
	// The inlined harness must not carry its shebang into the middle of a
	// packaged script.
	assert.False(t, strings.HasPrefix(b, "#!"))
	assert.NotContains(t, b, "#!/usr/bin/env bash")
}

// TestHarnessEmbeddedMatchesFile guards against the embed and the file drifting
// apart - the web-mode URL serves the on-disk file, so the embedded copy must
// be identical to it.
func TestHarnessEmbeddedMatchesFile(t *testing.T) {
	onDisk, err := os.ReadFile("profile.sh")
	require.NoError(t, err)
	assert.Equal(t, string(onDisk), harness)
}

// TestHarnessIsValidBash runs `bash -n` over the embedded harness so a syntax
// error can never ship inside a packaged script.
func TestHarnessIsValidBash(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "harness.sh")
	require.NoError(t, os.WriteFile(p, []byte(harness), 0644))
	out, err := exec.Command("bash", "-n", p).CombinedOutput()
	require.NoErrorf(t, err, "bash -n failed: %s", out)
}
