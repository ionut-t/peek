//go:build !darwin && !linux

package main

import "os"

// filePath can't find the file behind a descriptor on this OS, so the
// language comes from --lang or the JSON check only.
func filePath(*os.File) string { return "" }
