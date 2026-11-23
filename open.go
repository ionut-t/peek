package main

import (
	"errors"
	"net/url"
	"strings"

	"github.com/ionut-t/goeditor"
)

// errNotWebLink is shown when gx is on anything but an http(s) link.
var errNotWebLink = errors.New("gx: peek opens only http and https links")

// openWithSystem is goeditor.OpenWithSystem, swapped out in tests.
var openWithSystem = goeditor.OpenWithSystem

// openLink handles gx. peek shows other programs' output, so the target is
// untrusted text: the system opener would also launch a local app or
// script (`open evil.command`) or a custom URL scheme. Only web links with
// a host are opened.
func openLink(target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return errNotWebLink
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return openWithSystem(target)
	default:
		return errNotWebLink
	}
}
