# Quartz compiler internals

How the compiler is put together, and why it is put together that way.
For the language itself see [SYNTAX.md](SYNTAX.md); for what to build
next see [ROADMAP.md](ROADMAP.md).

---

## Contents

1. [The shape of the thing](#the-shape-of-the-thing)
2. [Stage 1 — lexer](#stage-1--lexer)
3. [Stage 2 — parser](#stage-2--parser)
4. [Stage 3 — resolver](#stage-3--resolver)
5. [Stage 4 — checker](#stage-4--checker)
6. [Stage 5 — codegen](#stage-5--codegen)
7. [Stage 6 — the Go handoff](#stage-6--the-go-handoff)
8. [The builtin system](#the-builtin-system)
9. [Invariants](#invariants)
10. [Debugging](#debugging)
11. [Testing](#testing)

---

## The shape of the thing

Quartz is a single Go package `main` in the repository root, with zero
external dependencies. `go.mod` lists nothing, and that is deliberate:
the compiler must build anywhere Go builds, with no network.

Six stages, each a plain function over the previous stage's output:

```
hello.qz
   |  lexer.go        text -> []Token
   v
 tokens
   |  parser.go       []Token -> *Program
   v
  AST
   |  resolve.go      names, scopes, arity, return paths
   v
checked AST
   |  check.go        a Type for every expression
   v
 typed AST
   |  codegen.go      -> Go source, with //line directives
   v
 main.go
   |  quartz.go       shells out to `go build`
   v
hello.exe
```

Each stage **gates** the next. If a stage produces errors, the driver
prints them and stops — the next stage never runs, so no stage has to
defend itself against nonsense from the one before it. That is what
lets codegen be as simple as it is: by the time it runs, every name
exists, every call has the right arity, and every expression has a type.

**Why compile to Go rather than to machine code or C?** Go gives us a
garbage collector, a cross-compiler for every major platform, and a
linker producing static binaries, for free. The cost is that Go must be
installed to build a Quartz program. A C backend is planned for the
low-level work that Go cannot express (see ROADMAP 11.1), and the AST is
the intended seam.

---

## Stage 1 — lexer

`lexer.go`. A hand-written scanner over a `string`, producing
`[]Token`. No regexps, no generated tables.

Three parts of it are worth knowing about.

**Numbers versus ranges.** `number()` consumes a fractional part only if
a digit follows the dot. That single rule is what lets `1..10` lex as
`NUMBER DOTDOT NUMBER` while `1.5` stays one token. Break it and every
`for` loop stops parsing.

**Nested block comments.** `/* ... /* ... */ ... */` tracks depth, so a
region containing a comment can itself be commented out.

**String interpolation.** The lexer keeps a brace depth and a
nested-quote flag while scanning a string, so a `"` inside `{...}` does
not end the literal. This is what makes `"{upper("hi")}"` work. The
parser's brace matcher (`parseStringLit`) implements the same rule
independently — **the two must stay in sync**, and a change to one
without the other produces baffling failures.

Every token carries `Line` and `Col`, 1-based. Nothing downstream can
recover them if they are dropped.

---

## Stage 2 — parser

`parser.go`. Recursive descent for statements, Pratt (precedence
climbing) for expressions, producing the node types in `ast.go`.

`parseExpr(minPrec)` is the Pratt loop; `precOf` is the precedence
table. Adding a binary operator means adding a `Kind`, a `precOf` entry,
and a `goBinOp` case — nothing else.

**Error recovery.** On a parse error the parser calls `synchronize()`,
which skips forward to the next token that plausibly starts a statement.
That lets one bad line produce one error instead of a cascade.

**Forward progress.** Every loop over statements compares `p.i` before
and after the iteration and forces `p.advance()` if it did not move.
Without it, a token no rule consumes spins forever. Preserve this
pattern in any new statement loop.

---

## Stage 3 — resolver

`resolve.go`. Walks the tree and answers the questions of *existence*,
not of *type*:

- Does this name exist, and is it in scope?
- Is this assignment writing to a `const`?
- Does this call have the right number of arguments?
- Does a function with a return type return on every path?
- Is this `break` or `continue` actually inside a loop?
- Is this variable ever read?

That last one exists to serve Go, not Quartz. Go rejects unused locals,
but `let x = 5` with no read is perfectly legal Quartz. The resolver
records `LetStmt.Used`, and codegen emits `_ = x` only where it is
false. **Any new binding form needs the same treatment** or the
generated program will not compile.

Return-path analysis (`blockReturns`) is deliberately conservative: it
understands a trailing `return` and an `if`/`else` where both branches
return, and nothing else. A function whose only `return` is inside a
loop is rejected even though it is provably fine. False negatives are
annoying; false positives would generate invalid Go.

---

## Stage 4 — checker

`check.go`, with the type representation in `types.go`. Structurally
parallel to the resolver, but each expression method *returns a type*
rather than only validating.

**`Type` is a struct, not an enum.** It has a `Kind` plus `Elem` and
`Key` pointers, so `[][]int` and `{str: []int}` need no special cases —
composition falls out of the representation. This was a deliberate
choice made while only scalars existed, because retrofitting nesting
onto an `int` enum later would have meant touching every use.

**`KUnknown` suppresses cascades.** When a check fails, the checker
reports once and returns `Unknown`. Every operation involving `Unknown`
succeeds silently and propagates it. One mistake yields one error. The
`tests/err/cascade.qz` case exists to keep it that way.

**`KNumeric` and `KAny` are signature-only.** They never appear as the
type of a real expression; they exist so a builtin can declare "any
number" or "anything". `Accepts` is where they are interpreted.

**Untyped integer literals.** `isUntypedInt` reports whether an
expression is built purely from integer literals. Such an expression may
stand in for a float, exactly as Go's untyped constants do, so
`radius * 2` works while `radius * someInt` is an error. This is the one
concession to convenience in an otherwise strict system, and it costs
codegen nothing — Go has the identical concept, so the emitted `2`
simply becomes a float64 constant on the Go side.

Error messages use Quartz's vocabulary. A user should never see
`float64` or `string`. `Type.String()` is the only place that decides
how a type is spelled, and `Type.Go()` is the only place that knows the
Go spelling.

The checker writes its results back onto the AST — `LetStmt.T`,
`Param.T`, `FnDecl.RetT`, `Binary.T` — so codegen can emit explicit Go
types instead of leaning on Go's inference.

---

## Stage 5 — codegen

`codegen.go`. Walks the typed AST and appends to a `strings.Builder`.

**Every binary expression is fully parenthesised.** The parser already
encoded precedence in the shape of the tree, so emitting `(a + b)`
guarantees Go's precedence rules can never silently disagree with
Quartz's. The output is ugly. It is also impossible to get wrong.

**Every statement is preceded by a `//line` directive**, at column 1,
naming the absolute path of the `.qz` file and the source line. This is
the single most valuable thing in the generated output: it means a type
error from the Go backend points at the user's Quartz line rather than
at generated code they never wrote. `c.line(node)` emits it, and it must
be called at the top of every statement case.

**Imports and helpers are pulled in on demand.** A program using only
`print` emits `import "fmt"` and nothing else. `helperDefs` maps a
helper name to Go source plus its own imports and dependencies;
`addHelper` resolves those dependencies recursively. The header is
assembled *after* the body is walked, since only then is the set known.

**Every binding gets an explicit Go type.** `var x int = ...`, never
`var x = ...`. The checker knows the type, so stating it keeps Go's
inference from quietly reaching a different conclusion.

---

## Stage 6 — the Go handoff

`quartz.go`. Writes `main.go` and a minimal `go.mod` into a temp
directory, runs `go build`, and moves the binary next to the source.
The temp directory is removed afterwards.

`moveFile` falls back to copy-then-delete because `os.Rename` fails
across volumes, and the temp directory frequently is on one.

`sortByPosition` orders accumulated errors into source order so a run of
them reads top to bottom. It parses `file:line:col:` out of the message
string, with a fallback path for Windows absolute paths, whose leading
`C:` shifts every colon-delimited field.

---

## The builtin system

There is no import system, so the builtin table *is* the standard
library. A builtin is a `builtin` struct (`codegen.go`) carrying:

- `emit` — given already-generated argument expressions, return Go source
- `imports`, `helpers` — pulled in only when the builtin is used
- `minArgs`, `maxArgs` — arity, checked by the **resolver**
- `osOnly` — restrict to one GOOS; `""` is portable
- `params`, `rest`, `ret`, `retOf` — the type signature, checked by the
  **checker**

Three tables merge into one map in `init()`:

| Table | File | Registered by |
| --- | --- | --- |
| core | `codegen.go` | inline |
| `stdlibBuiltins` | `stdlib.go` | `registerStdlib()` |
| `winBuiltins` | `runtime_win.go` | `registerWindowsRuntime()` |

Registration is called explicitly from codegen's `init()` rather than
relying on Go's cross-file initialisation order, so it is deterministic.

**Arity is checked in the resolver, types in the checker.** Both carry
source positions, so both appear alongside every other error rather than
crashing out separately.

**The `float64()` wrapper.** Math builtins declare parameters as
`Numeric` and wrap arguments in `float64(...)`. This is not a hack
working around the absence of types: the signature genuinely promises to
accept `int` or `float`, and the wrapper is the widening that promise
implies. `float64` to `float64` is a no-op in Go.

### The Windows half

`runtime_win.go` reaches Win32 through `syscall.NewLazyDLL`, which loads
DLLs at runtime. No cgo, no C compiler, no DLLs to ship.

Three things there will bite:

- **`syscall.NewCallback` has a small fixed pool.** The window procedure
  is created **once**, at package level (`__wndProcPtr`). One per window
  eventually panics.
- **Message loops must be thread-pinned.** `runtime.LockOSThread()`
  before creating the window. Without it Go's scheduler migrates the
  goroutine and the window silently stops responding.
- **Version detection must use `RtlGetVersion`.** `GetVersionEx` lies on
  unmanifested binaries, reporting 6.2 for both Windows 10 and 11, which
  would make every version check useless.

Rounded corners (`DWMWA_WINDOW_CORNER_PREFERENCE`, attribute 33) require
build 22000+. `__roundCorners` checks the build and **returns whether it
actually worked**; `openWindow` returns that value. Never report success
that was not verified — a previous version claimed rounded corners on a
Windows 10 machine where they are impossible, and that shipped.

---

## Invariants

Breaking any of these produces failures that are hard to trace back.

1. **Every token and AST node carries line and column.** New AST nodes
   must embed `pos`.
2. **Errors accumulate; stages do not abort.** Each stage appends to a
   `[]string` and keeps going. Never `return` on the first error.
3. **Parser loops guarantee forward progress.** Compare the index before
   and after; force an advance if it did not move.
4. **`//line` directives precede every emitted statement**, at column 1.
5. **Codegen fully parenthesises binary expressions.**
6. **`KUnknown` propagates silently.** Report once, then stay quiet.
7. **Type names are spelled in Quartz** in every user-facing message.
8. **No Go module dependencies.** `go.mod` stays empty.

---

## Debugging

`quartz emit <file>` prints the generated Go. When behaviour is wrong,
read it before theorising — it is faster than reasoning about what
codegen should have done.

`quartz tokens <file>` prints the token stream, for when the problem is
further upstream than it looks.

Verify a change with all of:

```
gofmt -l .          # must print nothing
go vet ./...        # must be clean
go test ./...       # must pass
go build -o quartz.exe .
```

For Windows-only code on a non-Windows machine, cross-compiling and
vetting the *generated* output proves it type-checks and nothing more.
It does not prove a window appeared. Say so.

---

## Testing

`quartz_test.go` is a data-driven harness. A case is two files:

```
tests/ok/NAME.qz   + tests/ok/NAME.expected     expected program output
tests/err/NAME.qz  + tests/err/NAME.expected    expected compiler errors
```

Cases in `tests/ok` must compile, run, and match their output. Cases in
`tests/err` must **fail to compile** and match their error text — which
means the wording and ordering of error messages is itself under test,
deliberately, because error quality is a feature here.

Golden files are normalised: absolute paths are reduced to the base
filename and CRLF is folded to LF, so they are identical on every
machine.

Regenerate after an intentional change:

```
go test -run TestQuartz -update .
```

Then read the diff. That flag is exactly how a bug becomes the expected
behaviour if you let it.
