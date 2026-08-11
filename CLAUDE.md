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

**Current version: v0.3.** Working and committed.

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

**PowerShell 5.1 mangles double quotes passed to a native exe.** A
commit message containing `"` gets split into extra arguments, and
`git commit -m` fails with a confusing `pathspec ... did not match`.
Write the message to a file and use `git commit -F <file>`.

---

## Verifying changes

Always run all four. The examples are the test suite.

```bash
gofmt -l .          # must print nothing
go vet ./...        # must be clean
go build -o quartz .
./quartz run examples/logic.qz
./quartz run examples/demo.qz
```

For Windows code, cross-compile and vet the *generated* output:

```bash
QUARTZ_TARGET=windows ./quartz build examples/window.qz
mkdir -p /tmp/vw && QUARTZ_TARGET=windows ./quartz emit examples/window.qz > /tmp/vw/main.go
printf 'module qzprog\n\ngo 1.21\n' > /tmp/vw/go.mod
cd /tmp/vw && GOOS=windows go vet ./...
```

`quartz emit <file>` prints the generated Go. It is the single most
useful debugging tool here — when behaviour is wrong, read the output.

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

## Current state (v0.3)

**Implemented**

- `let` / `const`, type inference, optional `: type` annotations
- `int`, `float`, `str`, `bool`
- Full operator precedence via Pratt parsing; `+ - * / %`,
  `== != < <= > >=`, `&& || !`, `+= -= *= /=`
- String interpolation `"{expr}"`, with nesting and `{{` escapes
- `if` / `else if` / `else`, `while`
- `for i in a..b`, `..=` inclusive, `step` (negative counts down)
- `break`, `continue`
- `fn` with typed params, optional return type, recursion,
  order-independent declaration, return-path checking
- ~60 builtins: I/O, conversion, math, trig, random, strings
- Constants `PI`, `E`, `INF`, `NAN`
- Windows: `setTitle`, `beep`, `messageBox`, `hideConsole`, `winBuild`,
  `isWin11`, `openWindow`
- Cross-compilation via `QUARTZ_TARGET`

**Not implemented**

- Type checker (biggest gap)
- Lists, maps, structs
- JSON, file I/O, databases
- Modules / `import`
- Global variables
- Error handling of any kind
- Manual memory, pointers, `unsafe`

**Reserved but inert keywords:** `struct impl self match pub defer own
unsafe import nil`

---

## Next milestone: the type checker (v0.4)

This is the agreed next step and it is a hard dependency for almost
everything after it.

### Why it blocks everything

The moment a user writes `let xs = []`, codegen must emit a Go type.
`[]int`? `[]string`? Nothing in the compiler currently tracks the type
of an expression. Lists, maps, structs, JSON, and database rows all
need this. It is not a nice-to-have.

It also fixes two visible problems:

- Type errors currently surface from the Go backend in Go's vocabulary
  (`string`, `float64`) rather than Quartz's (`str`, `float`).
- `7 / 2` truncates to `3` because the compiler cannot distinguish
  integer division from float division.

### Suggested shape

A new file `check.go`, structurally parallel to `resolve.go`. Walk the
tree, but return a type from each expression instead of only validating
names.

```go
type Type int
const (
    TypeUnknown Type = iota  // error already reported; suppress cascades
    TypeInt
    TypeFloat
    TypeStr
    TypeBool
    TypeVoid
)

func (c *Checker) expr(e Expr) Type   // returns the type, reports mismatches
func (c *Checker) stmt(s Stmt)
```

Points to handle:

- Store the inferred type on `LetStmt` so codegen can emit an explicit
  Go type rather than relying on `var x = ...`.
- Verify an explicit `: type` annotation matches the value's type.
- Binary operators: `+` works on `int`, `float`, `str`; the rest are
  numeric only; comparisons yield `bool`; `&&`/`||` require `bool`.
- Numeric promotion: decide whether `int + float` is legal (implicit
  widening) or an error requiring `float(x)`. **Recommend: an error.**
  It matches the language's existing no-implicit-conversion rule and
  keeps codegen simple.
- Builtins need declared signatures. Currently `builtin` has only arity.
  Add param types and a return type, which also removes the `float64()`
  wrapping hack.
- Function calls: check argument types against parameter types.
- Once types are known, fix `/`: emit integer division for two `int`s
  and float division otherwise, and consider making `divf` redundant.
- `TypeUnknown` must suppress downstream errors so one mistake does not
  cascade into ten.

### After that

lists and maps -> structs and `impl` -> JSON -> file I/O and SQLite ->
error type `T!` with `?` propagation -> modules -> C backend for
`unsafe` and manual memory.

SQLite will require a Go dependency (`database/sql` plus a driver),
which breaks the zero-dependency property. Flag that trade-off to the
user before doing it.
