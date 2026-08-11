# ROADMAP.md

Working practices, current state, and the full backlog for Quartz.

Read `CLAUDE.md` first for architecture and invariants. This file covers
**how we work**, **where we are**, and **what to build next**.

---

## Part 1 — How we use Git

### The setup that exists

The repo was created locally with `git init`. There is **no GitHub
remote**. Everything lives on the user's machine at
`C:\Users\john linux\Desktop\quartz`. Nothing has ever been pushed, which
matters because it means history can be safely rewritten.

Identity was configured globally:

```bash
git config --global user.name "John"
git config --global user.email "..."
```

### The workflow actually in use

Deliberately minimal. The user knows these commands and no others:

```bash
git status                    # what changed
git add .                     # stage everything
git commit -m "message"       # save a snapshot
git log --oneline             # list snapshots
git commit --amend --no-edit  # fold changes into the last commit
git tag v0.3                  # name a version
git tag -f v0.3               # move a tag after an amend
git checkout .                # discard uncommitted changes (panic button)
```

**Commit rhythm:** one commit per working milestone, made immediately
after verifying that the compiler builds and the examples run. Not at
the end of a session, and not when something is half-finished.

**`--amend` is used freely** because nothing is pushed. When the user
rebuilds or copies in a corrected file after committing, the fix folds
into the same commit rather than creating a "fix typo" commit. If a tag
already points at the old commit, it must be moved with `git tag -f`.

Once a remote exists, this changes: amend only commits that have not
left the machine.

### `.gitignore`

Ignores build output and editor noise:

```
quartz
quartz.exe
qzc
examples/*.exe
examples/demo
examples/interactive
examples/hello
.vscode/
.idea/
*.swp
Thumbs.db
desktop.ini
```

Compiled binaries are ~2 MB. Committing them on every build would bloat
the repo fast. If `git status` ever lists `quartz.exe`, the ignore file
is not being read — on Windows this is almost always because a browser
download saved it as `gitignore.txt`. Windows hides known extensions, so
it *looks* correct in Explorer. Check with `dir /a`.

### The CRLF warning

Git for Windows prints:

```
warning: in the working copy of '...', LF will be replaced by CRLF
```

This is normal and expected. Git stores Unix line endings internally and
converts on checkout. It can be silenced with
`git config --global core.autocrlf true`. It is not an error and needs
no action.

### Conventions for commit messages

Short, imperative, describing the milestone:

```
v0.1: lexer, parser, codegen, hello world compiles to exe
v0.2: functions, resolver, builtins, Windows GUI
v0.3: for loops, break/continue, math and string stdlib
v0.3.1: honest corner rounding, winBuild/isWin11, full docs
```

Version bumps are tagged. Intermediate work commits do not need a
version prefix.

### What to do at the start and end of a work session

**Start:** `git status` to confirm a clean tree. If dirty, ask the user
whether to commit or discard before making changes.

**During:** commit after each feature that builds and passes the
examples. Do not batch several features into one commit.

**End:** ensure everything is committed and, if a version milestone was
reached, tagged.

---

## Part 2 — Where we are

**Version 0.4.** The compiler works, produces native executables, has a
real standard library, and now type-checks.

### Pipeline

```
.qz  ->  lexer  ->  parser  ->  resolver  ->  checker  ->  codegen  ->  Go  ->  .exe
```

Six stages, each gating the next. Errors accumulate within a stage,
are sorted into source order, and are all reported at once.

### What works

| Area | Detail |
|---|---|
| Variables | `let`, `const`, inference, `: type` annotations, shadowing |
| Types | `int`, `float`, `str`, `bool` |
| Operators | full Pratt precedence, `+ - * / %`, comparisons, `&& \|\| !`, `+= -= *= /=` |
| Strings | interpolation `"{expr}"`, nested literals, `{{` escapes |
| Control flow | `if` / `else if` / `else`, `while`, `for i in a..b`, `..=`, `step`, `break`, `continue` |
| Functions | typed params, return types, recursion, order-independent, return-path checking |
| Stdlib | ~60 builtins: I/O, conversion, math, trig, random, strings |
| Constants | `PI`, `E`, `INF`, `NAN` |
| Windows | `setTitle`, `beep`, `messageBox`, `hideConsole`, `winBuild`, `isWin11`, `openWindow` |
| Tooling | `run`, `build`, `emit`, `tokens`, cross-compilation via `QUARTZ_TARGET` |

### What does not exist

No lists, maps, or structs. No JSON, files, or databases. No modules.
No globals. No error handling. No pointers or manual memory. GUI is
Windows-only and windows are blank.

### The gate is open

**The type checker is done** (`types.go` + `check.go`). Every expression
has a type, errors are reported in Quartz's vocabulary, and the results
are written back onto the AST so codegen emits explicit Go types.

`Type` was built as a struct with `Elem`/`Key` pointers rather than a
flat enum, so nested types like `[][]int` and `{str: []int}` need no new
machinery — v0.5 should mostly be parser and codegen work.

Two decisions were made and are worth knowing before building on them:

- **No implicit conversion between values**, as recommended. `a + b`
  with an `int` and a `float` is an error.
- **Integer literals are untyped**, following Go. `radius * 2` works
  where radius is a float; `radius * someInt` does not. This was a
  refinement of the recommendation, not a departure from it — without
  it, ordinary arithmetic needs `float(...)` everywhere.

A data-driven test suite also landed early (ROADMAP 10.1), because
unattended work on the compiler is not safe without one.

---

## Part 3 — The backlog

Ordered by dependency. Items within a milestone can mostly be done in
any order; milestones should be done in sequence.

Each item notes **files touched** and **done when**.

---

### v0.4 — Type checker — **DONE**

The gate. Nothing below this ships without it. All items below are
implemented; kept here as a record of what was agreed and built.

**4.1 Type representation**
Add `check.go` with a `Type` value covering `int`, `float`, `str`,
`bool`, `void`, and `unknown`. `unknown` is the error-suppression type:
once an expression is `unknown`, downstream checks stay quiet so one
mistake does not cascade into ten.
*Files:* new `check.go`. *Done when:* the type enum exists and prints
Quartz names (`str`, not `string`).

**4.2 Expression typing**
`func (c *Checker) expr(e Expr) Type` covering every expression node.
Literals are obvious. `Ident` looks up the declared type. `Unary`
requires numeric for `-` and `bool` for `!`.
*Done when:* every expression node returns a type.

**4.3 Binary operator rules**
`+` on `int`, `float`, `str`. `- * / %` numeric only. Comparisons yield
`bool`. `&&` / `||` require `bool` operands. Mismatched operands are an
error naming both types.
*Done when:* `"x" + 5` reports a Quartz error with a Quartz message,
not a Go one.

**4.4 Decide on numeric promotion**
Is `1 + 2.5` legal? **Recommendation: no.** Require `float(1) + 2.5`.
It matches the existing no-implicit-conversion rule and keeps codegen
simple. Document the decision either way — this is a language design
choice, so surface it to the user rather than deciding silently.

**4.5 Statement checking**
`if` / `while` conditions must be `bool`. `let` with an annotation must
match its value. Assignment must match the declared type. `return` must
match the function's return type.
*Done when:* `if 5 { }` is an error.

**4.6 Store inferred types on the AST**
Add a `Type` field to `LetStmt` and `Param`. Codegen then emits
`var x int = ...` instead of relying on Go inference.
*Files:* `ast.go`, `check.go`, `codegen.go`.

**4.7 Typed builtin signatures**
`builtin` currently has only arity. Add parameter types and a return
type. This lets calls be checked properly **and removes the
`float64()`-wrapping hack** in `stdlib.go`. Some builtins are
polymorphic (`min`, `max`, `str`, `print`) — allow a signature to mark
a parameter as "any numeric" or "any".
*Files:* `codegen.go`, `stdlib.go`, `runtime_win.go`, `check.go`.
*Done when:* `sqrt("hello")` is a compile error.

**4.8 Function call checking**
Argument types against parameter types, with the position of the
offending argument.

**4.9 Fix integer division**
With types known, `/` on two `int`s emits integer division and anything
else emits float division. Then decide whether `divf` stays as an alias
or is deprecated.
*Done when:* the `7 / 2` trap is documented as resolved in `SYNTAX.md`.

**4.10 Wire the checker into the driver**
Between resolve and codegen. Errors gate codegen like every other stage.
*Files:* `quartz.go`.

**4.11 Update docs**
`SYNTAX.md` gains a Types section describing the rules. The "no type
checker" line leaves Known Limitations in both `SYNTAX.md` and
`README.md`.

---

### v0.5 — Collections

**5.1 List type and literals**
`[]int`, `[]str`, etc. Literal syntax `[1, 2, 3]`. Empty literal `[]`
needs an annotation (`let xs: []int = []`) since it cannot be inferred —
report a clear error saying exactly that.
*Files:* `token.go` (already has brackets), `parser.go`, `ast.go`,
`check.go`, `codegen.go`.

**5.2 Indexing**
`xs[0]` for read and `xs[0] = v` for write. Decide bounds behaviour:
Go panics. **Recommendation:** emit a bounds-checked helper that reports
a clean Quartz-level runtime message rather than a Go stack trace.

**5.3 List builtins**
`push`, `pop`, `len` (extend the existing one), `slice`, `contains`,
`indexOf`, `reverse`, `sort`, `join`, `clear`, `first`, `last`.

**5.4 `for x in list`**
Extend `ForStmt` with a collection form alongside the range form.
Emit `for _, x := range xs`.
*Done when:* iterating a list works and the loop variable is typed.

**5.5 Map type and literals**
`{K: V}`, literal `{"a": 1}`. Keys limited to `str` and `int` initially.

**5.6 Map operations**
Index, assign, `has`, `delete`, `keys`, `values`, `len`. Decide what a
missing key returns — this is where nullable types start to matter.

**5.7 `for k, v in map`**
Note Go's map iteration order is random. Either document that or emit
sorted iteration for determinism. **Recommendation:** sort, because
beginners find random ordering baffling.

**5.8 `split` and `chars`**
Now possible: `split(s, sep) -> []str`, `chars(s) -> []str`. Add
`join(list, sep)` as the inverse.

**5.9 Nested collections**
`[][]int`, `{str: []int}`. Mostly falls out of the type representation
if it was designed with nesting in mind — verify with tests.

---

### v0.6 — Structs

**6.1 Declaration**
```qz
struct User {
    name: str
    age:  int
}
```
`struct` is already a reserved keyword.

**6.2 Literals and field access**
`User{name: "a", age: 1}`, then `u.name`. `DOT` already lexes; the
parser needs a postfix field-access rule.

**6.3 `impl` blocks and methods**
```qz
impl User {
    fn greet(self) -> str { return "hi, {self.name}" }
}
```
`impl` and `self` are already reserved.

**6.4 Structs in collections**
`[]User`, `{str: User}`.

**6.5 Decide value vs reference semantics**
Go structs are values; assignment copies. Either match that and document
it, or emit pointers. **Recommendation:** values, matching Go, because
it avoids introducing pointer semantics before there is a story for
nullability.

---

### v0.7 — Nullability and errors

**7.1 `?T` nullable types**
Plain `T` can never be nil; `?T` can. This is the single biggest thing
Quartz can offer over Go, whose nil-pointer panics are its worst wart.

**7.2 Nil checks and narrowing**
`if x != nil { ... }` should narrow `?T` to `T` inside the block.

**7.3 `T!` error type**
A function returning `str!` returns either a value or an error.

**7.4 `?` propagation operator**
`let text = read(path)?` returns early on error. Replaces Go's
four-line `if err != nil` dance with one character.

**7.5 `match` for explicit handling**
```qz
match load("cfg") {
    ok(text) => print(text)
    err(e)   => print("failed: {e}")
}
```
`match` is already reserved.

**7.6 Revisit fallback-returning builtins**
`toInt`, `charAt`, `substr` currently return fallbacks on failure. With
`T!` available, decide which should return errors instead. This closes
the design issue the user found in v0.2 with `toInt("")` returning `0`.

---

### v0.8 — Files, JSON, data

**8.1 File I/O**
`readFile`, `writeFile`, `appendFile`, `exists`, `deleteFile`,
`listDir`, `makeDir`. All return `T!` once errors exist.

**8.2 Path helpers**
`joinPath`, `basename`, `dirname`, `extname`, `absPath`.

**8.3 JSON**
`parseJson(s)` and `toJson(v)`. Needs a dynamic value type or generics
over structs — this is a real design decision. **Recommendation:** a
`Json` union type that can be inspected, rather than reflection-based
struct mapping, which is much harder without a full generic system.

**8.4 CSV**
`parseCsv`, `writeCsv`. Easy once lists exist.

**8.5 Environment and process**
`env(name)`, `args()`, `run(cmd)`, `cwd()`.

**8.6 Time**
`now()`, `timestamp()`, `formatTime`, `parseTime`, `elapsed`.

**8.7 SQLite**
`openDb`, `query`, `exec`, `close`. **Flag to the user first:** this
requires a Go module dependency (`database/sql` plus a driver), which
breaks the project's zero-dependency property. Options are to accept it,
to vendor a pure-Go driver, or to shell out to the `sqlite3` binary. Do
not just add a dependency silently.

---

### v0.9 — Modules

**9.1 Multi-file programs**
`import` currently does nothing. Make it load another `.qz` file.

**9.2 Visibility**
`pub` marks a declaration as visible outside its file. Already reserved.

**9.3 Global constants**
Currently top-level `let` belongs to the implicit `main` and functions
cannot see it. Allow top-level `const` to be genuinely global.

**9.4 Namespacing**
`math.sqrt` vs bare `sqrt`. Decide whether the stdlib becomes modules or
stays as builtins. **Recommendation:** keep builtins bare — "no imports
needed" is a stated design goal — and use modules only for user code.

---

### v1.0 — Polish

**10.1 Real test suite**
The examples are currently the tests. Add `_test.go` files with table
tests per stage, plus golden-file tests comparing `emit` output.

**10.2 Better parser errors**
"expected `)` to close the call opened on line 4" instead of
"expected `)`". Track opening-token positions.

**10.3 Error recovery quality**
Verify one syntax error does not produce a cascade of nonsense.

**10.4 Warnings**
Unused variables, unreachable code after `return`, unused functions.
Distinct from errors; do not block compilation.

**10.5 `--version`, `--help`**
Plus a build-time version stamp.

**10.6 `quartz fmt`**
A canonical formatter. The AST already exists; this is a pretty-printer.

**10.7 VS Code extension**
A TextMate grammar is about an hour of work for syntax highlighting.
Disproportionate morale value.

**10.8 Installer**
Inno Setup script: PATH entry, stdlib next to the binary, `.qz` file
association with an icon, and a right-click "Run with Quartz" verb.

**10.9 Bundle the Go toolchain**
Quartz currently requires Go to be installed. Either bundle it (~100 MB,
but it Just Works) or document the requirement prominently.

**10.10 Website and tutorial**
`README.md` is the seed. A "learn Quartz in 20 minutes" page matters
more than reference docs for adoption.

---

### Beyond 1.0

**11.1 C backend**
The prerequisite for everything low-level. Emit C instead of Go behind
the same AST. Design the IR boundary so it is one module swapped, not a
rewrite.

**11.2 Manual memory**
`own T`, `defer`, `alloc`/`free`. Requires the C backend. `own`,
`defer`, and `unsafe` are already reserved.

**11.3 Pointers and `unsafe` blocks**

**11.4 Generics**
`fn first<T>(xs: []T) -> ?T`. Large. Do not start before v1.0.

**11.5 Concurrency**
Go's goroutines and channels map almost directly. Cheap on the Go
backend, hard on the C backend — which is an argument for designing the
API now.

**11.6 GUI event handling**
The current `openWindow` opens a real window that is blank and inert.
Buttons, drawing, keyboard and mouse events need a callback design
first, which is a genuine language-design question rather than more
syscalls.

**11.7 Cross-platform GUI**
Linux and macOS equivalents. Note the user has neither and cannot test
them.

**11.8 More Win32**
Clipboard, tray icons, notifications, registry, file dialogs, process
control.

---

## Part 4 — Rules for whoever works on this next

**Verify before handing anything over.** Run all four:

```bash
gofmt -l .        # must print nothing
go vet ./...      # must be clean
go build -o quartz .
./quartz run examples/logic.qz && ./quartz run examples/demo.qz
```

**`quartz emit <file>` is the debugger.** When behaviour is wrong, read
the generated Go before theorising.

**Do not add Go module dependencies** without raising it explicitly.
Zero dependencies is a deliberate property.

**Surface design decisions, do not make them silently.** Numeric
promotion, map iteration order, struct value semantics, and the SQLite
dependency are all real forks. Present the trade-off.

**Do not claim something works if it was not verified.** GUI code cannot
be run in a Linux container — cross-compiling proves it type-checks and
nothing more. A previous session claimed rounded corners worked on a
Windows 10 machine where they cannot; that was a real bug caused by
ignoring a return value.

**Update `SYNTAX.md` in the same commit as the feature.** Documentation
written later is documentation not written.

**Keep the Known Limitations list honest.** It is the most useful
section in the docs and the first thing to go stale.
