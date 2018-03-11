package rlimit

import (
	"errors"
	"net/url"
)

// ParseURL parses an URL string.
func ParseURL(s string) (url *url.URL, err error) {
	if s == "" {
		err = errors.New("empty URL")
		return
	}
	url, err = url.Parse(s)
	return
}
