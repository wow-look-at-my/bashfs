package validate

import (
	"os"
	"path/filepath"
	"testing"

	"bashfs/internal/fswalker"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func TestBashSyntaxValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "good.sh")
	writeFile(t, path, "#!/bin/bash\necho hello\n")

	msgs := bashSyntax(path, "good.sh")
	assert.Empty(t, msgs)
}

func TestBashSyntaxInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.sh")
	writeFile(t, path, "#!/bin/bash\nif true\necho broken\n")

	msgs := bashSyntax(path, "bad.sh")
	assert.NotEmpty(t, msgs)
	assert.Contains(t, msgs[0], "bad.sh")
}

func TestIsShellFileByExtension(t *testing.T) {
	dir := t.TempDir()
	shPath := filepath.Join(dir, "script.sh")
	writeFile(t, shPath, "echo hello")
	bashPath := filepath.Join(dir, "script.bash")
	writeFile(t, bashPath, "echo hello")
	txtPath := filepath.Join(dir, "data.txt")
	writeFile(t, txtPath, "just data")

	assert.True(t, isShellFile(fswalker.FileEntry{RelPath: "script.sh", AbsPath: shPath}))
	assert.True(t, isShellFile(fswalker.FileEntry{RelPath: "script.bash", AbsPath: bashPath}))
	assert.False(t, isShellFile(fswalker.FileEntry{RelPath: "data.txt", AbsPath: txtPath}))
}

func TestIsShellFileByShebang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "myscript")
	writeFile(t, path, "#!/bin/bash\necho hello\n")

	assert.True(t, isShellFile(fswalker.FileEntry{RelPath: "myscript", AbsPath: path}))
}

func TestIsShellFileNonShell(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeFile(t, path, `{"port": 8080}`)

	assert.False(t, isShellFile(fswalker.FileEntry{RelPath: "config.json", AbsPath: path}))
}

func TestFindSourcesBasic(t *testing.T) {
	content := `#!/bin/bash
source ./lib.sh
. ./utils.sh
echo hello
`
	refs := findSources(content)
	require.Len(t, refs, 2)
	assert.Equal(t, "./lib.sh", refs[0].Path)
	assert.Equal(t, 2, refs[0].Line)
	assert.Equal(t, "./utils.sh", refs[1].Path)
	assert.Equal(t, 3, refs[1].Line)
}

func TestFindSourcesQuoted(t *testing.T) {
	content := `#!/bin/bash
source "./lib.sh"
. './utils.sh'
`
	refs := findSources(content)
	require.Len(t, refs, 2)
	assert.Equal(t, "./lib.sh", refs[0].Path)
	assert.Equal(t, "./utils.sh", refs[1].Path)
}

func TestFindSourcesSkipsProcessSubstitution(t *testing.T) {
	content := `#!/bin/bash
source <(bashfs_cat lib.sh)
. <(some_command)
`
	refs := findSources(content)
	assert.Empty(t, refs)
}

func TestFindSourcesSkipsVariables(t *testing.T) {
	content := `#!/bin/bash
source "$DIR/lib.sh"
. "${SCRIPT_DIR}/utils.sh"
`
	refs := findSources(content)
	assert.Empty(t, refs)
}

func TestFindSourcesSkipsAbsolutePaths(t *testing.T) {
	content := `#!/bin/bash
source /etc/profile
. /usr/lib/bash/helpers.sh
`
	refs := findSources(content)
	assert.Empty(t, refs)
}

func TestFindSourcesSkipsComments(t *testing.T) {
	content := `#!/bin/bash
# source ./lib.sh
  # . ./utils.sh
`
	refs := findSources(content)
	assert.Empty(t, refs)
}

func TestRunNoShellFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data.json"), `{"key": "value"}`)
	writeFile(t, filepath.Join(dir, "readme.txt"), "hello")

	entries, err := fswalker.Walk(dir)
	require.NoError(t, err)

	script := "#!/bin/bash\neval $(bashfs gen ./myfiles)\necho hello\n"
	errs, _ := Run(entries, script, dir, dir)
	assert.Empty(t, errs)
}

func TestRunCatchesBadSyntax(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken.sh"), "#!/bin/bash\nif true\necho broken\n")

	entries, err := fswalker.Walk(dir)
	require.NoError(t, err)

	script := "#!/bin/bash\neval $(bashfs gen ./fs)\necho hello\n"
	errs, _ := Run(entries, script, dir, dir)
	assert.NotEmpty(t, errs)

	found := false
	for _, e := range errs {
		if filepath.Base(e) != "" {
			found = true
		}
	}
	assert.True(t, found || len(errs) > 0, "should report syntax errors")
}

func TestRunCatchesUnresolvedSource(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.sh"), "#!/bin/bash\nsource ./missing.sh\necho hello\n")

	entries, err := fswalker.Walk(dir)
	require.NoError(t, err)

	script := "#!/bin/bash\neval $(bashfs gen ./fs)\necho hello\n"
	errs, _ := Run(entries, script, dir, dir)

	found := false
	for _, e := range errs {
		if contains(e, "missing.sh") && contains(e, "does not exist") {
			found = true
		}
	}
	assert.True(t, found, "should report unresolved source: %v", errs)
}

func TestRunAllowsResolvedSource(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.sh"), "#!/bin/bash\nsource ./lib.sh\necho hello\n")
	writeFile(t, filepath.Join(dir, "lib.sh"), "#!/bin/bash\necho lib\n")

	entries, err := fswalker.Walk(dir)
	require.NoError(t, err)

	script := "#!/bin/bash\neval $(bashfs gen ./fs)\necho hello\n"
	errs, _ := Run(entries, script, dir, dir)

	for _, e := range errs {
		assert.NotContains(t, e, "lib.sh")
	}
}

func TestRunCatchesScriptSourceIntoFsDir(t *testing.T) {
	dir := t.TempDir()
	fsDir := filepath.Join(dir, "assets")
	writeFile(t, filepath.Join(fsDir, "lib.sh"), "#!/bin/bash\necho lib\n")

	entries, err := fswalker.Walk(fsDir)
	require.NoError(t, err)

	script := "#!/bin/bash\neval $(bashfs gen ./assets)\nsource ./assets/lib.sh\n"
	errs, _ := Run(entries, script, dir, fsDir)

	found := false
	for _, e := range errs {
		if contains(e, "bashfs_cat") {
			found = true
		}
	}
	assert.True(t, found, "should suggest bashfs_cat for source into fs dir: %v", errs)
}

func TestRunCatchesBadScriptSyntax(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data.txt"), "just data")

	entries, err := fswalker.Walk(dir)
	require.NoError(t, err)

	script := "#!/bin/bash\nif true\necho broken\n"
	errs, _ := Run(entries, script, dir, dir)
	assert.NotEmpty(t, errs, "should catch main script syntax errors")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
