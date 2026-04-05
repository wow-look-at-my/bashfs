package embed

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
)

// Compress gzip-compresses data and returns the raw compressed bytes.
func Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("creating gzip writer: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("compressing data: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("finalizing compression: %w", err)
	}
	return buf.Bytes(), nil
}

// Encode gzip-compresses data and returns the result as a base64 string.
func Encode(data []byte) (string, error) {
	compressed, err := Compress(data)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(compressed), nil
}
