package util

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSanitizeString(t *testing.T) {
	for _, item := range []struct {
		input, expected string
	}{
		{
			input:    "hello(1)world",
			expected: "hello_1_world",
		},
		{
			input:    "hello world",
			expected: "hello_world",
		},
		{
			input:    "/Hello_World",
			expected: "Hello_World",
		},
		{
			input:    "Hello-World",
			expected: "Hello-World",
		},
	} {
		got := SanitizeString(item.input)
		assert.Equal(t, item.expected, got)
	}
}

func TestIsInList(t *testing.T) {
	assert.False(t, IsInList("x", nil))
	assert.True(t, IsInList("x", []string{"x"}))
	assert.True(t, IsInList("x", []string{"a", "b", "x"}))
	assert.False(t, IsInList("x", []string{"a", "b", "c"}))
}

func TestUseTLSScheme(t *testing.T) {
	url, err := UseTLSScheme("", false)
	assert.NoError(t, err)
	assert.Equal(t, "", url)

	url, err = UseTLSScheme("", true)
	assert.NoError(t, err)
	assert.Equal(t, "https:", url)

	url, err = UseTLSScheme("/http://", false)
	assert.NoError(t, err)
	assert.Equal(t, "/http://", url)

	url, err = UseTLSScheme("/http://", true)
	assert.NoError(t, err)
	assert.Equal(t, "https:///http://", url)

	url, err = UseTLSScheme("http://google.com", true)
	assert.NoError(t, err)
	assert.Equal(t, "https://google.com", url)

	url, err = UseTLSScheme("ftp://google.com", true)
	assert.NoError(t, err)
	assert.Equal(t, "https://google.com", url)
}