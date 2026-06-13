// Package profiling bakes an optional, hyperfine-based "profiling mode" into
// packaged bashfs scripts.
//
// When a script is packaged with profiling support (the default), it gains a
// guarded block that does nothing on a normal run but, when the environment
// variable BASHFS_PROFILE_SCRIPT=1 is set, benchmarks the parts of the script
// that bashfs owns -- the startup integrity check and each embedded file's
// extraction -- using hyperfine, then exits without running the user's body.
//
// The harness itself lives in profile.sh. Two delivery modes keep the packaged
// script small while still supporting offline use:
//
//   - web (default): only a tiny stub is embedded; it curls profile.sh from the
//     public repo at runtime, so the packaged script barely grows.
//   - local: the whole harness is embedded, so profiling works with no network.
//   - none: no profiling support at all.
package profiling

import (
	_ "embed"
	"fmt"
	"strings"
)

// Support selects how profiling support is baked into a packaged script.
//
// Support implements pflag.Value (String/Set/Type) so cobra can bind it
// directly via Flags().Var, enforcing the web|local|none choice at parse time.
type Support int

const (
	// SupportWeb embeds only a small stub that downloads the profiling harness
	// from the public repo at runtime (and only when BASHFS_PROFILE_SCRIPT=1).
	// This keeps the packaged script tiny -- the default.
	SupportWeb Support = iota

	// SupportLocal embeds the full profiling harness in the packaged script, so
	// profiling works with no network access at the cost of a larger script.
	SupportLocal

	// SupportNone bakes in no profiling support at all; the packaged script is
	// byte-for-byte what it would be without this feature.
	SupportNone
)

// Supports lists the valid Support string values, in user-facing order.
// Cobra uses this for shell completion of --profiling-support.
var Supports = []string{"web", "local", "none"}

// String returns the canonical CLI name of the support mode.
func (s Support) String() string {
	switch s {
	case SupportWeb:
		return "web"
	case SupportLocal:
		return "local"
	case SupportNone:
		return "none"
	default:
		return fmt.Sprintf("profiling-support(%d)", int(s))
	}
}

// Set parses a CLI value into the Support. Implements pflag.Value.
func (s *Support) Set(v string) error {
	switch v {
	case "web":
		*s = SupportWeb
	case "local":
		*s = SupportLocal
	case "none":
		*s = SupportNone
	default:
		return fmt.Errorf("must be one of: %s", strings.Join(Supports, ", "))
	}
	return nil
}

// Type returns the type name shown in --help. Implements pflag.Value.
func (s *Support) Type() string { return "profiling-support" }

// HarnessURL is where the web-mode stub downloads the harness from at runtime.
// It must serve the very same profile.sh that is embedded below, on a public
// ref so an unauthenticated curl can reach it.
const HarnessURL = "https://raw.githubusercontent.com/wow-look-at-my/bashfs/master/internal/profiling/profile.sh"

//go:embed profile.sh
var harness string

// Block returns the bash snippet to inject into a packaged script for the given
// support mode. The snippet is a no-op unless BASHFS_PROFILE_SCRIPT=1, and it
// always exits the script after profiling so profiling mode never falls through
// to the user's body. An empty string means "no profiling support".
//
// The injected block assumes the surrounding bashfs runtime is already defined
// (the offset array, __bashfs_payload_start, bashfs_list, __bashfs_self and
// __bashfs_decode), which bashgen emits whenever this block is non-empty.
func Block(s Support) string {
	switch s {
	case SupportLocal:
		return localBlock()
	case SupportWeb:
		return webBlock()
	default: // SupportNone and anything unexpected
		return ""
	}
}

// localBlock inlines the entire harness, guarded by the env check.
func localBlock() string {
	body := stripShebang(harness)
	var b strings.Builder
	b.WriteString("if [ \"${BASHFS_PROFILE_SCRIPT:-}\" = \"1\" ]; then\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	// Safety net: the harness exits on every path, but if a future edit forgot
	// to, this guarantees profiling mode still never runs the user's body.
	b.WriteString("  exit 0\nfi\n")
	return b.String()
}

// webBlock embeds a stub that downloads the harness and eval's it. The harness
// runs in this same shell, so it sees the bashfs runtime defined above it.
func webBlock() string {
	return fmt.Sprintf(`if [ "${BASHFS_PROFILE_SCRIPT:-}" = "1" ]; then
  if ! command -v curl >/dev/null 2>&1; then
    echo "bashfs: profiling (web mode) requires curl to fetch the harness" >&2
    exit 1
  fi
  if ! __bashfs_profile_src=$(curl -fsSL "%[1]s"); then
    echo "bashfs: profiling: failed to download the harness from %[1]s" >&2
    exit 1
  fi
  eval "$__bashfs_profile_src"
  exit 0
fi
`, HarnessURL)
}

// stripShebang drops a leading #!... line so the inlined harness doesn't leave
// a stray shebang in the middle of the packaged script.
func stripShebang(s string) string {
	if strings.HasPrefix(s, "#!") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			return s[i+1:]
		}
	}
	return s
}
