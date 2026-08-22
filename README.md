# Veyl

Veyl is a programming language for people who want a real executable at
the end and do not want to fight the language to get one. You write
something that reads like Python. You get a single `.exe` with nothing
to install alongside it.

```veyl
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

No semicolons. No header files. No boilerplate `main`. No manual memory.
Math, strings, files, HTTP, JSON, time and native Windows calls are all
builtins, so most programs need no imports at all.

**Version 0.17.02.** The language is done enough for real work: a type
checker, lists and maps, structs with methods, nullable types, an error
type, modules, and a package manager. There is a formatter, a VS Code
extension and a Windows installer.

---

## Contents

- [Install](#install)
- [Commands](#commands)
- [A tour of the language](#a-tour-of-the-language)
- [Windows GUI](#windows-gui)
- [Two backends](#two-backends)
- [Building from source](#building-from-source)
- [Project layout](#project-layout)
- [Roadmap](#roadmap)
- [Known limitations](#known-limitations)

New here? [Learn Veyl in 20 minutes](docs/TUTORIAL.md).

Reference: [SYNTAX.md](docs/SYNTAX.md)
Packages: [PACKAGES.md](docs/PACKAGES.md)
Internals: [ARCHITECTURE.md](docs/ARCHITECTURE.md)

---

## Install

On Windows, run the installer from the
[releases page](https://github.com/owlspan/veyl/releases). It sets up
the compiler, adds it to PATH if you let it, associates `.vl` files, and
brings everything it needs with it. Then:

```
veyl doctor
```

That prints what it found and whether the install is sound.

If you already have Go, the installer leaves it completely alone. Its
own copy lives inside Veyl's folder and stays off PATH. Untick that
component during setup and Veyl will use yours instead.

---

## Commands

| Command | Effect |
| --- | --- |
| `veyl run f.vl` | compile and run |
| `veyl build f.vl` | write an executable next to the source |
| `veyl fmt f.vl` | reformat in place |
| `veyl emit f.vl` | print the generated Go |
| `veyl console` | interactive console |
| `veyl doctor` | check the install |
| `veyl init [name]` | start a project here |
| `veyl add <source>` | add a dependency and fetch it |
| `veyl install` | fetch everything the manifest lists |
| `veyl builtins` | list every builtin |
| `veyl version` | print the version |
| `veyl f.vl` | same as `run` |

Anything after the `.vl` file goes to your program, not to Veyl, so
`veyl run app.vl --verbose` reaches `os.args()`.

`emit` is the most useful debugging tool here. When a program does
something strange, read what it actually compiled to.

### Editors

Syntax files for VS Code, Notepad++, Sublime Text and Vim/Neovim ship in
`editors/`. The installer sets up whichever you tick. Ticking one for an
editor you do not have is harmless, since it only writes into that
editor's own config folder.

All of them except the VS Code grammar are generated from the compiler's
own keyword and builtin tables by `veyl editors`, so a new builtin is
highlighted everywhere as soon as that is re-run.

### Explorer

Double-clicking a `.vl` file asks whether to run it or build an `.exe`.
Right-click gives a **Veyl** submenu: compile, run, format, show the
generated Go. Right-clicking a folder offers a console there.

---

## A tour of the language

### Variables

```veyl
let count = 0            // mutable, type inferred
const limit = 10         // cannot be reassigned
let name: str = "Veyl"   // explicit type

count += 1
```

Reading an undeclared variable is an error, not an implicit declaration.

### Types

`int`, `float`, `str`, `bool`, `bytes` for raw binary, `[]T` lists,
`{K: V}` maps, `?T` for a value that might be missing, `T!` for one that
might have failed. Nothing converts implicitly. Use `str()`, `int()`,
`float()`.

```veyl
let nums = [3, 1, 2]
let ages = {"ada": 36}
let note: ?str = nil
```

### Structs

```veyl
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

### Failure

Anything that can fail returns `T!`, or `void!` when there is no value
to hand back. `?` either unwraps it or hands the failure up.

```veyl
fn wordCount(path: str) -> int! {
    let text = os.file.read(path)?
    return len(split(trim(text), " "))
}
```

That is the whole error-handling story. No exceptions, no panics in
normal code, and no way to ignore a failure by accident.

### Functions

```veyl
fn add(a: int, b: int) -> int {
    return a + b
}
```

Parameter types are required. Declaration order does not matter, so a
function can call one defined further down. A function with a return
type must return on every path and the compiler checks it.

Functions are values:

```veyl
let nums = [5, 3, 8, 1]
print(map(nums, fn(n: int) -> int { return n * n }))
print(filter(nums, fn(n: int) -> bool { return n > 4 }))
print(reduce(nums, 0, fn(a: int, b: int) -> int { return a + b }))
```

Closures capture by reference, so a closure sees later writes to what it
captured, and its own writes are visible outside.

### Strings

Any expression goes inside `{}`, including another string literal:

```veyl
let name = "world"
print("hello, {name}")
print("2 + 2 = {2 + 2}")
print("shouting: {upper("hi")}")
print("{{literal braces}}")
```

### Control flow

```veyl
if score >= 90 {
    print("A")
} else if score >= 80 {
    print("B")
}

while running { tick() }

for i in 0..5 { }            // 0 1 2 3 4
for i in 1..=5 { }           // 1 2 3 4 5
for i in 0..=100 step 25 { } // 0 25 50 75 100
for i in 10..0 step -2 { }   // 10 8 6 4 2
```

No parentheses around conditions. Braces always required.

### Multiple files

```veyl
import "geometry.vl"
```

`pub` decides what a file exports. A top-level `const` is a global; a
top-level `let` belongs to the program body.

### The library

No imports, ever. The dots only group names.

**Output** `print` `write`
**Input** `input` `pause`
**Convert** `str` `int` `float` `toInt` `toFloat` `isInt` `isFloat` `divf`
**Math** `sqrt` `cbrt` `pow` `exp` `hypot` `log` `log2` `log10` `abs`
`mod` `floor` `ceil` `round` `trunc` `clamp` `sign` `isNan` `min` `max`
`sin` `cos` `tan` `asin` `acos` `atan` `atan2`
**Strings** `len` `upper` `lower` `trim` `contains` `startsWith`
`endsWith` `indexOf` `count` `replace` `repeat` `charAt` `substr`
`padLeft` `padRight` `split` `chars` `lines`
**Lists** `push` `pop` `insert` `removeAt` `clear` `first` `last`
`slice` `reverse` `sort` `sum` `join` `map` `filter` `reduce` `sortBy`
`any` `all` `each`
**Maps** `has` `find` `remove` `keys` `values`
**Results** `must` `valueOr` `isOk` `failed` `errorOf` `fail`

And under a namespace: `os` for files and processes, `http`, `net`,
`json`, `time`, `re` for regular expressions, `hash`, `csv`, `zip`,
`bytes`, `rand`, `stats`, `term`, `bits`, `url`, `args`, `mem`, `task`,
and `win` for the Windows-only parts.

```veyl
let page = must(http.get("https://example.com"))
os.file.write("page.html", page)
print("saved at {time.stamp()}")
```

[SYNTAX.md](docs/SYNTAX.md) has every signature.

---

## Windows GUI

Windows-only builtins. Calling one while building for another OS is a
compile error, not a runtime crash.

```veyl
setTitle("My App")
hideConsole()
let rounded = openWindow("Hello from Veyl", 800, 500)
messageBox("Done", "Window closed.")
```

`openWindow` blocks until the user closes the window, because it runs
the Win32 message loop internally. Rounded corners need Windows 11
(build 22000 or later); on Windows 10 the OS refuses and the window
stays square, and `openWindow` returns `false` so you can tell.

These compile to `syscall` calls that load DLLs at runtime. No cgo, no C
compiler, no DLLs to ship.

---

## Two backends

Veyl has two ways of turning your source into an executable. They share
the lexer, the parser and the type checker, and differ only in the last
two stages.

**The Go backend** is the default and the definition of what Veyl means.
It emits Go source and hands it to the Go toolchain.

**The assembly backend** emits x86-64 directly, then assembles and links
it itself. No Go is involved in compiling with it and none is present in
what it produces.

```
                        +-->  Go source  -->  go build  -->  .exe
hello.vl  -->  frontend  |
                        +-->  x86-64  -->  encoder, linker  -->  .exe
```

The two are checked against each other byte for byte on every example.
The Go backend is the reference, so if they disagree, the assembly one
is wrong. Anything the assembly backend does not cover yet is a compile
error naming what is missing, never wrong output.

`collatz.vl`, nested loops, 10,000 iterations:

| | via Go | asm, linked by gcc | asm, self-linked |
| --- | ---: | ---: | ---: |
| runtime, best of 5 | 67 ms | 81 ms | 81 ms |
| executable size | 2,524,160 | 123,102 | **2,560** |

The size difference is the Go runtime, which is no longer there. The
speed difference is that every value still round-trips through a stack
slot, because there is no register allocator yet.

[asm-src/README.md](asm-src/README.md) covers what it does and does not
handle.

---

## Building from source

You need Go 1.21 or newer for the Go backend. The assembly backend needs
nothing installed once it is built.

```
git clone https://github.com/owlspan/veyl
cd veyl/src
go build -o veyl.exe ./compiler
```

Add that folder to your PATH to run it from anywhere. While working on
the compiler itself, skip the rebuild:

```
go run ./compiler run examples/demo.vl
```

The first `run` is Go's, the second is Veyl's.

### Tests

```
cd src      && go test ./...
cd frontend && go test ./...
cd asm-src  && go test ./...
```

The three modules are separate, so there is no single command that
covers everything.

Each Go-backend case is a `.vl` file next to a `.expected` file holding
what the compiler should produce: program output for `tests/ok`, error
messages for `tests/err`. Adding a test means adding two files, not
editing any Go.

After a deliberate change to output or wording, regenerate the
expectations and read the diff before committing:

```
go test -run TestVeyl -update ./compiler
```

The assembly backend has no expected-output files. Every example is run
through both backends and compared.

---

## Project layout

```
veyl/
  frontend/    lexer, parser, type checker. Shared.
  src/         the Go backend, the CLI, and all the tooling
  asm-src/     the assembly backend, its assembler and linker
  docs/        SYNTAX, TUTORIAL, ARCHITECTURE, PACKAGES
```

Three Go modules wired together with `replace`. Run `go` commands from
inside each one.

---

## Roadmap

| Version | Feature |
| --- | --- |
| v0.1 | expressions, variables, `if`, `while`, `print` |
| v0.2 | functions, resolver, builtins, Windows GUI |
| v0.3 | `for` loops, `break`/`continue`, math and strings |
| v0.4 | type checker, test suite |
| v0.5 | lists and maps |
| v0.6 | structs and `impl` |
| v0.7 | JSON, bitwise operators, `match` |
| v0.8 | nullable types `?T` |
| v0.9 | error type `T!` with `?` propagation |
| v0.10 | modules, `pub`, global constants |
| v0.15 | a Windows installer bundling its own Go toolchain |
| v0.16 | a package manager, `rand`, `stats`, `term`, `log` |
| v0.17 | namespaced imports, `bytes`, an interactive console |
| v0.17.02 | four editors, an Explorer menu, the Veyl rename |
| unreleased | the assembly backend |
| next | generics |

---

## Known limitations

An honest list.

- **Integer division truncates.** `7 / 2` is `3`. Use `divf(7, 2)` for
  `3.5`. This is a stated rule rather than a leaked backend detail, but
  it still catches people.
- **A missing map key is silent.** `m["absent"]` gives the zero value.
  `has()` and `find()` tell the difference. Putting a check on every
  read was not worth it.
- **No generics.** `Set` and `Queue` cannot be written in Veyl itself
  yet, which is why they are not in the library.
- **No global variables.** A top-level `const` is global; a top-level
  `let` belongs to the program body.
- **No databases.** SQLite needs a Go dependency, which would break the
  zero-dependency property.
- **Nil narrowing is syntactic.** `if x != nil` narrows inside the
  block, and `&&` chains work, but an early `return` does not narrow the
  rest of the function.
- **Garbage collected.** No manual memory, pointers or `unsafe`.
- **The formatter does not reflow lines.** `veyl fmt` fixes indentation
  and spacing. Where you break a line is your business.
- **Windows only for the installer and the GUI.** Elsewhere you build
  from source, and there is no windowing.
- **Windows are blank.** `openWindow` opens and manages a real window,
  but there is no drawing or event API yet.
- **Return checking is conservative.** A function whose only `return` is
  inside a loop is rejected, even when it is provably fine.
