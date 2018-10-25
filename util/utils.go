package util

import (
	"fmt"
	"net/http"
	netUrl "net/url"
	"strings"
	"time"
	"unicode"
)

// UseTLSScheme returns url with https scheme if use is true
func UseTLSScheme(url string, use bool) (string, error) {
	if url == "" {
		return "", fmt.Errorf("empty URL")
	}
	if use {
		urlObject, err := netUrl.Parse(url)
		if err != nil {
			return "", err
		}
		urlObject.Scheme = "https"
		return urlObject.String(), nil
	}
	return url, nil
}

// NewHTTPClient creates a new instance of http.Client
func NewHTTPClient(timeout time.Duration, transport http.RoundTripper) *http.Client {
	client := &http.Client{
		Timeout: timeout,
	}

	if transport != nil {
		client.Transport = transport
	}

	// go http client does not copy the headers when it follows the redirect.
	// https://github.com/golang/go/issues/4800
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		for attr, val := range via[0].Header {
			if _, ok := req.Header[attr]; !ok {
				req.Header[attr] = val
			}
		}
		return nil
	}

	return client
}

// IsInList returns true if item is in list
func IsInList(item string, l []string) bool {
	for _, listItem := range l {
		if item == listItem {
			return true
		}
	}
	return false
}

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
