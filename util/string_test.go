package util

import "testing"

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
		if got != item.expected {
			t.Fatalf("input %s expected %s. Got %s", item.input, item.expected, got)
		}
	}
}
