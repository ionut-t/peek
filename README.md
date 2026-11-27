# peek

A lightweight TUI for reading command output with vim keybindings.

Built with [goeditor](https://github.com/ionut-t/goeditor).

## Install

```sh
go install github.com/ionut-t/peek@latest
```

## Usage

Pipe any command into peek:

```sh
go build ./... 2>&1 | peek          # build errors (go build writes them to stderr)
some cmd |& peek                    # zsh/bash shorthand for 2>&1 |
git diff | peek                     # highlighted as a diff
curl -s https://api.example.com/items | peek   # JSON is detected
kubectl get pod web -o yaml | peek --lang yaml
peek < config.yaml                  # a file works too
```

A plain `|` only carries stdout, so add `2>&1` (or use `|&`) to see errors.

| Flag            | Description                                   |
| --------------- | --------------------------------------------- |
| `--lang <name>` | syntax highlighting (`yaml`, `json`, `go`, …) |

Without `--lang`, peek picks the language itself, from the surest sign down:

1. the file name, when stdin is a file (`peek < config.yaml`), on macOS and
   Linux;
2. the first line: a shebang (`#!/usr/bin/env python3`), a diff (`git diff`,
   `git show`, `git log -p`, `diff -u`), or an XML or HTML start;
3. a valid JSON object or array (`curl … | peek`);
4. markdown, when there's a fenced code block, or at least three kinds of
   markdown (headings, lists, links, `**bold**` or `` `code` ``).

Anything else is plain text. Markdown is a guess, so ordinary output can
occasionally be taken for it; use `--lang` to override any of these, for example
`--lang text`.

Output that keeps coming (`tail -f app.log | peek`, `kubectl logs -f web |& peek`,
a dev server) is followed like `less +F`: peek opens after a moment and adds
lines as they arrive. While the cursor is on the last line in normal mode, it
moves to each new last line; move up and it stays where you put it, and `G`
resumes following. A line shows once it ends with a newline, or when the input
ends. Arriving text never counts as an edit, so `u` doesn't remove it and `:q`
still exits. The language is detected from the first output only.

With a plain `|`, the command's stderr still goes to the terminal and is
written over peek's screen. peek redraws when the input ends, and `ctrl+l`
redraws at any time, but `|&` keeps the messages in peek instead.

Run without a pipe or redirect, peek prints usage and exits. When the command prints nothing (a `go build` that succeeds), peek
prints `peek: no input` and exits instead of opening an empty screen.

## Keybindings

All the usual vim motions, search and visual mode work. The text can be edited
before saving it.

| Key          | Action                                 |
| ------------ | -------------------------------------- |
| `gx`         | open the http(s) link under the cursor |
| `:w <file>`  | save to a file (`~/` works)            |
| `:wq <file>` | save and exit                          |
| `:x <file>`  | save if you edited the text, and exit  |
| `:q` / `:q!` | exit                                   |
| `ctrl+c`     | exit                                   |
| `ctrl+l`     | redraw the screen                      |

`gx` opens only `http` and `https` links, in your default browser. The text
comes from other programs, so file paths and other URL schemes are refused:
opening them could start an app or run a script.

Unlike vim, `:q` never asks you to save first: the input is command output, so
edits are thrown away on exit unless you saved them to a file.

Piped input has no file name, so always give one when saving: a bare `:w`
shows `E32: No file name` and saves nothing, and a bare `:wq` or `:x` exits
without saving.
