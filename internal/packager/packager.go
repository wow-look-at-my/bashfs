package packager

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"bashfs/internal/bashgen"
	"bashfs/internal/fswalker"
	"bashfs/internal/validate"
)

// evalPattern matches the eval line that bashfs package replaces with the
// embedded payload. Both the quoted and unquoted forms are accepted:
//
//	eval $(bashfs gen <dir>)
//	eval "$(bashfs gen <dir>)"
//
// The quoted form is the recommended shell idiom (and what the README shows)
// because it preserves the multi-line dev-mode output verbatim. The unquoted
// form also works at runtime now that bashfs gen emits word-split-safe bash,
// but historically only the unquoted form was accepted here -- which left
// users who followed the README unable to package their script. The two
// alternation branches capture the directory in groups 2 and 3 respectively.
var evalPattern = regexp.MustCompile(`^(\s*)eval\s+(?:"\$\(bashfs\s+gen\s+(.+?)\)"|\$\(bashfs\s+gen\s+(.+?)\))\s*$`)

// Encoding selects how the trailing payload is laid out at the end of the
// packaged script.
//
// Encoding implements pflag.Value (String/Set/Type) so cobra can bind it
// directly via Flags().Var, enforcing the raw|base64 choice at parse time
// and surfacing it in --help.
type Encoding int

const (
	// EncodingRaw appends the gzipped payload as raw binary bytes (smallest
	// on disk, but breaks any text-only round-trip like copy-paste through
	// chat or web forms).
	EncodingRaw Encoding = iota

	// EncodingBase64 appends the gzipped payload as concatenated per-file
	// base64 chunks (~33% larger, but printable ASCII so the script
	// survives copy-paste through any text channel).
	EncodingBase64
)

// Encodings lists the valid Encoding string values, in user-facing order.
// Cobra uses this for shell completion of --encoding.
var Encodings = []string{"raw", "base64"}

// String returns the canonical CLI name of the encoding.
func (e Encoding) String() string {
	switch e {
	case EncodingRaw:
		return "raw"
	case EncodingBase64:
		return "base64"
	default:
		return fmt.Sprintf("encoding(%d)", int(e))
	}
}

// Set parses a CLI value into the Encoding. Implements pflag.Value.
func (e *Encoding) Set(s string) error {
	switch s {
	case "raw":
		*e = EncodingRaw
	case "base64":
		*e = EncodingBase64
	default:
		return fmt.Errorf("must be one of: %s", strings.Join(Encodings, ", "))
	}
	return nil
}

// Type returns the type name shown in --help (e.g. "--encoding encoding").
// Implements pflag.Value.
func (e *Encoding) Type() string { return "encoding" }

// Options controls Package behavior.
type Options struct {
	Encoding       Encoding
	SkipValidation bool
}

// Result holds the packaged script text and binary payload.
type Result struct {
	Data []byte // complete output: script text + exit guard + binary payload
}

// Package reads a bash script and replaces eval $(bashfs gen <dir>) lines
// with embedded filesystem data and self-contained helper functions.
// The payload is appended after an "exit 0" guard.
func Package(scriptContent string, scriptDir string, opts Options) (*Result, error) {
	lines := strings.Split(scriptContent, "\n")
	var matchIdx []int

	for i, line := range lines {
		if evalPattern.MatchString(line) {
			matchIdx = append(matchIdx, i)
		}
	}

	if len(matchIdx) == 0 {
		return nil, fmt.Errorf("no 'eval $(bashfs gen ...)' line found in script")
	}
	if len(matchIdx) > 1 {
		return nil, fmt.Errorf("multiple 'eval $(bashfs gen ...)' lines found (lines %v); only one is supported", matchIdx)
	}

	idx := matchIdx[0]
	matches := evalPattern.FindStringSubmatch(lines[idx])
	indent := matches[1]
	// matches[2] is the quoted form's dir, matches[3] is the unquoted
	// form's dir; exactly one is populated by the alternation in
	// evalPattern.
	dirArg := matches[2]
	if dirArg == "" {
		dirArg = matches[3]
	}

	// Strip surrounding quotes if present
	dirArg = strings.Trim(dirArg, `"'`)

	// Resolve relative to script directory
	if !filepath.IsAbs(dirArg) {
		dirArg = filepath.Join(scriptDir, dirArg)
	}
	dirArg = filepath.Clean(dirArg)

	files, err := fswalker.Walk(dirArg)
	if err != nil {
		return nil, fmt.Errorf("walking directory %s: %w", dirArg, err)
	}

	if !opts.SkipValidation {
		errs, warnings := validate.Run(files, scriptContent, scriptDir, dirArg)
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "bashfs: warning: %s\n", w)
		}
		if len(errs) > 0 {
			var b strings.Builder
			b.WriteString("validation failed:\n")
			for _, e := range errs {
				b.WriteString("  ")
				b.WriteString(e)
				b.WriteString("\n")
			}
			return nil, fmt.Errorf("%s", b.String())
		}
	}

	embedded, err := generate(files, opts.Encoding)
	if err != nil {
		return nil, fmt.Errorf("generating embedded code: %w", err)
	}

	// Indent each line of the embedded code to match the original eval line
	embeddedLines := strings.Split(embedded.Script, "\n")
	var indented []string
	for _, el := range embeddedLines {
		if el == "" {
			indented = append(indented, "")
		} else {
			indented = append(indented, indent+el)
		}
	}

	// Replace the eval line with the embedded block
	var result []string
	result = append(result, lines[:idx]...)
	result = append(result, indented...)
	result = append(result, lines[idx+1:]...)

	// Append exit guard so the shell never tries to parse the binary payload
	scriptText := strings.Join(result, "\n")
	if !strings.HasSuffix(scriptText, "\n") {
		scriptText += "\n"
	}
	scriptText += "exit 0\n"

	// Inject the stream shim so `curl ... | bash` Just Works.
	// When piped via stdin, ${BASH_SOURCE[0]} is empty/main, so the helpers
	// can't tail -c the payload off disk. Bash reads piped scripts
	// line-by-line, so at shim time the rest of the script + payload is
	// still on stdin - we spool it to a tempfile and re-exec.
	// The runtime offset calc in embedded.go (filesize - payload_size) makes
	// this safe regardless of how the shim reshapes the prefix, and `cat`
	// in the shim doesn't care whether the payload is binary (raw mode) or
	// printable text (base64 mode).
	scriptText = injectStreamable(scriptText)

	// Combine script text + payload (raw bytes or base64 ASCII)
	out := make([]byte, 0, len(scriptText)+len(embedded.Payload))
	out = append(out, []byte(scriptText)...)
	out = append(out, embedded.Payload...)

	return &Result{Data: out}, nil
}

func generate(files []fswalker.FileEntry, enc Encoding) (*bashgen.EmbeddedResult, error) {
	switch enc {
	case EncodingRaw:
		return bashgen.GenerateEmbedded(files)
	case EncodingBase64:
		return bashgen.GenerateEmbeddedBase64(files)
	default:
		return nil, fmt.Errorf("unknown encoding %d", enc)
	}
}

const streamShim = `if [ -z "${BASH_SOURCE[0]:-}" ] || ! [ -r "${BASH_SOURCE[0]}" ]; then
  __bfs_self=$(mktemp) || { echo "bashfs: mktemp failed" >&2; exit 1; }
  { printf '#!/bin/bash\n: bashfs re-exec stub\n'; cat; } > "$__bfs_self" && exec bash "$__bfs_self" "$@"
  exit 1
fi
`

// injectStreamable inserts the auto-bootstrap block after the shebang line
// (or at the top if there isn't one).
func injectStreamable(scriptText string) string {
	if strings.HasPrefix(scriptText, "#!") {
		if nl := strings.Index(scriptText, "\n"); nl >= 0 {
			return scriptText[:nl+1] + streamShim + scriptText[nl+1:]
		}
		return scriptText + "\n" + streamShim
	}
	return streamShim + scriptText
}
