package main

import (
	"bytes"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	// startWait is how long peek waits for the input to end before it opens
	// and follows the rest. Output that ends sooner (most commands) opens
	// as it always did: whole, with its language detected from all of it.
	startWait = 200 * time.Millisecond

	// batchInterval bounds redraws while following a busy stream: each
	// append re-lays the whole buffer, so chunks are sent at most this often.
	batchInterval = 50 * time.Millisecond

	readSize = 32 * 1024
)

// chunkMsg carries input that arrived after peek opened.
type chunkMsg []byte

// eofMsg says the input ended. err is a read error, nil at a normal EOF.
type eofMsg struct{ err error }

// chunk is one read from the input. A chunk with err set is the last one
// before the channel closes.
type chunk struct {
	data []byte
	err  error
}

// readChunks reads r in a goroutine. The channel closes when r ends; a read
// error other than io.EOF is sent first.
func readChunks(r io.Reader) <-chan chunk {
	ch := make(chan chunk, 16)

	go func() {
		defer close(ch)

		buf := make([]byte, readSize)

		for {
			n, err := r.Read(buf)
			if n > 0 {
				// A copy of what was read, not the whole buffer: a slow
				// stream reads a line at a time.
				ch <- chunk{data: bytes.Clone(buf[:n])}
			}

			if err == io.EOF {
				return
			}

			if err != nil {
				ch <- chunk{err: err}
				return
			}
		}
	}()

	return ch
}

// collect gathers input from ch until it ends or wait passes. ended reports
// whether the input ended in time; err is its read error, if any.
func collect(ch <-chan chunk, wait time.Duration) (data []byte, ended bool, err error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()

	for {
		select {
		case c, ok := <-ch:
			if !ok {
				return data, true, err
			}

			if c.err != nil {
				err = c.err
			}

			data = append(data, c.data...)

		case <-timer.C:
			return data, false, nil
		}
	}
}

// forward sends what arrives on ch to the program as chunkMsgs, at most one
// every interval, then an eofMsg when the input ends.
func forward(ch <-chan chunk, send func(tea.Msg), interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var (
		pending []byte
		readErr error
	)

	for {
		select {
		case c, ok := <-ch:
			if !ok {
				if len(pending) > 0 {
					send(chunkMsg(pending))
				}

				send(eofMsg{err: readErr})

				return
			}

			if c.err != nil {
				readErr = c.err
			}

			pending = append(pending, c.data...)

		case <-ticker.C:
			if len(pending) > 0 {
				send(chunkMsg(pending))
				pending = nil
			}
		}
	}
}
