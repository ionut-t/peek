package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/ionut-t/goeditor"
	"github.com/ionut-t/goeditor/core"
)

const messageDuration = 3 * time.Second

// errNoFileName is returned for a bare `:w`: piped input has no file name.
var errNoFileName = errors.New("E32: No file name")

type model struct {
	editor   goeditor.Model
	language string
}

func main() {
	language := flag.String("lang", "", "syntax highlighting language (e.g. yaml, json, go); detected from a redirected file's name, or JSON, if empty")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: some cmd 2>&1 | peek [--lang language]")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Without a pipe or redirect, ReadAll would wait for the keyboard.
	if term.IsTerminal(os.Stdin.Fd()) {
		flag.Usage()
		os.Exit(2)
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	// A command that succeeds quietly (`go build ./... 2>&1 | peek`) gives
	// nothing to read: say so instead of opening an empty editor.
	if isEmpty(data) {
		fmt.Fprintln(os.Stderr, "peek: no input")
		return
	}

	lang := *language
	if lang == "" {
		lang = detectLanguage(filePath(os.Stdin), data)
	}

	// stdin is the pipe; Bubble Tea reads keys from /dev/tty by itself.
	if _, err := tea.NewProgram(newModel(data, lang)).Run(); err != nil {
		log.Fatal(err)
	}
}

// isEmpty reports whether input holds nothing but white space.
func isEmpty(data []byte) bool {
	return len(bytes.TrimSpace(data)) == 0
}

func newModel(data []byte, language string) model {
	editor := goeditor.New(80, 24)
	editor.SetBytes(data)
	editor.Focus()
	editor.SetCursorMode(goeditor.CursorBlink)
	editor.ShowRelativeLineNumbers(true)
	editor.SetMoveAcrossLines(true)

	if language != "" {
		editor.SetLanguage(language, languageTheme(true))
	}

	return model{editor: editor, language: language}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.editor.CursorBlink(), tea.RequestBackgroundColor)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.editor.SetSize(msg.Width, msg.Height)

	case tea.BackgroundColorMsg:
		isDark := msg.IsDark()
		m.editor.WithTheme(goeditor.DefaultTheme(isDark))

		if m.language != "" {
			m.editor.SetLanguage(m.language, languageTheme(isDark))
		}

	case goeditor.SaveMsg:
		cmd := m.save(msg)
		return m, cmd

	case goeditor.ErrorMsg:
		if msg.ID == core.ErrUnsavedChangesId {
			return m, tea.Quit
		}

		return m, m.editor.DispatchError(msg.Error, messageDuration)

	case goeditor.OpenMsg:
		if err := openLink(msg.Target); err != nil {
			return m, m.editor.DispatchError(err, messageDuration)
		}

		return m, nil

	case goeditor.QuitMsg:
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)

	return m, cmd
}

// save handles `:w <file>` and reports the result on the status line.
func (m *model) save(msg goeditor.SaveMsg) tea.Cmd {
	var name string
	if msg.Path != nil {
		name = *msg.Path
	}

	n, err := writeContent(name, msg.Content)
	if err != nil {
		return m.editor.DispatchError(err, messageDuration)
	}

	return m.editor.DispatchMessage(fmt.Sprintf("%q written, %d bytes", name, n), messageDuration)
}

// writeContent writes content to name, expanding a leading "~/", and
// returns the number of bytes written. Like Vim, the last line always
// ends with a newline.
func writeContent(name, content string) (int, error) {
	if name == "" {
		return 0, errNoFileName
	}

	path := name
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return 0, err
		}

		path = filepath.Join(home, path[2:])
	}

	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return 0, err
	}

	return len(content), nil
}

func (m model) View() tea.View {
	view := tea.NewView(m.editor.View())
	view.AltScreen = true

	return view
}

func languageTheme(isDark bool) string {
	if isDark {
		return "catppuccin-mocha"
	}

	return "catppuccin-latte"
}
