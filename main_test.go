package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ionut-t/goeditor"
	"github.com/ionut-t/goeditor/core"
)

func TestWriteContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "adds a final newline", content: "a\nb", want: "a\nb\n"},
		{name: "keeps an existing final newline", content: "a\nb\n", want: "a\nb\n"},
		{name: "empty buffer stays empty", content: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out.txt")

			n, err := writeContent(path, tt.content)
			if err != nil {
				t.Fatalf("writeContent: %v", err)
			}

			if got := readFile(t, path); got != tt.want {
				t.Errorf("file = %q, want %q", got, tt.want)
			}

			if n != len(tt.want) {
				t.Errorf("n = %d, want %d", n, len(tt.want))
			}
		})
	}
}

func TestWriteContentNoFileName(t *testing.T) {
	if _, err := writeContent("", "x"); !errors.Is(err, errNoFileName) {
		t.Fatalf("err = %v, want %v", err, errNoFileName)
	}
}

func TestWriteContentExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := writeContent("~/out.txt", "x"); err != nil {
		t.Fatalf("writeContent: %v", err)
	}

	if got := readFile(t, filepath.Join(home, "out.txt")); got != "x\n" {
		t.Errorf("file = %q, want %q", got, "x\n")
	}
}

func TestWriteContentMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "out.txt")

	if _, err := writeContent(path, "x"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want %v", err, os.ErrNotExist)
	}
}

func TestNewModelLoadsInput(t *testing.T) {
	m := newModel([]byte("ls: /nope: No such file or directory\n"), "")

	if got := m.editor.GetCurrentContent(); !strings.Contains(got, "ls: /nope") {
		t.Errorf("content = %q, want the piped input", got)
	}

	if !strings.Contains(screen(t, m), "ls: /nope") {
		t.Error("piped input is not on screen")
	}
}

func TestSaveWithoutFileName(t *testing.T) {
	m := update(t, newModel([]byte("x"), ""), goeditor.SaveMsg{Content: "x"})

	if got := screen(t, m); !strings.Contains(got, "E32: No file name") {
		t.Errorf("screen does not show E32:\n%s", got)
	}
}

func TestSaveWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	m := update(t, newModel([]byte("x"), ""), goeditor.SaveMsg{Path: &path, Content: "x"})

	if got := readFile(t, path); got != "x\n" {
		t.Errorf("file = %q, want %q", got, "x\n")
	}

	if got := screen(t, m); !strings.Contains(got, "written, 2 bytes") {
		t.Errorf("screen does not show the write message:\n%s", got)
	}
}

func TestEditorErrorIsShown(t *testing.T) {
	m := update(t, newModel([]byte("x"), ""), goeditor.ErrorMsg{Error: errors.New("E37: No write since last change")})

	if got := screen(t, m); !strings.Contains(got, "E37: No write since last change") {
		t.Errorf("screen does not show the error:\n%s", got)
	}
}

func TestQuit(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.Msg
	}{
		{name: "editor quit (:q)", msg: goeditor.QuitMsg{}},
		{name: "ctrl+c", msg: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
		{name: ":q with unsaved changes", msg: goeditor.ErrorMsg{ID: core.ErrUnsavedChangesId, Error: core.ErrUnsavedChanges}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cmd := newModel(nil, "").Update(tt.msg)
			if cmd == nil {
				t.Fatal("no command returned")
			}

			if _, ok := run(cmd).(tea.QuitMsg); !ok {
				t.Errorf("command does not quit")
			}
		})
	}
}

func TestCtrlLRepaints(t *testing.T) {
	_, cmd := newModel([]byte("x"), "").Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("no command returned")
	}

	if run(cmd) != tea.ClearScreen() {
		t.Error("ctrl+l does not repaint the screen")
	}
}

func TestLanguageTheme(t *testing.T) {
	if got := languageTheme(true); got != "catppuccin-mocha" {
		t.Errorf("dark theme = %q", got)
	}

	if got := languageTheme(false); got != "catppuccin-latte" {
		t.Errorf("light theme = %q", got)
	}
}

// run calls cmd and returns its message, or nil if it doesn't return within
// a second (commands such as the cursor blink wait on a timer).
func run(cmd tea.Cmd) tea.Msg {
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		return msg
	case <-time.After(time.Second):
		return nil
	}
}

// update sizes the editor, then sends msg.
func update(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	next, _ = next.Update(msg)

	return next.(model)
}

// screen renders m with an 80x10 size and strips ANSI styling.
func screen(t *testing.T, m model) string {
	t.Helper()

	m.editor.SetSize(80, 10)

	return ansi.Strip(m.View().Content)
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}

func TestIsEmpty(t *testing.T) {
	for _, in := range []string{"", "\n", "  \n\t\n"} {
		if !isEmpty([]byte(in)) {
			t.Errorf("isEmpty(%q) = false, want true", in)
		}
	}

	for _, in := range []string{"x", "\n./main.go:1:1: error\n"} {
		if isEmpty([]byte(in)) {
			t.Errorf("isEmpty(%q) = true, want false", in)
		}
	}
}
