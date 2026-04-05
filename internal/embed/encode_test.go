package embed

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"
)

func TestEncode(t *testing.T) {
	input := []byte("hello, world!")
	encoded, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}

	if encoded == "" {
		t.Fatal("Encode() returned empty string")
	}

	// Decode and decompress to verify round-trip
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode error: %v", err)
	}

	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip reader error: %v", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("gzip decompress error: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), input) {
		t.Errorf("round-trip mismatch: got %q, want %q", buf.String(), string(input))
	}
}

func TestEncodeEmpty(t *testing.T) {
	encoded, err := Encode([]byte{})
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	if encoded == "" {
		t.Fatal("Encode() returned empty string for empty input")
	}
}

func TestEncodeBinary(t *testing.T) {
	input := make([]byte, 256)
	for i := range input {
		input[i] = byte(i)
	}
	encoded, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}

	// Verify round-trip
	compressed, _ := base64.StdEncoding.DecodeString(encoded)
	r, _ := gzip.NewReader(bytes.NewReader(compressed))
	var buf bytes.Buffer
	buf.ReadFrom(r)
	if !bytes.Equal(buf.Bytes(), input) {
		t.Error("binary round-trip mismatch")
	}
}
