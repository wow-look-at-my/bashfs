package bashgen

import (
	"fmt"
	"path/filepath"
	"strings"

	"bashfs/internal/fswalker"
)

// GenerateDevMode produces bash code that references real files on disk.
// The returned code defines bashfs_cat, bashfs_extract, bashfs_list, and bashfs_jq functions.
//
// The output is laid out so that BOTH `eval "$(bashfs gen <dir>)"` and the
// unquoted `eval $(bashfs gen <dir>)` work. Without quotes, bash word-splits
// the command substitution on IFS (space, tab, newline) before eval sees it,
// then eval re-joins the words with spaces. To survive that round-trip:
//   - No leading or interior comments -- a `#` would swallow every following
//     word and silently leave no functions defined.
//   - Statements separated by `;`, never just by newlines.
//   - A trailing `;` after each closing `}` so two adjacent function
//     definitions don't fuse into a single un-parseable command after
//     newlines collapse to spaces.
func GenerateDevMode(files []fswalker.FileEntry, baseDir string) (string, error) {
	absDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolving base directory: %w", err)
	}

	var b strings.Builder

	fmt.Fprintf(&b, `bashfs_cat() { local _bashfs_file="%s/$1"; if [ ! -f "$_bashfs_file" ]; then echo "bashfs: file not found: $1" >&2; return 1; fi; cat "$_bashfs_file"; };`+"\n", absDir)

	b.WriteString(`bashfs_extract() { mkdir -p "$(dirname "$2")" && bashfs_cat "$1" > "$2"; };` + "\n")

	b.WriteString("bashfs_list() {")
	if len(files) == 0 {
		// bash rejects an empty function body, so emit a no-op for the
		// zero-file case.
		b.WriteString(" :;")
	}
	for _, f := range files {
		fmt.Fprintf(&b, " echo '%s';", f.RelPath)
	}
	b.WriteString(" };\n")

	fmt.Fprintf(&b, `bashfs_jq() { local _bashfs_file="%s/$1"; if [ ! -f "$_bashfs_file" ]; then echo "bashfs: file not found: $1" >&2; return 1; fi; jq "${2:-.}" "$_bashfs_file"; };`+"\n", absDir)

	return b.String(), nil
}
