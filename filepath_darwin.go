package main

import (
	"bytes"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// filePath returns the path of the file f reads from, or "" when it isn't a
// regular file on disk (a pipe, a terminal). For `peek < test.json`, stdin
// is test.json itself, so this finds its name.
func filePath(f *os.File) string {
	if !isRegular(f) {
		return ""
	}

	// fcntl(F_GETPATH) writes the path into buf. x/sys has no wrapper for
	// it, so the buffer's address goes through FcntlInt's int argument, as
	// x/sys itself does for FcntlFlock. Pinning moves buf to the heap and
	// keeps it in place until libSystem has written to it.
	buf := make([]byte, unix.PathMax)

	var pinner runtime.Pinner
	pinner.Pin(&buf[0])
	defer pinner.Unpin()

	if _, err := unix.FcntlInt(f.Fd(), unix.F_GETPATH, int(uintptr(unsafe.Pointer(&buf[0])))); err != nil {
		return ""
	}

	if i := bytes.IndexByte(buf, 0); i >= 0 {
		buf = buf[:i]
	}

	return string(buf)
}
