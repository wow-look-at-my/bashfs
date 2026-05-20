package bashgen

import (
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"bashfs/internal/fswalker"
)

type devModeData struct {
	AbsDir string
	Files  []fswalker.FileEntry
}

var devModeTmpl = template.Must(template.New("devmode").Parse(devModeTemplate))

// Each function is on a single line ending with `};` so that the output
// survives the unquoted `eval $(bashfs gen ...)` form. Without quotes, bash
// word-splits on IFS before eval sees the text, so no `#` comments and no
// bare `}` without a trailing `;` (which would fuse with the next token).
const devModeTemplate = `bashfs_cat() { local _bashfs_file="{{.AbsDir}}/$1"; if [ ! -f "$_bashfs_file" ]; then echo "bashfs: file not found: $1" >&2; return 1; fi; cat "$_bashfs_file"; };
bashfs_extract() { mkdir -p "$(dirname "$2")" && bashfs_cat "$1" > "$2"; };
bashfs_list() {
{{- if not .Files}} :;{{else}}{{range .Files}} echo '{{.RelPath}}';{{end}}{{end}} };
bashfs_jq() { bashfs_cat "$1" | jq "${2:-.}"; };
`

// GenerateDevMode produces bash code that references real files on disk.
// The returned code defines bashfs_cat, bashfs_extract, bashfs_list, and
// bashfs_jq functions.
//
// The output is laid out so that BOTH `eval "$(bashfs gen <dir>)"` and the
// unquoted `eval $(bashfs gen <dir>)` work. See devModeTemplate for details.
func GenerateDevMode(files []fswalker.FileEntry, baseDir string) (string, error) {
	absDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolving base directory: %w", err)
	}

	var b strings.Builder
	err = devModeTmpl.Execute(&b, devModeData{
		AbsDir: absDir,
		Files:  files,
	})
	if err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return b.String(), nil
}
