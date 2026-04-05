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

// Result holds the packaged script text and binary payload.
type Result struct {
	Data []byte // complete output: script text + exit guard + binary payload
}

// Package reads a bash script and replaces eval $(bashfs gen <dir>) lines
// with embedded filesystem data and self-contained helper functions.
// The binary payload is appended after an "exit 0" guard.
func Package(scriptContent string, scriptDir string) (*Result, error) {
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

	embedded, err := bashgen.GenerateEmbedded(files)
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

	// Compute the 1-indexed byte offset of the payload within the final file.
	// Replace the fixed-width placeholder with the actual offset.
	payloadOffset := len(scriptText) + 1 // tail -c + is 1-indexed
	replacement := fmt.Sprintf("0x%08x", payloadOffset)
	scriptText = strings.Replace(scriptText, bashgen.OffsetPlaceholder, replacement, 1)

	// Combine script text + binary payload
	out := make([]byte, 0, len(scriptText)+len(embedded.Payload))
	out = append(out, []byte(scriptText)...)
	out = append(out, embedded.Payload...)

	return &Result{Data: out}, nil
}
