package main

import (
	"fmt"
	"os"
)

// filePath returns the path of the file f reads from, or "" when it isn't a
// regular file on disk (a pipe, a terminal). For `peek < test.json`, stdin
// is test.json itself, so this finds its name.
func filePath(f *os.File) string {
	if !isRegular(f) {
		return ""
	}

	path, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", f.Fd()))
	if err != nil {
		return ""
	}

	return path
}
