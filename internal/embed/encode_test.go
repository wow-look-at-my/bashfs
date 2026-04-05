package embed

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"
	"github.com/wow-look-at-my/testify/assert"
	"github.com/wow-look-at-my/testify/require"
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
