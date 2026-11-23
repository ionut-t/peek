package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/ionut-t/goeditor"
)

// stubOpener records what would be opened instead of opening it.
func stubOpener(t *testing.T) *[]string {
	t.Helper()

	var opened []string
	orig := openWithSystem
	openWithSystem = func(target string) error {
		opened = append(opened, target)
		return nil
	}
	t.Cleanup(func() { openWithSystem = orig })

	return &opened
}

func TestOpenLink(t *testing.T) {
	tests := []struct {
		target string
		open   bool
	}{
		{target: "https://github.com/ionut-t/peek", open: true},
		{target: "http://localhost:8080/health", open: true},
		{target: "HTTPS://EXAMPLE.COM", open: true},

		{target: "https://", open: false},
		{target: "example.com", open: false},
		{target: "file:///etc/passwd", open: false},
		{target: "/Applications/Calculator.app", open: false},
		{target: "./evil.command", open: false},
		{target: "~/script.sh", open: false},
		{target: "javascript:alert(1)", open: false},
		{target: "x-apple-systempreferences:com.apple.preference.security", open: false},
		{target: "vscode://file/etc/passwd", open: false},
		{target: "ssh://host", open: false},
		{target: "", open: false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			opened := stubOpener(t)

			err := openLink(tt.target)

			if tt.open {
				if err != nil || len(*opened) != 1 || (*opened)[0] != tt.target {
					t.Fatalf("err = %v, opened = %q; want %q opened", err, *opened, tt.target)
				}
				return
			}

			if !errors.Is(err, errNotWebLink) || len(*opened) != 0 {
				t.Fatalf("err = %v, opened = %q; want errNotWebLink and nothing opened", err, *opened)
			}
		})
	}
}

func TestOpenMsg(t *testing.T) {
	opened := stubOpener(t)

	m := update(t, newModel([]byte("x"), ""), goeditor.OpenMsg{Target: "https://example.com"})
	if len(*opened) != 1 {
		t.Fatalf("opened = %q, want the link opened", *opened)
	}

	m = update(t, m, goeditor.OpenMsg{Target: "./evil.command"})
	if len(*opened) != 1 {
		t.Fatalf("opened = %q, want the script not opened", *opened)
	}

	if got := screen(t, m); !strings.Contains(got, errNotWebLink.Error()) {
		t.Errorf("screen does not show the refusal:\n%s", got)
	}
}
