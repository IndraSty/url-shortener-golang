package base62_test

import (
	"testing"

	"github.com/IndraSty/url-shortener-golang/pkg/base62"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecode(t *testing.T) {
	cases := []struct {
		id   int64
		slug string
	}{
		{1, "1"},
		{9, "9"},
		{10, "a"},
		{61, "Z"},
		{62, "10"},
		{100000, "q0U"},
		{9999999, "FXqX"},
		{1<<32 - 1, "4gfFC3"},
	}

	for _, tc := range cases {
		t.Run(tc.slug, func(t *testing.T) {
			// Encode
			got := base62.Encode(tc.id)
			assert.Equal(t, tc.slug, got)

			// Decode back
			decoded, err := base62.Decode(got)
			require.NoError(t, err)
			assert.Equal(t, tc.id, decoded)
		})
	}
}

func TestDecodeInvalidCharacter(t *testing.T) {
	_, err := base62.Decode("abc!def")
	assert.Error(t, err)
}

func TestDecodeEmpty(t *testing.T) {
	_, err := base62.Decode("")
	assert.Error(t, err)
}

func TestIsValid(t *testing.T) {
	assert.True(t, base62.IsValid("abc123"))
	assert.True(t, base62.IsValid("ABC"))
	assert.True(t, base62.IsValid("0"))
	assert.False(t, base62.IsValid(""))
	assert.False(t, base62.IsValid("abc!"))
	assert.False(t, base62.IsValid("hello world"))
	assert.False(t, base62.IsValid("abc-def")) // hyphen not in base62
}

func BenchmarkEncode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		base62.Encode(9999999)
	}
}

func BenchmarkDecode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		base62.Decode("FXqX") //nolint:errcheck
	}
}
