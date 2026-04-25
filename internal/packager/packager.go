package packager

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"bashfs/internal/bashgen"
	"bashfs/internal/fswalker"
)

var evalPattern = regexp.MustCompile(`^(\s*)eval\s+\$\(bashfs\s+gen\s+(.+?)\)\s*$`)

// Encoding selects how the trailing payload is laid out at the end of the
// packaged script.
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

// Options controls Package behavior. Adding fields here is the extension
// point for future flags (e.g. compression off) without breaking the
// signature again.
type Options struct {
	Encoding Encoding
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
	dirArg := matches[2]

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

	// Inject auto-bootstrap trampoline so `curl ... | bash` Just Works.
	// When piped via stdin, ${BASH_SOURCE[0]} is empty/main, so the helpers
	// can't tail -c the payload off disk. Bash reads piped scripts
	// line-by-line, so at trampoline time the rest of the script + payload
	// is still on stdin — we spool it to a tempfile and re-exec.
	// The runtime offset calc in embedded.go (filesize - payload_size) makes
	// this safe regardless of how the trampoline reshapes the prefix, and
	// `cat` in the trampoline doesn't care whether the payload is binary
	// (raw mode) or printable text (base64 mode).
	scriptText = injectTrampoline(scriptText)

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

const trampoline = `# bashfs auto-bootstrap: re-exec from a real file when piped via stdin.
# When piped (curl ... | bash), BASH_SOURCE[0] is empty at top level, so
# the helpers (which run inside functions where it becomes "main") can't
# tail -c the payload off disk. Spool the rest of stdin to a tempfile
# and re-exec — bash reads piped scripts line-by-line, so at this point
# the rest of the script + payload is still queued on stdin.
if [ -z "${BASH_SOURCE[0]:-}" ] || ! [ -r "${BASH_SOURCE[0]}" ]; then
  __bfs_self=$(mktemp) || { echo "bashfs: mktemp failed" >&2; exit 1; }
  { printf '#!/bin/bash\n: bashfs re-exec stub\n'; cat; } > "$__bfs_self" && exec bash "$__bfs_self" "$@"
  exit 1
fi
`

// injectTrampoline inserts the auto-bootstrap block after the shebang line
// (or at the top if there isn't one).
func injectTrampoline(scriptText string) string {
	if strings.HasPrefix(scriptText, "#!") {
		if nl := strings.Index(scriptText, "\n"); nl >= 0 {
			return scriptText[:nl+1] + trampoline + scriptText[nl+1:]
		}
		return scriptText + "\n" + trampoline
	}
	return trampoline + scriptText
}
