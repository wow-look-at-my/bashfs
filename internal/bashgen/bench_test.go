package bashgen

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"bashfs/internal/fswalker"
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
		if _, err := rand.Read(data); err != nil {
			b.Fatalf("rand: %v", err)
		}
		path := filepath.Join(dir, fmt.Sprintf("file%03d.bin", i))
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatalf("write: %v", err)
		}
	}
	files, err := fswalker.Walk(dir)
	if err != nil {
		b.Fatalf("walk: %v", err)
	}
	return files
}

func BenchmarkGenerateEmbedded(b *testing.B) {
	for _, s := range benchSizes {
		files := fixtureDir(b, s.fileCount, s.fileSize)
		totalBytes := int64(s.fileCount * s.fileSize)
		b.Run(s.label, func(b *testing.B) {
			b.SetBytes(totalBytes)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := GenerateEmbedded(files); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkGenerateEmbeddedBase64(b *testing.B) {
	for _, s := range benchSizes {
		files := fixtureDir(b, s.fileCount, s.fileSize)
		totalBytes := int64(s.fileCount * s.fileSize)
		b.Run(s.label, func(b *testing.B) {
			b.SetBytes(totalBytes)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := GenerateEmbeddedBase64(files); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
