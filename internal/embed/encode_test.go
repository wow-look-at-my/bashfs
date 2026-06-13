package embed

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEncode(t *testing.T) {
	input := []byte("hello, world!")
	encoded, err := Encode(input)
	require.Nil(t, err)

	require.NotEqual(t, "", encoded)

	// Decode and decompress to verify round-trip
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	require.Nil(t, err)

	r, err := gzip.NewReader(bytes.NewReader(compressed))
	require.Nil(t, err)

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.Nil(t, err)

	assert.True(t, bytes.Equal(buf.Bytes(), input))

}

func TestEncodeEmpty(t *testing.T) {
	encoded, err := Encode([]byte{})
	require.Nil(t, err)

	require.NotEqual(t, "", encoded)

}

func TestEncodeBinary(t *testing.T) {
	input := make([]byte, 256)
	for i := range input {
		input[i] = byte(i)
	}
	encoded, err := Encode(input)
	require.Nil(t, err)

	// Verify round-trip
	compressed, _ := base64.StdEncoding.DecodeString(encoded)
	r, _ := gzip.NewReader(bytes.NewReader(compressed))
	var buf bytes.Buffer
	buf.ReadFrom(r)
	assert.True(t, bytes.Equal(buf.Bytes(), input))
}

func TestChecksum(t *testing.T) {
	data := []byte("hello, world!")
	got := Checksum(data)
	assert.Len(t, got, 64, "SHA-256 hex digest must be 64 characters")

	got2 := Checksum(data)
	assert.Equal(t, got, got2, "same input must produce same checksum")

	got3 := Checksum([]byte("different"))
	assert.NotEqual(t, got, got3, "different input must produce different checksum")
}

func TestChecksumEmpty(t *testing.T) {
	got := Checksum([]byte{})
	assert.Len(t, got, 64)
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", got)
}

func TestCompress(t *testing.T) {
	input := []byte("hello, world!")
	compressed, err := Compress(input)
	require.Nil(t, err)
	require.NotEmpty(t, compressed)

	// Verify round-trip decompression
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	require.Nil(t, err)
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.Nil(t, err)
	assert.Equal(t, input, buf.Bytes())
}
