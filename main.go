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
	isDark   bool
	// detectPending is set while the language is still to be detected from
	// the first input that arrives.
	detectPending bool
	// noInput is set when the input ended without any text.
	noInput bool
	// followRow is the row the cursor was following from when input arrived
	// during a part typed command, or -1. See followEnd.
	followRow int
	// readErr is an error that cut the input short before peek opened.
	readErr error
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

	// A read error after some output still opens what arrived, with the
	// error on the status line.
	data, rest, readErr := readInput(os.Stdin)
	if readErr != nil && isEmpty(data) {
		log.Fatal(readErr)
	}

	if rest != nil {
		follow(data, rest, *language)
		return
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
	m := newModel(data, lang)
	m.readErr = readErr

	if _, err := tea.NewProgram(m).Run(); err != nil {
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

	return model{editor: editor, language: language, isDark: true, followRow: -1}
}

// readInput reads f whole when it is a file or ends within startWait.
// Otherwise it returns what arrived so far and rest, which delivers the
// remainder; rest is nil when the input was read whole.
func readInput(f *os.File) (data []byte, rest <-chan chunk, err error) {
	if isRegular(f) {
		data, err = io.ReadAll(f)
		return data, nil, err
	}

	ch := readChunks(f)

	data, ended, err := collect(ch, startWait)
	if ended {
		return data, nil, err
	}

	return data, ch, nil
}

// follow opens peek on input that is still arriving, appends the rest as it
// comes, and follows the end like `less +F`.
func follow(data []byte, rest <-chan chunk, language string) {
	m := newModel(nil, language)
	m.detectPending = language == ""
	m.editor.SetPlaceholder("waiting for input…")
	m.appendInput(data)

	p := tea.NewProgram(m)
	go forward(rest, p.Send, batchInterval)

	final, err := p.Run()
	if err != nil {
		log.Fatal(err)
	}

	if fm, ok := final.(model); ok && fm.noInput {
		fmt.Fprintln(os.Stderr, "peek: no input")
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.editor.CursorBlink(), tea.RequestBackgroundColor, m.reportReadErr())
}

// reportReadErr reports an error that cut input short before peek opened,
// as endInput reports one that ends a stream, or returns nil.
func (m model) reportReadErr() tea.Cmd {
	err := m.readErr
	if err == nil {
		return nil
	}

	return func() tea.Msg { return eofMsg{err: err} }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.editor.SetSize(msg.Width, msg.Height)

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.editor.WithTheme(goeditor.DefaultTheme(m.isDark))

		if m.language != "" {
			m.editor.SetLanguage(m.language, languageTheme(m.isDark))
		}

	case chunkMsg:
		m.appendInput(msg)
		return m, nil

	case eofMsg:
		return m, m.endInput(msg.err)

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
	m.catchUp()

	return m, cmd
}

// appendInput adds input that arrived while peek is open. If the cursor was
// on the last line in normal mode, it moves to the new last line, so the end
// stays in view; once the user moves up, it stays put until `G`.
func (m *model) appendInput(data []byte) {
	if len(data) == 0 {
		return
	}

	m.followEnd(func() { m.editor.AppendBytes(data) })

	if m.detectPending && !isEmpty(data) {
		m.detectPending = false

		if lang := detectLanguage("", data); lang != "" {
			m.language = lang
			m.editor.SetLanguage(lang, languageTheme(m.isDark))
		}
	}
}

// endInput adds a final line without a newline and reports the end of the
// input, or quits when it held no text.
func (m *model) endInput(err error) tea.Cmd {
	m.followEnd(m.editor.FlushAppend)

	if err != nil {
		return m.editor.DispatchError(fmt.Errorf("peek: reading input: %w", err), messageDuration)
	}

	if isEmpty([]byte(m.editor.GetCurrentContent())) {
		m.noInput = true
		return tea.Quit
	}

	lines := m.editor.GetEditor().GetBuffer().LineCount()

	return m.editor.DispatchMessage(fmt.Sprintf("input ended, %d lines", lines), messageDuration)
}

// followEnd runs add and, if the cursor was on the last line in normal mode,
// moves it to the start of the new last line. Not to its end: in normal mode
// that is one past the last character.
//
// While a command is part way typed (`d` of `dd`, a count, `g`), the cursor
// isn't moved: the rest of the command would act on the new last line. The
// row is kept instead, and catchUp follows once the command is done.
func (m *model) followEnd(add func()) {
	buf := m.editor.GetEditor().GetBuffer()
	row := buf.GetCursor().Position.Row
	following := m.editor.IsNormalMode() && row == buf.LineCount()-1

	add()

	switch {
	case !following:
	case m.editor.CommandPending():
		m.followRow = row
	default:
		m.toEnd()
	}
}

// catchUp follows input that arrived during a part typed command, once the
// command is done, if it left the cursor on the row it was following from
// (`yy`, `dd`, `d<Esc>`). A command that moved it (`3k`, `gg`) stops
// following, as moving up always does.
func (m *model) catchUp() {
	if m.followRow < 0 || m.editor.CommandPending() {
		return
	}

	if m.editor.IsNormalMode() && m.editor.GetCursorPosition().Row == m.followRow {
		m.toEnd()
	}

	m.followRow = -1
}

// toEnd moves the cursor to the start of the last line.
func (m *model) toEnd() {
	last := m.editor.GetEditor().GetBuffer().LineCount() - 1
	_ = m.editor.SetCursorPosition(last, 0) // fails only on an empty buffer
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
