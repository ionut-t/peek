You are reviewing peek, a Go CLI tool — a lightweight TUI for reading command output with vim keybindings, built on Bubble Tea v2 and goeditor. It reads all of stdin (a pipe or a redirected file), then shows it in a vim-style view with Chroma syntax highlighting; keys come from `/dev/tty`. The code is a single `main` package: `main.go` (flags, stdin, the model, `:w` saving), `detect.go` (language detection), `open.go` (`gx`) and per-OS `filepath_darwin.go` / `filepath_linux.go` / `filepath_other.go`. Focus your review on the areas below.

## Untrusted Input

The text peek shows comes from other programs, so treat it as untrusted.

- Flag any path where input text reaches a shell, `exec.Command`, or the system opener without validation — `gx` must open only `http`/`https` URLs with a host; file paths, `file:` and custom schemes must stay refused
- Flag weakening of the checks in `openLink` (e.g. matching the scheme case-sensitively, or accepting a URL without a host)
- Flag input that could be written to the terminal as raw control or escape sequences rather than rendered as text
- Flag detection or parsing code that could be very slow on large or adversarial input (unbounded regexes, repeated full scans, quadratic loops)

## Stdin and Terminal

- Flag reads from stdin after the initial `io.ReadAll` — stdin is the pipe; Bubble Tea reads keys from the terminal by itself
- Flag changes that remove the terminal check before reading — without a pipe or redirect, `ReadAll` would wait on the keyboard
- Flag changes to the empty-input path: input that is only white space must print `peek: no input` and exit, not open an empty screen
- Flag `log.Fatal` or `os.Exit` after the Bubble Tea program has started — the terminal must be restored first

## Language Detection

- Flag changes to the order in `detectLanguage` that put a less certain rule before a surer one: file name, then first-line markers, then JSON, then markdown
- Flag new heuristics that are loose enough to misfire on ordinary command output (`git log`, `ps aux`, compiler errors, tables); markdown in particular must stay a strict guess
- Flag use of Chroma's content analyser (`lexers.Analyse`) — it was dropped for misdetecting real output
- Flag detection changes without tests using realistic samples of the output they target and output they must not match

## Platform-Specific Code

- Flag a change to `filePath` in one `filepath_*.go` file that isn't matched in the others — all three must keep the same signature and return `""` when stdin isn't a regular file
- Flag new OS-specific code with no fallback for other systems, or build tags that leave a platform without an implementation
- Flag `unsafe` use without the pointer kept valid for the whole call (see the `runtime.Pinner` in `filepath_darwin.go`), and unchecked results from system calls

## Saving

- Flag writes that could happen without an explicit file name — a bare `:w` must return `E32: No file name`
- Flag changes to `~/` expansion that would also expand `~user/` or a `~` elsewhere in the path
- Flag changes to file permissions or the trailing-newline rule in `writeContent` without a stated reason
- Flag new prompts or confirmations on `:q` — exiting without saving is intended, since the input is command output

## Bubble Tea Model

- Flag new blocking or long-running work inside `Update()` — offload it to a `tea.Cmd`
- Flag model changes made to a copy that is never returned — `model` is a value type
- Flag `View()` implementations that mutate the model
- Flag theme or language changes that aren't applied in both `newModel` and the `tea.BackgroundColorMsg` handler, so the light/dark switch drops them
- Flag errors shown to the user without going through `DispatchError` on the status line

## Error Handling

- Flag ignored errors (`_` on error returns), except where the fallback is deliberate and commented (e.g. `filePath` returning `""`)
- Flag errors returned without context — wrap with `fmt.Errorf("context: %w", err)` — unless the message is shown on the status line as-is, vim-style
- Flag errors compared with `==` instead of `errors.Is()` or `errors.As()`

## Docs and Tests

- Flag user-visible changes (flags, keybindings, detection rules, refused actions) that don't update the README
- Flag changes to `flag.Usage` or the `--lang` help text that no longer match the README
- Flag new behaviour without tests; prefer table-driven tests, and swap side effects through package variables (as `openWithSystem` is) rather than calling the real system opener

## General Go

- Flag comments that describe what the code does rather than why, or that no longer match the code
- Flag functions longer than ~40 lines that could be meaningfully split
- Flag deeply nested logic that could be flattened with early returns
- Flag global mutable state other than the test seams (`openWithSystem`)
