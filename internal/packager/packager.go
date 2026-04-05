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

// Package reads a bash script and replaces eval $(bashfs gen <dir>) lines
// with embedded filesystem data and self-contained helper functions.
func Package(scriptContent string, scriptDir string) (string, error) {
	lines := strings.Split(scriptContent, "\n")
	var matchIdx []int

	for i, line := range lines {
		if evalPattern.MatchString(line) {
			matchIdx = append(matchIdx, i)
		}
	}

	if len(matchIdx) == 0 {
		return "", fmt.Errorf("no 'eval $(bashfs gen ...)' line found in script")
	}
	if len(matchIdx) > 1 {
		return "", fmt.Errorf("multiple 'eval $(bashfs gen ...)' lines found (lines %v); only one is supported", matchIdx)
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
		return "", fmt.Errorf("walking directory %s: %w", dirArg, err)
	}

	embedded, err := bashgen.GenerateEmbedded(files)
	if err != nil {
		return "", fmt.Errorf("generating embedded code: %w", err)
	}

	// Indent each line of the embedded code to match the original eval line
	embeddedLines := strings.Split(embedded, "\n")
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

	return strings.Join(result, "\n"), nil
}
