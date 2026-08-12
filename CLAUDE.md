# CLAUDE.md

Context for an AI assistant working on this repository. Read this first.

---

## What this project is

**Quartz** is a programming language. Source files (`.qz`) are compiled
to Go source, which the Go toolchain compiles to a native executable.

```
hello.qz  ->  [Quartz compiler]  ->  main.go  ->  [go build]  ->  hello.exe
```

The compiler itself is written in Go, is a single package `main` in the
repository root, and has **zero external dependencies**. `go.mod` lists
nothing. Keep it that way unless there is a very good reason.

The user is a beginner-to-intermediate programmer building this as a
learning project, on **Windows 10**, using VS Code. They do not have
Linux or a VM. They know Git basics only (`add`, `commit`, `log`,
`status`, `checkout`, `--amend`).

**Current version: v0.16.** Working, committed, and tagged.

---

## Design goals, in priority order

1. **Native speed, single self-contained binary.** No runtime to
   install, no DLLs to ship, no cgo.
2. **Low ceremony.** No semicolons, no headers, no boilerplate `main`,
   no manual memory management. Closer to Python than to C++.
3. **Batteries included.** Math, strings, I/O, and native Windows GUI
   are all builtins requiring no imports.
4. **Honest errors.** Every error points at a `.qz` file, line, and
   column. Never claim something worked when it might not have.

---

## Repository layout

```
quartz/
  token.go          Token kinds, keyword table, Token struct
  lexer.go          source text -> []Token
  ast.go            AST node type definitions
  parser.go         []Token -> *Program   (recursive descent + Pratt)
  resolve.go        *Program -> checked   (scopes, names, arity, returns)
  codegen.go        *Program -> Go source (+ core builtin table)
  stdlib.go         math and string builtins
  runtime_win.go    Win32 builtins and injected Go helpers
  quartz.go         CLI driver, temp build dir, shells out to `go build`
  examples/*.qz     sample programs (also the de facto test suite)
  README.md         user-facing overview
  SYNTAX.md         full language reference
  CLAUDE.md         this file
```

---

## The compiler pipeline

Five stages. Each is a plain function taking the previous stage's
output. Each stage **gates** the next: if a stage produces errors, the
driver reports them and stops.

| Stage | File | In | Out |
|---|---|---|---|
| Lex | `lexer.go` | text | `[]Token` |
| Parse | `parser.go` | `[]Token` | `*Program` |
| Resolve | `resolve.go` | `*Program` | same, annotated |
| Codegen | `codegen.go` | `*Program` | Go source string |
| Build | `quartz.go` | Go source | `.exe` |

### Key invariants — do not break these

**Every token and AST node carries line and column.** `Token` has
`Line`/`Col`; every AST node embeds `pos`. Error messages and `//line`
directives depend on this. Any new AST node must embed `pos`.

**Errors accumulate, they do not abort.** Each stage collects a
`[]string` of `file:line:col: message` and keeps going. The driver
sorts them into source order (`sortByPosition` in `quartz.go`) and
prints them all. Never `return` on the first error inside a stage.

**Forward progress is guaranteed.** Parser loops compare `p.i` before
and after and force `p.advance()` if it did not move. This prevents
infinite loops on malformed input. Preserve this pattern in any new
loop over statements.

**`//line` directives are emitted before every statement.** Format:
`//line <abs-path>:<line>` starting at column 1, no indentation. This is
what makes Go's own errors point at `.qz` files. `codegen.line(node)`
does it. Call it at the top of every statement case.

**Codegen fully parenthesises binary expressions.** The parser already
encoded precedence in the tree shape, so `(a + b)` grouping means Go's
precedence can never silently disagree. Ugly output, zero bugs. Keep it.

---

## How to add a language feature

Every feature threads through all five stages. Follow this order.

1. **`token.go`** — add any new `Kind` constants. The `kindNames` array
   must stay in the exact same order as the const block, or `Kind.String()`
   returns the wrong name. Add keywords to the `keywords` map.
2. **`lexer.go`** — recognise the new syntax. Multi-character operators
   must be matched longest-first.
3. **`ast.go`** — define the node. Embed `pos`. Add the marker method
   (`func (*X) stmtNode() {}` or `exprNode()`).
4. **`parser.go`** — parse it. Add to `parseStmt`'s switch, and to the
   `synchronize()` recovery set if it starts a statement.
5. **`resolve.go`** — validate it. Declare/lookup names, check context.
6. **`codegen.go`** — emit Go for it. Call `c.line(st)` first.
7. **`SYNTAX.md`** — document it, including the limitations.
8. **`examples/`** — add or extend a program that exercises it.

---

## Builtins: how they work

Builtins are functions the compiler knows by name. There is no import
system, so this table *is* the standard library.

A builtin is a `builtin` struct (defined in `codegen.go`):

```go
type builtin struct {
    emit    func(args []string) string  // Go source given generated args
    imports []string                    // Go imports to pull in
    helpers []string                    // runtime helpers to inject
    minArgs, maxArgs int                // -1 max means variadic
    osOnly  string                      // "" = portable, else a GOOS
}
```

Three tables merge into one map at `init()` in `codegen.go`:

- core builtins — defined inline in `codegen.go`
- `stdlibBuiltins` — `stdlib.go`, registered by `registerStdlib()`
- `winBuiltins` — `runtime_win.go`, by `registerWindowsRuntime()`

**Imports and helpers are pulled in on demand.** A program using only
`print` emits `import "fmt"` and nothing else. `helperDefs` maps a
helper name to Go source plus its own imports and deps; `addHelper`
resolves deps recursively.

**The `float64()` trick.** Numeric builtins accept `int` *or* `float`
because every argument is wrapped in `float64(...)`. Converting a
`float64` to `float64` is a no-op in Go. See `f()` in `stdlib.go`. This
is a workaround for having no type checker and should be revisited once
one exists.

**Arity is checked in `resolve.go`**, not codegen, so errors carry
positions and appear alongside other resolver errors.

---

## Gotchas already discovered

These cost real debugging time. Do not reintroduce them.

**Go rejects unused local variables.** `let x = 5` with no read is valid
Quartz but invalid Go. The resolver tracks `LetStmt.Used`; codegen emits
`_ = x` only when false. Any new binding form needs the same treatment.

**`syscall.NewCallback` has a small fixed pool.** The Win32 window
procedure callback is created **once** at package level
(`__wndProcPtr`). Creating one per window eventually panics.

**Win32 message loops must be thread-pinned.** `runtime.LockOSThread()`
before creating the window; without it Go's scheduler moves the
goroutine and the window silently stops responding.

**Rounded corners are Windows 11 only.** `DWMWA_WINDOW_CORNER_PREFERENCE`
(attribute 33) needs build 22000+. `__roundCorners` checks `__winBuild()`
and returns a bool; `openWindow` returns it. **Never claim rounding
worked without checking** — this was a real bug that shipped.

**Number vs range lexing.** `number()` only consumes a fractional part
if a digit follows the dot. That is what lets `1..10` lex as
`NUMBER DOTDOT NUMBER` while `1.5` stays one token.

**String literals nest inside interpolations.** `"{upper("hi")}"` works.
The lexer tracks brace depth and a nested-quote flag, so a `"` only ends
the string at depth 0. The parser's brace matcher does the same when
finding the closing `}`. Both must stay in sync.

**Windows paths break naive error parsing.** `C:\...` contains a colon,
so `sortByPosition` has a fallback path for it.

**Never round-trip a file through PowerShell 5.1.** This corrupts UTF-8:

```powershell
(Get-Content x.md -Raw) -replace 'a','b' | Set-Content -Encoding utf8 x.md
```

Without a BOM, `Get-Content` decodes as ANSI, so every em-dash becomes
three Latin-1 characters and is written back re-encoded. It happened
once to `SYNTAX.md` and silently corrupted 62 characters across two
commits. Edit text files with the editor tooling, or use
`[System.IO.File]::ReadAllText` / `WriteAllText`, which default to
UTF-8 and round-trip cleanly.

**A PowerShell pipeline is not a byte stream.** It carries objects and
re-encodes text, so piping binary between two native programs corrupts
it:

```powershell
git archive HEAD | tar -x -C dest    # produces garbage
```

`tar` reports `Failed to open '\.	ape0'`, which is it falling back to a
default tape device because it never received an archive at all. Go via
a file instead: `git archive -o x.tar HEAD` then `tar -x -f x.tar`.
This broke release.ps1 on its first real run.

**PowerShell 5.1 mangles double quotes passed to a native exe.** A
commit message containing `"` gets split into extra arguments, and
`git commit -m` fails with a confusing `pathspec ... did not match`.
Write the message to a file and use `git commit -F <file>`.

---

## Verifying changes

Always run all four. The examples are the test suite.

```bash
gofmt -l $(git ls-files '*.go')   # must print nothing
go vet ./...                      # must be clean
go test ./...                     # must pass
go build -o quartz.exe .
./quartz.exe run examples/logic.qz
./quartz.exe run examples/demo.qz
```

**Use `git ls-files`, not `gofmt -l .`.** The latter walks into
`Working version/`, the untracked snapshot, whose files come from
`git archive` with CRLF endings and are therefore reported by gofmt
every time. That noise would hide a real formatting problem. `go vet`
and `go test` are unaffected: the snapshot has its own `go.mod`, so Go
treats it as a separate module and skips it.

For Windows code, cross-compile and vet the *generated* output:

```bash
QUARTZ_TARGET=windows ./quartz build examples/window.qz
mkdir -p /tmp/vw && QUARTZ_TARGET=windows ./quartz emit examples/window.qz > /tmp/vw/main.go
printf 'module qzprog\n\ngo 1.21\n' > /tmp/vw/go.mod
cd /tmp/vw && GOOS=windows go vet ./...
```

`quartz emit <file>` prints the generated Go. It is the single most
useful debugging tool here — when behaviour is wrong, read the output.

**One known vet exception.** `go vet` on the compiler is clean and must
stay that way. The *generated* program is too, with one exception: a
program using `win.clipboard.*` reports two "possible misuse of
unsafe.Pointer". That is Win32 memory from `GlobalAlloc`, which the Go
collector never moves, held between a `GlobalLock` and `GlobalUnlock`.
The reasoning is written out above the helper in `runtime_win.go`.
Silencing it needs `golang.org/x/sys`, which costs the zero-dependency
property to quiet one warning.

**You cannot run Windows GUI code in a Linux container.** Cross-compiling
and vetting proves it type-checks, nothing more. Say so plainly; let the
user confirm runtime behaviour. Do not assert that a window looked a
certain way.

---

## Working style the user expects

- **Test before handing over code.** Build it, run it, show the output.
- **Flag design problems, don't paper over them.** The `toInt("")`
  returning `0` issue and the rounded-corners lie were both surfaced as
  problems, not hidden.
- **Be explicit about which files replace which.** They copy files by
  hand into `C:\Users\john linux\Desktop\quartz`. Note when a browser
  might mangle a filename (`.gitignore` -> `gitignore.txt`).
- **Their path contains a space.** Quote paths in commands.
- Conversational, direct, no filler. Short prose over long bullet lists.
- Suggest a `git commit` after each working milestone.

---

## Current state (v0.15)

**The language**

- `let` / `const`, inference, optional `: type` annotations
- `int`, `float`, `str`, `bool`, and a real type checker in `check.go`
- Lists `[]T`, maps `{K: V}`, `struct` with `impl` methods
- First-class functions, closures, higher-order list operations
- Nullable `?T` with narrowing; error type `T!` with `?` propagation
- `match`, bitwise operators, hex/binary literals, digit separators
- Raw backtick strings; interpolation `"{expr}"` with nesting
- `if` / `while` / `for i in a..b` with `step`, `break`, `continue`
- Multi-file programs via `import` and `pub`
- Structured concurrency through `task` — no raw goroutines exposed
- Cross-compilation via `QUARTZ_TARGET`

**The library** — all failures reported as `T!`, never a panic:
`os`, `http`, `net`, `json`, `time`, `mem`, `task`, `re`, `hash`,
`csv`, plus `win` for the Windows-only parts.

**Tooling:** `quartz fmt`, `quartz doctor`, `quartz builtins`,
`quartz emit`, warnings for unused variables and unreachable code, a
VS Code extension in `editors/vscode`, and a verified Windows installer.

**Not implemented**

- Generics
- Databases — SQLite needs a Go dependency; flag the trade-off first
- Manual memory, pointers, `unsafe` — all need the C backend
- GUI event handling; `openWindow` opens a real but inert window
- Any GUI outside Windows

**Reserved but inert keywords:** `self defer own unsafe nil`

---

## Finding the Go toolchain

Quartz compiles to Go source and shells out to the Go toolchain, so one
has to exist. `findGo` in `toolchain.go` looks in three places, in this
order:

1. `$QUARTZ_GO`, to force a specific one
2. `go\bin\go.exe` next to `quartz.exe` — the installer's private copy
3. `PATH`

The bundled copy beats `PATH` on purpose, so an installed Quartz keeps
working the same way whatever else the machine picks up later. It is
also kept **off** `PATH` on purpose, so a developer's own Go is never
shadowed. When a bundled toolchain is used, codegen sets `GOROOT` for
the child process, since nothing else will have.

`quartz doctor` prints which one was found and where. That is the first
thing to ask for when someone says it does not work.

---

## Building the installer

```powershell
powershell -ExecutionPolicy Bypass -File installer\build.ps1
```

That builds `quartz.exe`, stages a trimmed GOROOT into `dist\stage\go`,
proves the staged toolchain can compile `hello.qz` with `PATH` stripped,
and only then spends two minutes on LZMA2. Output is
`dist\quartz-<version>-setup.exe`.

Things learned doing it, worth not rediscovering:

- **Inno Setup installs per-user via winget**, to
  `%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe`, not to
  `Program Files`. `build.ps1` checks all three locations.
- **A full GOROOT is ~225 MB; trimmed it is 177 MB; compressed into the
  installer it is 36 MB.** The trimming drops `api`, `test`, `doc`,
  `misc`, every `testdata` directory and every `*_test.go`. None of it
  is needed to compile someone else's program.
- **The first compile against a fresh bundled toolchain takes ~13
  seconds** because Go is building its standard library into an empty
  cache. It is ~1.6 s afterwards. This looks like a hang exactly once.
- **`dist/` must stay gitignored.** It holds a 177 MB copy of Go.
- **Verify by installing it.** Silent-install to a temp directory with
  `/DIR=... /TASKS=` so no PATH or registry entries are touched, strip
  `PATH` to `System32`, compile something, then run `unins000.exe
  /VERYSILENT`. That is how the current script was checked; do not
  weaken it to "it compiled".

---

## Next milestone: the C backend (11.1)

v1.0 is essentially done. The next real step is emitting C instead of
Go behind the same AST, because it is the prerequisite for everything
low-level: `own T`, `defer`, `alloc`/`free`, pointers, and `unsafe`.

Design the IR boundary so it is one module swapped and not a rewrite.
Keep the Go backend as the default; put C behind `QUARTZ_TARGET=c`.
Concurrency is the hard part — `task` maps almost directly onto
goroutines and not at all onto C, which is an argument for pinning the
API down before the backend exists rather than after.

The honest framing for the user: compiling through Go is a normal
technique, not a fake language — but it does cost a garbage collector,
a runtime, a ~2 MB floor on binary size, and any hope of manual memory.
The C backend is what buys those back.
