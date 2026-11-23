package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/lexers"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name string
		path string
		data string
		want string
	}{
		{name: "json file", path: "/tmp/test.json", data: `{"a": 1}`, want: "JSON"},
		{name: "yaml file", path: "/tmp/test.yaml", data: "a: 1\n", want: "YAML"},
		{name: "go file", path: "/tmp/main.go", data: "package main\n", want: "Go"},
		{name: "file name wins over content", path: "/tmp/data.yaml", data: `{"a": 1}`, want: "YAML"},
		{name: "unknown extension, json content", path: "/tmp/out.log", data: `{"a": 1}`, want: "json"},
		{name: "unknown extension, plain text", path: "/tmp/out.log", data: "ERROR failed\n", want: ""},
		{name: "pipe, json object", data: "{\n  \"a\": [1, 2]\n}\n", want: "json"},
		{name: "pipe, json array", data: "  [1, 2, 3]\n", want: "json"},
		{name: "pipe, invalid json", data: `{"a": 1`, want: ""},
		{name: "pipe, json scalar is plain", data: "42\n", want: ""},
		{name: "pipe, plain text", data: "ls: /nope: No such file or directory\n", want: ""},
		{name: "pipe, empty", data: "", want: ""},

		// Shebangs.
		{name: "shebang bash", data: "#!/bin/bash\nset -e\n", want: "Bash"},
		{name: "shebang sh", data: "#!/bin/sh\necho hi\n", want: "Bash"},
		{name: "shebang env python3", data: "#!/usr/bin/env python3\nimport sys\n", want: "Python"},
		{name: "shebang versioned python", data: "#!/usr/bin/python3.12\n", want: "Python"},
		{name: "shebang env -S", data: "#!/usr/bin/env -S python3 -u\n", want: "Python"},
		{name: "shebang node", data: "#!/usr/bin/env node\nconsole.log(1)\n", want: "JavaScript"},
		{name: "shebang dash", data: "#!/bin/dash\n", want: "Bash"},
		{name: "shebang ruby", data: "#!/usr/bin/env ruby\n", want: "Ruby"},
		{name: "shebang unknown interpreter", data: "#!/usr/bin/env frobnicate\n", want: ""},
		{name: "bare #!", data: "#!\nx\n", want: ""},
		{name: "file name wins over shebang", path: "/tmp/run.py", data: "#!/bin/bash\n", want: "Python"},

		// Diffs.
		{name: "git diff", data: "diff --git a/x.go b/x.go\nindex 1..2 100644\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-a\n+b\n", want: "diff"},
		{name: "git show", data: "commit 99c1dfe0000\nAuthor: someone\n\n    msg\n\ndiff --git a/x b/x\n", want: "diff"},
		{name: "git log without -p is plain", data: "commit 99c1dfe0000\nAuthor: someone\n\n    msg\n", want: ""},
		{name: "diff -u", data: "--- a.txt\t2026-11-23\n+++ b.txt\t2026-11-23\n@@ -1 +1 @@\n", want: "diff"},
		{name: "--- without +++ is plain", data: "--- FAIL: TestX (0.00s)\n    x_test.go:3: boom\n", want: ""},

		// XML and HTML.
		{name: "xml", data: "<?xml version=\"1.0\"?>\n<a><b/></a>\n", want: "xml"},
		{name: "html doctype", data: "<!doctype html><html lang=en>\n", want: "html"},
		{name: "html upper-case doctype", data: "<!DOCTYPE html>\n<html>\n", want: "html"},
		{name: "html tag", data: "<html>\n<body></body>\n", want: "html"},

		// Markdown.
		{name: "markdown fenced block", data: "Run it:\n\n```sh\ngo test ./...\n```\n", want: "markdown"},
		{name: "markdown three kinds", data: "# Title\n\n- one\n- two\n\nSee [docs](https://example.com).\n", want: "markdown"},
		{name: "markdown heading list code", data: "## Summary\n\n* fixes `:wq`\n", want: "markdown"},
		{name: "one fence line is plain", data: "```\nnot closed\n", want: ""},
		{name: "log with # and * lines is plain", data: "2026-11-23T10:00:00Z INFO starting\n# not a heading, a comment in output\n* item that is a log bullet\nERROR: failed\n", want: ""},
		{name: "git branch is plain", data: "* main\n  feature/x\n  fix/y\n", want: ""},
		{name: "git status -sb is plain", data: "## main...origin/main\n M main.go\n?? new.txt\n", want: ""},
		{name: "shell comments are plain", data: "# comment line\n# another\nKEY=value\n", want: ""},
		{name: "go panic is plain", data: "panic: runtime error: index out of range [3] with length 3\n\ngoroutine 1 [running]:\nmain.main()\n\t/tmp/x.go:8 +0x1d\nexit status 2\n", want: ""},
		{name: "go build errors are plain", data: "./main.go:12:2: undefined: foo\n./main.go:14:5: x declared and not used\n", want: ""},
		{name: "markdown signals past the scan limit are ignored", data: strings.Repeat("plain\n", markdownScanLines) + "```\nx\n```\n", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectLanguage(tt.path, []byte(tt.data))
			if got != tt.want {
				t.Fatalf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}

			// goeditor's highlighter looks the name up with lexers.Get.
			if got != "" && lexers.Get(got) == nil {
				t.Errorf("the highlighter doesn't know %q", got)
			}
		})
	}
}

func TestFilePathRegularFile(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir()) // macOS: /var → /private/var
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	got := filePath(f)
	if got == "" {
		t.Skip("filePath is not supported on this OS")
	}

	if got != path {
		t.Errorf("filePath = %q, want %q", got, path)
	}
}

func TestFilePathNotAFile(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()

	if got := filePath(r); got != "" {
		t.Errorf("pipe: filePath = %q, want empty", got)
	}

	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = null.Close() }()

	if got := filePath(null); got != "" {
		t.Errorf("%s: filePath = %q, want empty", os.DevNull, got)
	}
}
