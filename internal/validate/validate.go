package validate

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"bashfs/internal/fswalker"
)

// sourceRe matches source/. commands at the start of a line.
// Groups: 1=double-quoted arg, 2=single-quoted arg, 3=unquoted arg.
var sourceRe = regexp.MustCompile(`^\s*(?:source|\.)\s+(?:"([^"]+)"|'([^']+)'|(\S+))`)

func isShellFile(entry fswalker.FileEntry) bool {
	switch filepath.Ext(entry.RelPath) {
	case ".sh", ".bash":
		return true
	}
	f, err := os.Open(entry.AbsPath)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 128)
	n, _ := f.Read(buf)
	first := string(buf[:n])
	if nl := strings.IndexByte(first, '\n'); nl >= 0 {
		first = first[:nl]
	}
	return strings.Contains(first, "/bin/bash") ||
		strings.Contains(first, "/bin/sh") ||
		strings.Contains(first, "/usr/bin/env bash") ||
		strings.Contains(first, "/usr/bin/env sh")
}

func extractSourceArg(m []string) string {
	for _, g := range m[1:] {
		if g != "" {
			return g
		}
	}
	return ""
}

type sourceRef struct {
	Line int
	Path string
}

func findSources(content string) []sourceRef {
	var refs []sourceRef
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		trimmed := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		m := sourceRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		path := extractSourceArg(m)
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, "<(") || strings.HasPrefix(path, "$(") {
			continue
		}
		if strings.Contains(path, "$") {
			continue
		}
		if filepath.IsAbs(path) {
			continue
		}
		refs = append(refs, sourceRef{lineNum, path})
	}
	return refs
}

func bashSyntax(absPath, displayPath string) []string {
	cmd := exec.Command("bash", "-n", absPath)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	var msgs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		msgs = append(msgs, strings.ReplaceAll(line, absPath, displayPath))
	}
	if len(msgs) == 0 {
		msgs = append(msgs, fmt.Sprintf("%s: bash syntax check failed", displayPath))
	}
	return msgs
}

func runShellcheck(absPath, displayPath string) []string {
	cmd := exec.Command("shellcheck", "-f", "gcc", "-x", "-S", "warning", absPath)
	out, _ := cmd.CombinedOutput()
	var msgs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		msgs = append(msgs, strings.ReplaceAll(line, absPath, displayPath))
	}
	return msgs
}

// Run validates shell script files before packaging. It checks:
//   - bash -n syntax on all shell files in the filesystem
//   - shellcheck on all shell files (if installed)
//   - source/. commands within filesystem files resolve to other files in the filesystem
//   - source/. commands in the main script that point into the bashfs directory
func Run(entries []fswalker.FileEntry, scriptContent string, scriptDir string, fsDir string) (errors, warnings []string) {
	fsFileSet := make(map[string]bool)
	for _, e := range entries {
		fsFileSet[e.RelPath] = true
	}

	_, shellcheckErr := exec.LookPath("shellcheck")
	hasShellcheck := shellcheckErr == nil
	shellFileCount := 0

	for _, entry := range entries {
		if !isShellFile(entry) {
			continue
		}
		shellFileCount++

		if errs := bashSyntax(entry.AbsPath, entry.RelPath); len(errs) > 0 {
			errors = append(errors, errs...)
		}

		if hasShellcheck {
			if warns := runShellcheck(entry.AbsPath, entry.RelPath); len(warns) > 0 {
				warnings = append(warnings, warns...)
			}
		}

		content, err := os.ReadFile(entry.AbsPath)
		if err != nil {
			continue
		}
		entryDir := filepath.Dir(entry.RelPath)
		for _, src := range findSources(string(content)) {
			resolved := filepath.Clean(filepath.Join(entryDir, src.Path))
			if !fsFileSet[resolved] {
				errors = append(errors, fmt.Sprintf(
					"%s:%d: source '%s' resolves to '%s' which does not exist in the filesystem",
					entry.RelPath, src.Line, src.Path, resolved,
				))
			}
		}
	}

	tmpFile, err := os.CreateTemp("", "bashfs-validate-*.sh")
	if err == nil {
		_, _ = tmpFile.WriteString(scriptContent)
		tmpFile.Close()
		if errs := bashSyntax(tmpFile.Name(), "<script>"); len(errs) > 0 {
			errors = append(errors, errs...)
		}
		os.Remove(tmpFile.Name())
	}

	for _, src := range findSources(scriptContent) {
		resolved := filepath.Clean(filepath.Join(scriptDir, src.Path))
		relToFs, relErr := filepath.Rel(fsDir, resolved)
		if relErr != nil {
			continue
		}
		if strings.HasPrefix(relToFs, "..") {
			continue
		}
		if fsFileSet[relToFs] {
			errors = append(errors, fmt.Sprintf(
				"<script>:%d: source '%s' references '%s' inside the bashfs filesystem; use 'source <(bashfs_cat %s)' instead",
				src.Line, src.Path, relToFs, relToFs,
			))
		}
	}

	if !hasShellcheck && shellFileCount > 0 {
		warnings = append(warnings, "shellcheck not found; install it for additional validation")
	}

	return errors, warnings
}
