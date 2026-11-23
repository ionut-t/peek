package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
)

// detectLanguage picks a highlighting language when --lang isn't given, in
// order of how sure each answer is:
//
//  1. path is the file behind stdin for `peek < file` (empty for a pipe):
//     its name decides, as an editor would.
//  2. A marker on the first line: a shebang (`#!/usr/bin/env python3`), a
//     diff (`git diff`, `git show`, `diff -u`), or an XML or HTML start.
//  3. A valid JSON object or array (`curl … | peek`).
//  4. Markdown, by the strict rule in isMarkdown.
//
// Anything else is plain text. Chroma's own content analyser isn't used: on
// real output it took `git log`, `ps aux` and diffs for GDScript.
func detectLanguage(path string, data []byte) string {
	if path != "" {
		if lexer := lexers.Match(path); lexer != nil {
			return lexer.Config().Name
		}
	}

	if lang := firstLineLanguage(data); lang != "" {
		return lang
	}

	if isJSON(data) {
		return "json"
	}

	if isMarkdown(data) {
		return "markdown"
	}

	return ""
}

// isRegular reports whether f is a regular file, not a pipe or a device.
func isRegular(f *os.File) bool {
	info, err := f.Stat()

	return err == nil && info.Mode().IsRegular()
}

// firstLineLanguage recognises input by its first line (two for `diff -u`).
func firstLineLanguage(data []byte) string {
	first, rest, _ := bytes.Cut(data, []byte("\n"))
	line := string(bytes.TrimRight(first, "\r"))
	lower := strings.ToLower(line)

	switch {
	case strings.HasPrefix(line, "#!"):
		return shebangLanguage(line)
	case isDiff(line, rest):
		return "diff"
	case strings.HasPrefix(line, "<?xml"):
		return "xml"
	case strings.HasPrefix(lower, "<!doctype html"), strings.HasPrefix(lower, "<html"):
		return "html"
	}

	return ""
}

// interpreterAliases covers interpreters chroma has no lexer name for.
var interpreterAliases = map[string]string{
	"node":   "javascript",
	"nodejs": "javascript",
	"deno":   "javascript",
	"bun":    "javascript",
	"dash":   "bash",
}

// shebangLanguage maps `#!/bin/bash` or `#!/usr/bin/env -S python3 -u` to a
// language, or "" for an interpreter chroma doesn't know.
func shebangLanguage(line string) string {
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return ""
	}

	interp := filepath.Base(fields[0])
	if interp == "env" {
		fields = fields[1:]
		for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
			fields = fields[1:] // env -S, env -i
		}
		if len(fields) == 0 {
			return ""
		}
		interp = fields[0]
	}

	if alias, ok := interpreterAliases[interp]; ok {
		interp = alias
	}

	// python3.12 → python3 → python
	for _, name := range []string{interp, strings.TrimRight(interp, "0123456789."), strings.TrimRight(interp, "0123456789.-")} {
		if lexer := lexers.Get(name); lexer != nil {
			return lexer.Config().Name
		}
	}

	return ""
}

// isDiff recognises `git diff` (`diff --git`), `git show` and `git log -p`
// (`commit <hash>` followed by a diff), and `diff -u` (`--- ` then `+++ `).
func isDiff(first string, rest []byte) bool {
	switch {
	case strings.HasPrefix(first, "diff --git "):
		return true
	case strings.HasPrefix(first, "commit ") && bytes.Contains(rest, []byte("\ndiff --git ")):
		return true
	case strings.HasPrefix(first, "--- "):
		second, _, _ := bytes.Cut(rest, []byte("\n"))
		return bytes.HasPrefix(second, []byte("+++ "))
	}

	return false
}

// isJSON reports whether data is a JSON object or array. Bare scalars such
// as `true` or `42` are valid JSON too, but more likely plain output.
func isJSON(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return false
	}

	return json.Valid(trimmed)
}

// markdownScanLines bounds the markdown check on huge input: the signals
// show up early in a real document.
const markdownScanLines = 1000

var (
	mdHeading = regexp.MustCompile(`^#{1,6} \S`)
	mdList    = regexp.MustCompile(`^\s*([-*+]|\d+\.) \S`)
	mdLink    = regexp.MustCompile(`\[[^\]]+\]\([^)\s]+\)`)
	mdInline  = regexp.MustCompile("\\*\\*[^*]+\\*\\*|`[^`]+`")
)

// isMarkdown is a strict guess: a fenced code block (two ``` lines), or at
// least three different kinds of markdown: headings, list items, links, and
// **bold** or `code`. Command output often has one or two of these by
// chance (`# comments`, `* main` from git branch, `- ` bullets), rarely
// three.
func isMarkdown(data []byte) bool {
	var fences int
	var heading, list, link, inline bool

	n := 0
	for line := range bytes.Lines(data) {
		if n++; n > markdownScanLines {
			break
		}

		l := string(bytes.TrimRight(line, "\r\n"))
		if strings.HasPrefix(strings.TrimLeft(l, " "), "```") {
			if fences++; fences >= 2 {
				return true
			}
		}

		heading = heading || mdHeading.MatchString(l)
		list = list || mdList.MatchString(l)
		link = link || mdLink.MatchString(l)
		inline = inline || mdInline.MatchString(l)
	}

	kinds := 0
	for _, found := range []bool{heading, list, link, inline} {
		if found {
			kinds++
		}
	}

	return kinds >= 3
}
