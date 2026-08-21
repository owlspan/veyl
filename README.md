# Veyl

A small, fast, readable programming language that compiles to native
executables.

Veyl source (`.vl`) is translated to Go, which the Go toolchain
compiles to a single self-contained binary. You get native speed and a
program you can hand to someone with nothing to install - but you write
something closer to Python than to C++.

There is also a second, newer backend that emits x86-64 assembly
directly, with no Go anywhere in the pipeline. It covers a subset of
the language so far; the Go backend is the default and is what Veyl
means. See [the second backend](#the-second-backend).

```qz
fn isPrime(n: int) -> bool {
    if n < 2 { return false }
    for d in 2..=int(sqrt(float(n))) {
        if n % d == 0 { return false }
    }
    return true
}

for n in 2..50 {
    if isPrime(n) { write("{n} ") }
}
print("")
```

```
$ veyl run primes.vl
2 3 5 7 11 13 17 19 23 29 31 37 41 43 47
```

**Status:** v0.17.02. The language is feature-complete for everyday work:
a full type checker, lists and maps, structs with methods, nullable
types, an error type, modules, and libraries for files, HTTP,
networking, JSON, time, randomness, statistics and terminal output,
plus a package manager. Tooling is in place too - a formatter,
warnings, a VS Code extension, and a Windows installer that brings its
own Go toolchain. See [Roadmap](#roadmap).

---

## Contents

- [Why](#why)
- [Requirements](#requirements)
- [Building the compiler](#building-the-compiler)
- [Commands](#commands)
- [A tour of the language](#a-tour-of-the-language)
- [Windows GUI](#windows-gui)
- [Cross-compiling](#cross-compiling)
- [Project layout](#project-layout)
- [How the compiler works](#how-the-compiler-works)
- [Roadmap](#roadmap)
- [Known limitations](#known-limitations)

**New here? [Learn Veyl in 20 minutes](docs/TUTORIAL.md).**

Full language reference: **[SYNTAX.md](docs/SYNTAX.md)**
Using and writing packages: **[PACKAGES.md](docs/PACKAGES.md)**
Compiler internals: **[ARCHITECTURE.md](docs/ARCHITECTURE.md)**

---

## Why

Three things at once, which most languages make you pick two of:

- **Native performance.** No interpreter, no VM, no runtime to install.
- **Low ceremony.** No semicolons, no header files, no boilerplate
  `main`, no manual memory management.
- **Batteries included.** Math, strings, input, files, HTTP, JSON,
  randomness, statistics, colour, and native Windows GUI calls are all
  builtins - no imports and nothing to install before you can use them.
  A package manager exists for everything else, but you should be able
  to write a useful program without touching it.

A Veyl program that opens a native window is one `.exe` with no DLLs
to ship and no C compiler involved anywhere.

---

## Installing

**The easy way, on Windows:** run `veyl-<version>-setup.exe`. It
installs the compiler, adds it to PATH if you let it, associates `.vl`
files, and ships a private copy of the Go toolchain so there is nothing
else to install. Then check it:

```
veyl doctor
```

The bundled toolchain lives inside Veyl's own folder and is kept off
PATH deliberately, so if you already have Go it is left completely
alone. Untick that component during setup and Veyl will use yours.

**Building it yourself** needs **Go 1.21 or newer** -
<https://go.dev/dl/>. Go is required both to build the compiler and to
compile Veyl programs, since Veyl hands its generated code to the
Go toolchain.

If Veyl cannot find a Go toolchain it says so and tells you what to
do, rather than failing with a PATH error. `VEYL_GO` forces a
specific one.

---

## Building the compiler

```
git clone <your-repo>   # or just cd into the folder
cd veyl/src
go build -o veyl.exe ./compiler
```

That produces `veyl.exe` in the project folder. To run it from
anywhere, add that folder to your PATH:

```
setx PATH "%PATH%;C:\path\to\veyl"
```

Open a new terminal afterwards for the change to take effect.

While working on the compiler itself, skip the rebuild step:

```
go run ./compiler run examples\demo.vl
```

The first `run` is Go's, the second is Veyl's.

## Running the tests

```
go test ./...
```

Each case is a `.vl` file next to a `.expected` file holding what the
compiler should produce - program output for `tests/ok`, error messages
for `tests/err`. Adding a test means adding two files, not editing any
Go code.

After an intentional change to output or error wording, regenerate the
expectations and **read the diff before committing it**:

```
go test -run TestVeyl -update ./compiler
```

---

## Commands

| Command                  | Effect                                        |
| ------------------------ | --------------------------------------------- |
| `veyl run f.vl`        | compile and run                                |
| `veyl build f.vl`      | write an executable next to the source         |
| `veyl fmt f.vl`        | reformat the file in place                     |
| `veyl emit f.vl`       | print the generated Go                         |
| `veyl tokens f.vl`     | print the token stream                         |
| `veyl console`         | an interactive console                         |
| `veyl open f.vl`       | ask whether to run it or build an .exe         |
| `veyl doctor`          | check the install and report which Go it found |
| `veyl init [name]`     | start a project here                           |
| `veyl add <source>`    | add a dependency and fetch it                  |
| `veyl install`         | fetch everything the manifest lists            |
| `veyl packages`        | list dependencies                              |
| `veyl builtins`        | list every builtin, for editor tooling         |
| `veyl editors [dir]`   | regenerate the editor syntax files             |
| `veyl version`         | print the version                              |
| `veyl f.vl`            | same as `run`                                  |

Anything after the `.vl` file is passed to the program, not to Veyl:
`veyl run app.vl --verbose` reaches `os.args()`.

`emit` is the single most useful debugging tool in the project. When a
program behaves strangely, look at what it actually compiled to.

### Editors

Syntax highlighting for **VS Code**, **Notepad++**, **Sublime Text** and
**Vim/Neovim**. The installer sets up whichever you tick; ticking one
for an editor you do not have is harmless, since it only writes into
that editor's own config folder.

To install by hand, copy from `editors/` into:

| Editor | Destination |
| --- | --- |
| VS Code | `%USERPROFILE%\.vscode\extensions\veyl-lang` |
| Notepad++ | `%APPDATA%\Notepad++\userDefineLangs` |
| Sublime Text | `%APPDATA%\Sublime Text\Packages\User` |
| Vim | `%USERPROFILE%\Documents\vimfiles\syntax` and `\ftdetect` |
| Neovim | `%LOCALAPPDATA%\nvim\syntax` and `\ftdetect` |

Restart the editor afterwards.

All of them except the VS Code grammar are **generated from the
compiler's own keyword and builtin tables** by `veyl editors`, so a
builtin added to the library is highlighted everywhere as soon as that
is re-run. Three hundred builtins transcribed by hand into four file
formats would be wrong within a week.

### Explorer

**On Windows:** double-clicking a `.vl` file asks what you want - run
it, or build a standalone `.exe` you can hand to someone. The window
stays open either way. Right-click gives both directly, without the
menu.

Right-click gives a **Veyl** submenu: Compile to .exe, Run, Format,
and Show the generated Go. Right-clicking a folder offers **Open Veyl
console here**.

Double-clicking a `.exe` you built is a different matter: it opens a
console, runs, and closes instantly. That's normal for a console
program. Run it from a terminal, or end the program with `pause()`.

---

## A tour of the language

### Variables

```qz
let count = 0            // mutable, type inferred
const limit = 10         // cannot be reassigned
let name: str = "Veyl" // explicit type

count += 1
```

Reading an undeclared variable is an error, not an implicit
declaration. Assigning to a `const` is a compile error.

### Types

`int`, `float`, `str`, `bool`, `bytes` for raw binary, `[]T` lists,
`{K: V}` maps, `?T` for a value that might be missing, and `T!` for one
that might have failed.
No implicit conversion - use `str()`, `int()`, `float()`.

```qz
let nums  = [3, 1, 2]
let ages  = {"ada": 36}
let note: ?str = nil
```

### Structs

```qz
struct Vec {
    x: float
    y: float
}

impl Vec {
    fn length(self) -> float {
        return sqrt(self.x * self.x + self.y * self.y)
    }
}

print(Vec{x: 3.0, y: 4.0}.length())     // 5
```

### Handling failure

Anything that can fail returns `T!`, or `void!` when there is no value
to hand back - writing a file either worked or has a reason it did not. `?` unwraps it or hands the
failure up, which is Go's four-line error check in one character.

```qz
fn wordCount(path: str) -> int! {
    let text = os.file.read(path)?
    return len(split(trim(text), " "))
}
```

### Multiple files

```qz
import "geometry.vl"
```

`pub` decides what a file exports. A top-level `const` is a global; a
top-level `let` belongs to the program body.

### Strings

Any expression goes inside `{}`, including other string literals:

```qz
let name = "world"
print("hello, {name}")
print("2 + 2 = {2 + 2}")
print("shouting: {upper("hi")}")
print("{{literal braces}}")
```

### Control flow

```qz
if score >= 90 {
    print("A")
} else if score >= 80 {
    print("B")
} else {
    print("C")
}

while running {
    tick()
}

for i in 0..5 { }            // 0 1 2 3 4
for i in 1..=5 { }           // 1 2 3 4 5
for i in 0..=100 step 25 { } // 0 25 50 75 100
for i in 10..0 step -2 { }   // 10 8 6 4 2

for i in 0..100 {
    if i % 2 == 0 { continue }
    if i > 20 { break }
}
```

No parentheses around conditions. Braces always required.

### Functions

```qz
fn add(a: int, b: int) -> int {
    return a + b
}

fn greet(name: str) {
    print("hello, {name}")
}
```

Parameter types are required. Declaration order doesn't matter - a
function can call one defined later in the file. Recursion works. A
function with a return type must return on every path, and the compiler
checks it.

### Builtins

No imports needed, ever.

**Output** `print` `write`
**Input** `input` `pause`
**Convert** `str` `int` `float` `toInt` `toFloat` `isInt` `isFloat` `divf`
**Math** `sqrt` `cbrt` `pow` `exp` `hypot` `log` `log2` `log10` `abs`
`mod` `floor` `ceil` `round` `trunc` `clamp` `sign` `isNan` `min` `max`
`sin` `cos` `tan` `asin` `acos` `atan` `atan2`
**Constants** `PI` `E` `INF` `NAN`
**Random** `random` `randomInt`
**Strings** `len` `upper` `lower` `trim` `contains` `startsWith`
`endsWith` `indexOf` `count` `replace` `repeat` `charAt` `substr`
`padLeft` `padRight`
**System** `sleep` `exit`
**Lists** `push` `pop` `insert` `removeAt` `clear` `first` `last`
`slice` `reverse` `sort` `sum` `join` `contains` `indexOf`
**Maps** `has` `find` `remove` `keys` `values`
**Splitting** `split` `chars` `lines`
**Results** `must` `valueOr` `isOk` `failed` `errorOf` `fail`

Every numeric builtin accepts `int` or `float`.

The rest of the library lives under a dotted path - still no imports,
the dots only group the names:

**`os`** files, directories, paths, environment, processes
**`http`** get, post, download, URL escaping
**`net`** DNS, TCP connection tests, a concurrent port scanner
**`json`** encode, decode, and reading one field by path
**`time`** clocks and readable date formats
**`mem`** allocation and collector statistics

```qz
let page = must(http.get("https://example.com"))
os.file.write("page.html", page)
print("saved at {time.stamp()}")
```

See [SYNTAX.md](docs/SYNTAX.md) for signatures and details.

---

## Windows GUI

Windows-only builtins. Using one while building for another OS is a
compile error, not a runtime crash.

| Function                           | Description                        |
| ---------------------------------- | ---------------------------------- |
| `setTitle(s)`                      | console window title                |
| `beep(freq, ms)`                   | tone at a frequency                 |
| `messageBox(title, text)`          | native dialog                       |
| `hideConsole()`                    | hide the console window             |
| `winBuild()`                       | Windows build number                |
| `isWin11()`                        | whether build >= 22000              |
| `openWindow(title, w, h)`          | real window; returns whether corners rounded |
| `openWindow(title, w, h, rounded)` | same, rounding on or off            |

```qz
setTitle("My App")
hideConsole()
let rounded = openWindow("Hello from Veyl", 800, 500)
messageBox("Done", "Window closed.")
```

`openWindow` **blocks** until the user closes the window - it runs the
Win32 message loop internally.

**Rounded corners need Windows 11** (build 22000+). On Windows 10 the
OS refuses the request and the window stays square; `openWindow` returns
`false` so your program can tell.

These compile to `syscall` calls that load DLLs at runtime, so there is
no cgo, no C compiler, and no DLLs to ship.

---

## Cross-compiling

```
VEYL_TARGET=windows veyl build app.vl
```

On Windows cmd:

```
set VEYL_TARGET=windows
veyl build app.vl
```

`veyl run` needs the target to match your machine. Use `build`
otherwise.

---

## Project layout

The repository root holds one folder, `src/`, plus an untracked
`Working version/` holding the last verified build. Inside `src/`, the
compiler is one Go package in `compiler/`; everything else is grouped
by what it is for.

```
veyl/
  Working version/  untracked snapshot of the last good build
  src/
    compiler/       the compiler, one package main
      token.go        token kinds, keywords, the Token type
      lexer.go        source text  -> tokens
      ast.go          node type definitions
      parser.go       tokens       -> AST
      resolve.go      AST          -> checked AST (names, scopes, arity)
      types.go        the Type value and type-annotation parsing
      check.go        AST          -> typed AST (the type checker)
      codegen.go      AST          -> Go source
      stdlib.go       math and string builtins
      lib_*.go        the rest of the standard library
      runtime_win.go  Win32 builtins and helpers
      veyl.go         CLI driver
      toolchain.go    finding the Go toolchain, and 'veyl doctor'
      veyl_test.go    the test harness
    docs/           SYNTAX, TUTORIAL, ARCHITECTURE, PACKAGES
    examples/       sample programs
    tests/ok/       programs that must compile and run
    tests/err/      programs that must be rejected
    editors/        syntax files for four editors
    installer/      Inno Setup script and its build script
    icons/          the logo, generated by tools/mkicon
    tools/mkicon/   draws the logo and the Windows resource object
    scripts/        release.ps1 and the double-click helpers
    go.mod          the module root; run go commands from here
```

The compiler has grown past the file list above; `token.go`, `lexer.go`,
`parser.go`, `ast.go`, `resolve.go`, `types.go`, `check.go`,
`codegen.go`, `format.go` and `veyl.go` are the pipeline, and the
`lib_*.go` and `collections.go` files are the standard library.

---

## How the compiler works

```
hello.vl
   |
   |  lexer.go        text into tokens, each tagged with line and column
   v
 tokens
   |
   |  parser.go       recursive descent, Pratt expressions
   v
  AST
   |
   |  resolve.go      scopes, undefined names, arity, return paths
   v
checked AST
   |
   |  check.go        a type for every expression, in Veyl's vocabulary
   v
 typed AST
   |
   |  codegen.go      emits Go, with //line directives
   v
 main.go   -->   go build   -->   hello.exe
```

The generated Go is written to a temporary directory, compiled, and the
resulting binary is moved next to your source. The temp directory is
deleted afterwards.

**`//line` directives** are what make errors readable. Every emitted
statement is preceded by a comment pointing at the original `.vl` line,
so when the Go backend reports a type error it names your file and your
line number, not generated code you never wrote.

That describes the Go backend, which is the default. The assembly
backend replaces these last two stages with a lowering pass and an
emitter, keeping the lexer, parser and type checker unchanged; see
[the second backend](#the-second-backend).

More detail in [ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## Roadmap

| Version | Feature                                            |
| ------- | -------------------------------------------------- |
| v0.1    | expressions, variables, `if`, `while`, `print`      |
| v0.2    | functions, resolver, builtins, Windows GUI          |
| v0.3    | `for` loops, `break`/`continue`, math and strings   |
| v0.4    | type checker, test suite                            |
| v0.5    | lists and maps                                      |
| v0.5.1  | dotted libraries: `os`, `http`, `net`, `time`, `mem`|
| v0.6    | structs and `impl`                                  |
| v0.7    | JSON, bitwise operators, `match`                    |
| v0.8    | nullable types `?T`                                 |
| v0.9    | error type `T!` with `?` propagation                |
| v0.9.1  | the whole library reports failure through `T!`      |
| v0.10   | modules, `pub`, global constants                    |
| v1.0    | a formatter, warnings, a VS Code grammar            |
| v0.15   | a Windows installer bundling its own Go toolchain   |
| **v0.16**| **a package manager, `rand`/`stats`/`term`/`log`** |
| v0.17   | namespaced imports, tracebacks, `url`/`zip`/`args`/`bits` |
| v0.17   | a `bytes` type, an interactive console, `void!`     |
| **v0.17.02** | **four editors, an Explorer menu, the Veyl rename** |
| unreleased | a second backend: Veyl to x86-64, no Go in the pipeline |
| next    | generics                                            |

**The type checker was the bottleneck for everything below it**, and it
is now in place. Lists, maps, structs, JSON and modules all need the
compiler to know the type of an expression, and it does: `check.go`
walks the tree between resolve and codegen and returns a type for every
node.

### The second backend

There is now an **assembly backend** in `asm-src/`, sharing the whole
front end and replacing only the last two stages: it lowers to a
three-address IR and emits x86-64 rather than Go source. No Go is
involved in compiling with it, and none is present in what it produces.

It covers a subset of the language and is checked against the Go
backend byte for byte - the Go backend is the definition of what Veyl
means, so if the two disagree, the assembly one is wrong. Anything
outside the subset is a compile error naming what is missing, never
wrong output.

On `collatz.vl`, nested loops, 10,000 iterations:

| | via Go | via assembly |
| --- | ---: | ---: |
| runtime, best of 5 | 67 ms | 81 ms |
| executable size | 2,524,160 bytes | 123,027 bytes |

About 20% slower and 20 times smaller. The size difference is the Go
runtime, which is what is no longer there; the speed difference is that
every value still round-trips through a stack slot, because there is no
register allocator yet.

See [asm-src/README.md](asm-src/README.md) for what it covers and what
it does not. This is what replaces the C backend that earlier versions
of this file promised: the goal was manual memory and a binary with no
runtime in it, and a backend that emits machine code reaches that
without a second C toolchain in the way.

The other structural piece still missing is generics, without which
containers like `Set` and `Queue` cannot be written in Veyl itself.

---

## Known limitations

Honest list of what v0.17.02 does not do.

- **Integer division truncates.** `7 / 2` is `3`. This is now a stated
  language rule rather than a leaked backend detail, but it still
  surprises people. Use `divf(7, 2)` for `3.5`.
- **A missing map key is silent.** `m["absent"]` gives the zero value.
  `has()` and `find()` tell the difference; the bare index was left
  alone rather than putting a nil check on every read.
- **No generics.** `Set`, `Queue` and friends cannot be written in
  Veyl itself yet, which is why they are not in the library.
- **No global *variables*.** A top-level `const` is global, but a
  top-level `let` belongs to the program body.
- **No databases.** SQLite needs a Go dependency, which would break the
  zero-dependency property, so it has not been added.
- **Nil narrowing is syntactic.** `if x != nil` narrows inside the
  block, and `&&` chains work, but an early `return` does not narrow
  the rest of the function.
- **Garbage collected.** Manual memory, pointers, and `unsafe` are not
  available. Programs built through the Go backend inherit Go's
  collector; the assembly backend has none at all yet and frees
  nothing, which is fine for a program that runs and exits and not for
  one that loops.
- **The formatter does not reflow lines.** `veyl fmt` fixes
  indentation and spacing; where you break a line is left to you.
- **The installer is Windows only.** `installer\veyl.iss` builds a
  Windows setup program. Linux and macOS have no packaging at all; you
  build from source there.
- **Windows only for GUI.** No Linux or macOS equivalent yet.
- **Windows are blank.** `openWindow` opens and manages a real window,
  but there is no drawing or event API - no buttons, no input handling,
  no canvas.
- **Conservative return checking.** A function whose only `return` is
  inside a loop is rejected, even when it is provably fine.
