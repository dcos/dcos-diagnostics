package util

import (
	"strings"
	"unicode"
)

// SanitizeString will remove the first occurrence of a slash in a string and
// replaces all special characters with underscores.
func SanitizeString(s string) string {
	trimmedLeftSlash := strings.TrimLeft(s, "/")

	// replace all special characters with underscores
	return strings.Map(func(r rune) rune {
		switch {
		case r == '_' || r == '-':
			return r
		case !unicode.IsDigit(r) && !unicode.IsLetter(r):
			return '_'
		}
		return r
	}, trimmedLeftSlash)
}
