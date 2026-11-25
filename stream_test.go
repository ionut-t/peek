package main

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestCollectEndedInput(t *testing.T) {
	data, ended, err := collect(readChunks(strings.NewReader("a\nb\n")), time.Second)
	if err != nil || !ended || string(data) != "a\nb\n" {
		t.Fatalf("collect = %q, %v, %v; want all input, ended", data, ended, err)
	}
}

func TestCollectOpenInput(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()

	ch := readChunks(r)

	go func() { _, _ = w.Write([]byte("first\n")) }()

	data, ended, err := collect(ch, 100*time.Millisecond)
	if err != nil || ended || string(data) != "first\n" {
		t.Fatalf("collect = %q, %v, %v; want what arrived, not ended", data, ended, err)
	}
}

func TestCollectReadError(t *testing.T) {
	boom := errors.New("boom")

	_, ended, err := collect(readChunks(io.MultiReader(strings.NewReader("x"), errReader{boom})), time.Second)
	if !ended || !errors.Is(err, boom) {
		t.Fatalf("collect = %v, %v; want ended with %v", ended, err, boom)
	}
}

func TestForwardBatchesThenEnds(t *testing.T) {
	ch := make(chan chunk, 3)
	ch <- chunk{data: []byte("a\n")}
	ch <- chunk{data: []byte("b\n")}
	close(ch)

	var msgs []tea.Msg

	forward(ch, func(msg tea.Msg) { msgs = append(msgs, msg) }, time.Hour)

	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want a chunk and an EOF: %v", len(msgs), msgs)
	}

	if got, ok := msgs[0].(chunkMsg); !ok || string(got) != "a\nb\n" {
		t.Errorf("first message = %#v, want chunkMsg(\"a\\nb\\n\")", msgs[0])
	}

	if got, ok := msgs[1].(eofMsg); !ok || got.err != nil {
		t.Errorf("second message = %#v, want eofMsg{}", msgs[1])
	}
}

func TestFollowWhileOnLastLine(t *testing.T) {
	m := streamModel(t, "1\n2\n3\n")

	m = update(t, m, chunkMsg("4\n5\n"))

	if row := m.editor.GetCursorPosition().Row; row != 4 {
		t.Errorf("cursor row = %d, want 4 (the new last line)", row)
	}

	if !strings.Contains(screen(t, m), "5") {
		t.Error("arrived line is not on screen")
	}
}

func TestNoFollowAfterMovingUp(t *testing.T) {
	m := streamModel(t, "1\n2\n3\n")
	m = update(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})

	m = update(t, m, chunkMsg("4\n"))

	if row := m.editor.GetCursorPosition().Row; row != 1 {
		t.Errorf("cursor row = %d, want 1 (left where the user put it)", row)
	}

	// G goes back to the end and following resumes.
	m = update(t, m, tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = update(t, m, chunkMsg("5\n"))

	if row := m.editor.GetCursorPosition().Row; row != 4 {
		t.Errorf("after G, cursor row = %d, want 4", row)
	}
}

func TestNoFollowInVisualMode(t *testing.T) {
	m := streamModel(t, "1\n2\n")
	m = update(t, m, tea.KeyPressMsg{Code: 'V', Text: "V"})

	m = update(t, m, chunkMsg("3\n"))

	if row := m.editor.GetCursorPosition().Row; row != 1 {
		t.Errorf("cursor row = %d, want 1 (a selection never moves)", row)
	}
}

func TestPartialLineWaitsForEOF(t *testing.T) {
	m := streamModel(t, "1\n")
	m = update(t, m, chunkMsg("par"))
	m = update(t, m, chunkMsg("tial"))

	if got := m.editor.GetCurrentContent(); got != "1" {
		t.Fatalf("content = %q, want the partial line held back", got)
	}

	m = update(t, m, eofMsg{})

	if got := m.editor.GetCurrentContent(); got != "1\npartial" {
		t.Errorf("content = %q, want the partial line added at EOF", got)
	}

	if row := m.editor.GetCursorPosition().Row; row != 1 {
		t.Errorf("cursor row = %d, want 1 (followed to the flushed line)", row)
	}

	if !strings.Contains(screen(t, m), "input ended, 2 lines") {
		t.Errorf("screen does not show the end of input:\n%s", screen(t, m))
	}
}

func TestArrivedTextIsNotAnEdit(t *testing.T) {
	m := streamModel(t, "1\n")
	m = update(t, m, chunkMsg("2\n"))

	if m.editor.GetEditor().GetBuffer().IsModified() {
		t.Error("arrived text marks the buffer modified")
	}
}

func TestNoInputAfterWaiting(t *testing.T) {
	m := streamModel(t, "")
	m = update(t, m, chunkMsg("  \n"))

	next, cmd := m.Update(eofMsg{})
	if cmd == nil {
		t.Fatal("no command returned")
	}

	if _, ok := run(cmd).(tea.QuitMsg); !ok {
		t.Error("blank input does not quit")
	}

	if !next.(model).noInput {
		t.Error("noInput not set")
	}
}

func TestLanguageDetectedFromFirstChunk(t *testing.T) {
	m := streamModel(t, "")
	m = update(t, m, chunkMsg("diff --git a/x b/x\n--- a/x\n+++ b/x\n"))

	if m.language != "diff" {
		t.Errorf("language = %q, want diff", m.language)
	}
}

// streamModel returns a model following input that began with data, as
// follow sets it up.
func streamModel(t *testing.T, data string) model {
	t.Helper()

	m := newModel(nil, "")
	m.detectPending = true
	m.appendInput([]byte(data))

	return m
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// TestNoFollowDuringPartTypedCommand checks that `d`, input, `d` deletes the
// line the user was on, not the line that arrived, and following goes on.
func TestNoFollowDuringPartTypedCommand(t *testing.T) {
	m := streamModel(t, "1\n2\n3\n")

	m = update(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = update(t, m, chunkMsg("4\n5\n"))

	if row := m.editor.GetCursorPosition().Row; row != 2 {
		t.Fatalf("cursor row = %d, want 2 (not moved during `d`)", row)
	}

	m = update(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})

	if got := m.editor.GetCurrentContent(); got != "1\n2\n4\n5" {
		t.Errorf("content = %q, want line 3 deleted", got)
	}

	if row := m.editor.GetCursorPosition().Row; row != 3 {
		t.Errorf("cursor row = %d, want 3 (caught up with the end)", row)
	}
}

func TestCountThenMoveDuringInputStopsFollowing(t *testing.T) {
	m := streamModel(t, "1\n2\n3\n4\n")

	m = update(t, m, tea.KeyPressMsg{Code: '2', Text: "2"})
	m = update(t, m, chunkMsg("5\n"))
	m = update(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})

	if row := m.editor.GetCursorPosition().Row; row != 1 {
		t.Errorf("cursor row = %d, want 1 (2k from the old last line)", row)
	}
}

func TestReadErrorShownWithInput(t *testing.T) {
	m := newModel([]byte("partial output\n"), "")
	m.readErr = errors.New("broken pipe")

	m = update(t, m, run(m.reportReadErr()))

	got := screen(t, m)
	if !strings.Contains(got, "partial output") || !strings.Contains(got, "peek: reading input: broken pipe") {
		t.Errorf("screen does not show the input and the error:\n%s", got)
	}
}
