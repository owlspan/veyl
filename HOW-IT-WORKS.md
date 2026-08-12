# How Veyl works

Written to be read start to finish, so that you can explain your own
compiler to somebody else. Every section ends with **the one-liner**:
the sentence to say if you get asked about it and have ten seconds.

---

## Contents

1. [The thirty second version](#1-the-thirty-second-version)
2. [What happens when you run a program](#2-what-happens-when-you-run-a-program)
3. [Stage 1: the lexer](#3-stage-1-the-lexer)
4. [Stage 2: the parser](#4-stage-2-the-parser)
5. [Stage 3: the resolver](#5-stage-3-the-resolver)
6. [Stage 4: the type checker](#6-stage-4-the-type-checker)
7. [Stage 5: codegen](#7-stage-5-codegen)
8. [Stage 6: the Go toolchain](#8-stage-6-the-go-toolchain)
9. [Why compile to Go](#9-why-compile-to-go)
10. [Error handling: T! and ?](#10-error-handling-t-and-)
11. [The type system](#11-the-type-system)
12. [The package manager](#12-the-package-manager)
13. [The installer](#13-the-installer)
14. [Questions you will be asked](#14-questions-you-will-be-asked)

---

## 1. The thirty second version

Veyl is a compiled programming language. You write `.vy` files with
syntax close to Python, and you get a native `.exe` you can hand to
somebody who has nothing installed.

The compiler is written in Go. It reads your `.vy` file, checks it
thoroughly, and **emits Go source code**, which the Go toolchain turns
into the binary. Go is the intermediate step, not something your
finished program needs.

**The one-liner:** *"Veyl compiles to a native executable. My compiler
emits Go source as an intermediate representation, the same way C++
originally emitted C."*

---

## 2. What happens when you run a program

```
veyl run hello.vy
```

Six stages, in order. Each takes the output of the one before it.

```
hello.vy
   |
   |  1. LEX        text  ->  tokens
   |  2. PARSE      tokens  ->  a tree
   |  3. RESOLVE    do these names exist?
   |  4. CHECK      do the types make sense?
   |  5. CODEGEN    tree  ->  Go source
   |  6. BUILD      Go source  ->  hello.exe
   v
Hello world
```

| Stage | File | Question it answers |
| --- | --- | --- |
| Lex | `lexer.go` | What are the words? |
| Parse | `parser.go` | What is the structure? |
| Resolve | `resolve.go` | Do these names exist? |
| Check | `check.go` | Do the types make sense? |
| Codegen | `codegen.go` | What Go says the same thing? |
| Build | `veyl.go` | Hand it to the Go toolchain |

**Two rules the whole compiler obeys.**

**Every stage gates the next.** If lexing produces errors, parsing never
runs. There is no point telling you about a type error in a line the
compiler could not even read.

**Errors accumulate, they never abort.** A stage collects a list of
errors and keeps going, so you see all ten mistakes at once instead of
recompiling ten times. This is why every stage has an `Errors []string`
rather than returning on the first problem.

**The one-liner:** *"Six stages, each gating the next, and every stage
collects all its errors instead of stopping at the first."*

---

## 3. Stage 1: the lexer

**Text goes in. Tokens come out.** A token is one meaningful unit.

```
let x = 2 + 2
```

becomes

```
LET  IDENT("x")  ASSIGN  NUMBER(2)  PLUS  NUMBER(2)
```

The lexer does not care whether this makes sense. It only splits the
text into words and punctuation and labels each one.

**Every token remembers its line and column.** That is not decoration.
It is what lets an error say `hello.vy:3:15` instead of "something is
wrong somewhere", and it feeds the `//line` directives in stage 5.

### Three details worth knowing

**Numbers versus ranges.** `1..10` must lex as `1`, `..`, `10`, but
`1.5` is a single number. The rule: only consume a `.` as a decimal
point if a **digit follows it**.

**String interpolation nests.** `"{upper("hi")}"` works, because the
lexer tracks how deep it is inside braces and only ends the string on a
quote at depth zero.

**A byte order mark is skipped.** Notepad writes an invisible three byte
marker at the start of files it saves. Without skipping it, every file
saved in Notepad failed with three errors about characters you cannot
see.

**The one-liner:** *"The lexer turns text into tokens, and every token
carries its line and column so errors can point at real source."*

---

## 4. Stage 2: the parser

**Tokens go in. A tree comes out.** The tree is called an AST, an
abstract syntax tree, and it captures structure that a flat list of
tokens cannot.

The classic example is precedence. These are the same tokens:

```
2 + 3 * 4
```

but only one tree is correct:

```
      +
     / \
    2   *
       / \
      3   4
```

The parser builds that shape, so `*` binds tighter than `+`. The
technique is **recursive descent** with **Pratt parsing** for
expressions, which is the standard way to handle operator precedence
without writing a separate function per precedence level.

**Forward progress is guaranteed.** Every loop over statements checks
whether the position moved and forces it forward if not. Without that,
one malformed input could hang the compiler in an infinite loop.

**The one-liner:** *"The parser turns tokens into a tree, which is what
encodes precedence: `2 + 3 * 4` has one correct shape and the parser
builds it."*

---

## 5. Stage 3: the resolver

**Names.** The resolver walks the tree and answers: does this name
exist, and are you allowed to use it here?

It catches:

- using a variable that was never declared
- assigning to a `const`
- calling a function with the wrong number of arguments
- `break` outside a loop
- using something private from another file
- two functions with the same name

It also records **whether each variable is ever read**, which matters
in stage 5: Go refuses to compile a program with an unused local
variable, but `let x = 5` with no use is perfectly legal Veyl. The
resolver marks it, and codegen emits a discard so Go stays happy.

**The one-liner:** *"The resolver answers 'does this name exist and can
you use it here', and tracks which variables are actually read."*

---

## 6. Stage 4: the type checker

**The biggest piece of the compiler, and the one that unblocked
everything else.**

It walks the tree and works out **the type of every single
expression**, then checks the types fit together.

```
let a = 1        // a is int
let b = 2.5      // b is float
let c = a + b    // error: cannot mix int and float in '+'
```

### Why it had to exist before anything else

```
let xs = []
```

Codegen must write a Go type. Is that `[]int`? `[]string`? Nothing in
the tokens or the tree says. **Something has to work it out before code
can be generated at all.** That is why lists, maps, structs and JSON
were all blocked until the type checker existed. It was not a
nice-to-have, it was the bottleneck.

### What else breaks without it

**Errors would speak Go instead of Veyl.** A type mistake would still
get caught, but by the Go compiler, using Go's words, pointing at
generated code. The user would see `cannot use x (variable of type
string) as float64`. They wrote `str` and `float` and never asked for
Go.

**`7 / 2` would not know what to do.** Integer division and float
division are different machine operations. Choosing needs to know both
operands are `int`.

### It records as well as checks

The checker writes the type it worked out **onto the tree**. Codegen
then emits explicit Go types rather than relying on Go's inference, so
Go's inference can never quietly disagree with Veyl's.

**The one-liner:** *"The type checker works out the type of every
expression and records it on the tree. Without it `let xs = []` cannot
be compiled at all, because codegen would not know what type to write."*

---

## 7. Stage 5: codegen

**Tree goes in. Go source comes out.** A tree walk, emitting text.

Two decisions worth defending:

**Every binary expression is fully parenthesised.** The output is ugly:
`((a + b) * c)`. But the parser already encoded precedence in the tree
shape, so parenthesising everything means **Go's precedence rules can
never silently disagree with Veyl's**. Ugly output, zero bugs.

**`//line` directives are emitted before every statement.** These tell
the Go compiler "this next line really came from `hello.vy` line 12".
That single trick is why:

- a Go backend error points at your `.vy` file
- a runtime crash prints a Veyl traceback naming your lines

```
error: divided by zero
  at boom.vy:2
  at boom.vy:6
```

**The one-liner:** *"Codegen walks the tree and writes Go. It fully
parenthesises everything so Go's precedence can never disagree with
mine, and emits `//line` directives so errors point back at `.vy`
files."*

---

## 8. Stage 6: the Go toolchain

The driver writes the generated Go into a temporary folder with a
minimal `go.mod`, runs `go build`, and then runs the resulting
executable.

**Finding Go** is `findGo` in `toolchain.go`, which looks in three
places in order:

1. `$VEYL_GO`, to force a specific one
2. `go\bin\go.exe` **next to `veyl.exe`**, the installer's private copy
3. `PATH`

The bundled copy beats `PATH` on purpose, so an installed Veyl keeps
working no matter what else the machine picks up later. It is kept
**off** `PATH` on purpose, so a developer's own Go is never shadowed.

If none is found, the error explains the problem in Veyl's terms rather
than failing with a Windows `%PATH%` message.

**The one-liner:** *"The driver writes the Go into a temp folder and
shells out to the toolchain. It looks for Go in three places and the
bundled copy wins, so an install keeps working regardless of the
machine."*

---

## 9. Why compile to Go

This is the question you will definitely be asked. Answer it
confidently, because the technique is completely normal.

**Prior art:** C++ began as Cfront, which emitted C. Nim, Vala and Haxe
emit C. TypeScript emits JavaScript. Go's own first compiler was
written in C.

**What it buys:**

- a mature optimiser and linker, for free
- cross-compilation to Windows, Linux and macOS, for free
- garbage collection, for free
- you write a frontend instead of a register allocator

**What it costs, and say this before they ask:**

- a garbage collector you cannot remove
- a runtime, so about 2 MB minimum binary size
- no manual memory, no pointers, no `unsafe`
- Go must be present to compile (solved by bundling it)

**Veyl is not a low level language.** It is garbage collected. If you
call it low level, the next question is "so how does memory management
work" and the honest answer contradicts you. Say **native speed**
instead, which is true.

The C backend is the planned fix, and it is what would buy back manual
memory. Say it as a plan, not as a thing that exists.

**The one-liner:** *"Compiling through Go is a normal technique, C++
did the same thing. It costs me a garbage collector and a 2 MB floor,
and it buys a mature optimiser and free cross-compilation. The C
backend is what would buy the low level control back."*

---

## 10. Error handling: T! and ?

**Veyl has no exceptions.** No `try`, no `catch`, no `throw`.

Anything that can fail returns `T!`: either a `T`, or a reason it
failed.

```
fn wordCount(path: str) -> int! {
    let text = os.file.read(path)?
    return len(split(trim(text), " "))
}
```

The `?` means: unwrap this, or if it failed, stop and hand the failure
to my caller. In Go that is four lines of `if err != nil`. Here it is
one character.

| | |
| --- | --- |
| `x?` | unwrap, or pass the failure up |
| `must(x)` | unwrap, or crash deliberately |
| `valueOr(x, d)` | unwrap, or use a default |
| `isOk(x)` / `errorOf(x)` | ask, and read the reason |

### Why not exceptions

**You cannot tell by looking at a function whether it throws.**
`readFile(path)` in Python gives no hint, so people forget, and the
program dies far from the mistake. With `T!` the failure is **in the
type**, the compiler will not let you use the value without dealing
with it, and there is no invisible jump up five stack frames. Rust and
Go both made this choice.

### void!

For an action that can fail but returns nothing, like writing a file.

`os.file.write` used to return `bool`. So "permission denied", "no such
folder" and "disk full" were all just `false`. The information existed,
the operating system told us, and the API threw it away.

```
let r = os.file.write("/nowhere/a.txt", "x")
print(errorOf(r))   // open /nowhere/a.txt: The system cannot find the path
```

**The one-liner:** *"`bool` tells you that it failed. `void!` tells you
why."*

---

## 11. The type system

`int`, `float`, `str`, `bool`, `bytes`, `[]T` lists, `{K: V}` maps,
`?T` nullable, `T!` fallible, `fn(A) -> B` functions, and `struct`.

**No implicit conversion.** `1 + 2.5` is an error. Use `float(1)`. The
one exception is a plain integer *literal*, which becomes a float where
one is expected, following Go's untyped constant rule, so `radius * 2`
works.

**`?T` is how a value can be missing.** A plain `T` can never be nil.
Inside `if x != nil { ... }` the compiler narrows `x` to a plain `T`.

**`bytes` is raw binary.** Worth knowing why it exists, because the
obvious explanation is wrong. Converting binary to a `str` and back is
lossless. What breaks is every operation that assumes text. Measured on
four bytes that are not valid UTF-8:

| Operation | Result |
| --- | --- |
| `len`, `trim` | unchanged |
| `upper` | corrupted |
| `json.encode` | every invalid byte becomes a replacement character |

The data survives right up until something touches it, then it is
quietly wrong. A separate type makes that impossible rather than
unlikely.

**The one-liner:** *"Types are checked before any code is generated,
there are no implicit conversions, and the error messages use Veyl's
words, never Go's."*

---

## 12. The package manager

`veyl init`, `veyl add`, `veyl install`, `veyl packages`.

Three decisions, each with a cost:

**No central registry.** A package is a GitHub repository at a tag. A
registry is a service somebody has to run forever. *Cost: no search.*

**Tarballs over HTTPS, not git.** Go's standard library can fetch and
unpack them, so this costs no dependency and nobody needs git
installed. *Cost: no commit hash pinning.*

**Exact versions, no resolver.** If two dependencies want different
versions of the same package you get an error, not a guess. *Cost:
you fix it by hand. That is the point.*

**`veyl.lock` stores a SHA-256** over every filename and byte, because
**a git tag can be moved** after people depend on it. That is the one
attack it defends against, and the documentation says exactly that
rather than implying more.

**Imports are namespaced.** Two packages can both export `hello`;
you write `greet.hello()` and `loud.hello()`. Inside a package a bare
name means that package's own function, so a library can call its own
helpers and cannot reach into the program that imported it.

**The one-liner:** *"Packages are GitHub repos at a tag, fetched as
tarballs so there is no dependency on git, with a lock file that
notices if a tag is moved underneath you."*

---

## 13. The installer

A Windows installer that bundles a **trimmed copy of the Go toolchain**,
about 39 MB.

**Why bundle it:** someone learning to program should not have to
install a second toolchain and get the version right. The failure it
prevents is a beginner installing the wrong Go and getting an error
they cannot act on.

**What it costs:** the bundled Go goes stale between releases, and
39 MB per download. Both are mitigated by the toolchain being an
**optional component**: anyone with their own Go unticks it.

**Better than PyInstaller,** and say so. PyInstaller bundles the
interpreter into *every program you ship*. Veyl bundles the toolchain
**once, at install time**, and the programs it produces contain no Go
at all. The developer pays 39 MB once; their users never pay it.

---

## 14. Questions you will be asked

Practise these out loud.

**"Is this a real programming language or a wrapper?"**
It is a real compiler: lexer, parser, resolver, type checker, code
generator. Go source is my intermediate representation, the same way
C++ started by emitting C. Nothing of Veyl exists when the program
runs.

**"Why not just use Python?"**
Python is interpreted, so it is slower, and shipping it means the user
needs Python installed. Veyl produces one native file with nothing to
install.

**"Why not just use Go?"**
Go is what I compile to. Veyl is a smaller, simpler surface aimed at
people who find Go's ceremony heavy, with a batteries included library
and native Windows GUI built in.

**"What was the hardest part?"**
The type checker, because everything else depended on it. `let xs = []`
cannot be compiled without knowing the element type, so lists, maps,
structs and JSON were all blocked until it existed.

**"What would you do differently?"**
Generics earlier. Without them, containers like `Set` and `Queue`
cannot be written in Veyl itself, which is why they are not in the
library yet.

**"What is it missing?"**
Generics, a C backend for manual memory and pointers, a language server
for editor autocomplete, and databases. All known, all on the roadmap,
none of them pretended to be done.

**"What are you most proud of?"**
Pick a real one and be specific. The `//line` directives are a good
answer: three lines of code, and they are why a runtime crash prints a
traceback naming your `.vy` lines instead of a Go stack trace.

---

## The shortest honest summary

> Veyl is a compiled programming language with a six stage compiler:
> lexer, parser, resolver, type checker, code generator, and a Go
> backend. Programs are native executables with nothing to install.
> It has a real type system with nullable and fallible types, a package
> manager, a formatter, an interactive console, and a Windows installer
> that bundles its own toolchain. It is garbage collected, and a C
> backend for manual memory is the next major step.
