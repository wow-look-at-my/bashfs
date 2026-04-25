package bashgen

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"bashfs/internal/fswalker"
	"github.com/wow-look-at-my/testify/require"
)

// benchSizes is the matrix of "total bytes worth of fixture across N files"
// each generator benchmark runs against. The smallest size catches per-call
// overhead; the larger sizes show how the encode loop scales.
var benchSizes = []struct {
	label     string
	fileCount int
	fileSize  int
}{
	{"1KiB_x1", 1, 1 << 10},
	{"64KiB_x4", 4, 16 << 10},
	{"1MiB_x16", 16, 64 << 10},
}

// fixtureDir materializes fileCount files of fileSize random bytes each into
// a temp dir and walks it. Uses random bytes so gzip can't trivially compress
// the payload to nothing — the compression cost shows up in timings.
func fixtureDir(b *testing.B, fileCount, fileSize int) []fswalker.FileEntry {
	b.Helper()
	dir := b.TempDir()
	for i := 0; i < fileCount; i++ {
		data := make([]byte, fileSize)
		_, err := rand.Read(data)
		require.NoError(b, err)
		path := filepath.Join(dir, fmt.Sprintf("file%03d.bin", i))
		require.NoError(b, os.WriteFile(path, data, 0644))
	}
	files, err := fswalker.Walk(dir)
	require.NoError(b, err)
	return files
}

func BenchmarkGenerateEmbedded(b *testing.B) {
	for _, s := range benchSizes {
		files := fixtureDir(b, s.fileCount, s.fileSize)
		b.Run(s.label, func(b *testing.B) {
			// b.SetBytes is intentionally omitted: it makes the testing
			// framework inject `MB/s` between `ns/op` and `B/op`, which
			// trips the regex in go-toolchain's bench output parser and
			// zeroes the alloc columns. Throughput is implied by ns/op
			// vs the fixture size.
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := GenerateEmbedded(files)
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkGenerateEmbeddedBase64(b *testing.B) {
	for _, s := range benchSizes {
		files := fixtureDir(b, s.fileCount, s.fileSize)
		b.Run(s.label, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := GenerateEmbeddedBase64(files)
				require.NoError(b, err)
			}
		})
	}
}
