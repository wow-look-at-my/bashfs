package embed

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
)

// Encode gzip-compresses data and returns the result as a base64 string.
func Encode(data []byte) (string, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", fmt.Errorf("creating gzip writer: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return "", fmt.Errorf("compressing data: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("finalizing compression: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
