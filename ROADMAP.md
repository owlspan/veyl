# Veyl roadmap

Where Veyl actually is, measured against what a serious language needs.
Checked against the source rather than remembered, so a tick means the
thing exists and has tests, not that it is planned.

**Current version: 0.17.**

---

## The honest summary

**Level 1, basic language: done.**
**Level 2, real programming language: done except generics.**
**Level 3, systems-capable: not started. Blocked on the C backend.**
**Level 4, production tooling: about half.**
**Level 5, C++ class: not started, and correctly so.**

The single most important sentence about Veyl's position: **it is a
garbage collected language that compiles through Go.** Everything in
Level 3 follows from removing that, which is why the C backend is the
one item that unblocks a whole tier rather than adding a feature.

---

## Level by level

| Level | State |
| --- | --- |
| 1. Basic language | complete |
| 2. Real language | complete except generics |
| 3. Systems-capable | not started |
| 4. Production tooling | roughly half |
| 5. C++ class | not started |

---

## 1. Core language semantics

**Have:** variables, constants, `int`, `float`, `str`, `bool`, `bytes`,
nil semantics through `?T`, lists `[]T`, maps `{K: V}`, structs, a unit
type through `void!`, explicit conversions (`int()`, `float()`,
`str()`), equality by contents for lists, maps and structs, ordering on
numbers and strings.

**Missing:**

- **characters** - there is no `char`; a single character is a `str` of
  length one, and `bytes` indexing gives an `int`
- **enums** - the keyword is not even reserved
- **unions / tagged unions**
- **tuples** - a function returns one value, or a struct
- **references and pointers** - needs the C backend
- **type aliases**
- **operator overloading**

**Deliberately not planned:** truthiness. `if 5` is an error, and `if`
requires a `bool`. That is a design decision, not a gap.

---

## 2. Functions

**Have:** first-class functions, closures, recursion, multiple
parameters, anonymous functions, callable values, order-independent
declaration, return-path checking.

**Missing:** default arguments, variadic parameters for user functions
(builtins like `print` are variadic; your own functions cannot be),
function overloading, multiple return values, calling conventions.

---

## 3. Type system

**Have:** static typing, type inference, a real type checker in
`check.go` that types every expression and records it on the tree,
nullable types with narrowing inside `if x != nil`, explicit casts, no
implicit conversion, `match`.

**Missing, in priority order:**

1. **Generics.** The biggest single gap in the language. Without them
   `Set`, `Queue` and `Stack` cannot be written in Veyl itself, which
   is why they are absent from the standard library rather than merely
   unwritten.
2. **Traits or interfaces.** Structs compose but cannot share a
   contract.
3. **Algebraic data types** and exhaustive pattern matching. `match`
   compares values; it cannot destructure.
4. Constraints and bounds, `typeof`, compile-time type information,
   recursive types.

---

## 4. Memory model

**Nothing here exists, and the reason is structural.** Veyl compiles to
Go and inherits Go's garbage collector. There is no ownership model, no
`alloc`, no `free`, no pointers, no `sizeof`, no alignment control, no
custom allocators, no pointer arithmetic.

`own` and `unsafe` are reserved keywords that do nothing.

**This entire section is one prerequisite: the C backend.** Adding any
of it to the Go backend would be pretending.

---

## 5. Structs and object model

**Have:** struct definitions, nested structs, methods through `impl`,
composition, visibility through `pub`, contents-based equality.

**Missing:** constructors, destructors, static methods and fields,
interfaces or traits, virtual dispatch, operator overloading.

**Not planned:** inheritance. Composition plus traits is the better
model and the one to aim at.

---

## 6. Control flow

**Have:** `if` / `else`, `while`, `for i in a..b` with `step`,
`for x in list`, `for k, v in map`, `break`, `continue`, `return`,
`match`.

**Missing:** labeled loops and breaks, `defer` (reserved, does nothing),
destructuring pattern matching.

---

## 7. Error handling

**The strongest area in the language, and genuinely first class.**

**Have:** `T!` for a value or a reason it failed, `void!` for an action
with nothing to return, `?` propagation, `must`, `valueOr`, `isOk`,
`errorOf`, `fail`, `ok`, compile-time enforcement that a result cannot
be used without being unwrapped, and Veyl-level stack traces on a crash
naming your `.vy` lines rather than Go's.

**Missing:** error metadata beyond a message, typed errors, catching a
panic.

---

## 8. Modules and packages

**Have:** `import`, `pub`, namespaced package imports so two packages
can both export `hello`, private symbols, a package manager
(`veyl init/add/remove/install/packages`), dependency resolution,
version pinning, `veyl.lock` with a SHA-256 that notices a moved tag,
circular imports detected and refused.

**Missing:** module initialisation order, build configuration, a
central registry (deliberately, see PACKAGES.md).

---

## 9. Compilation and execution

**Have:** lexer, parser, AST, resolver, semantic analysis, type
checker, code generation, native executable output, cross-compilation
through `VEYL_TARGET`, error accumulation, parser error recovery, and
diagnostics that name Veyl types and point at `.vy` lines.

**Missing, and this is the part that matters for credibility:**

- **an intermediate representation of its own.** Go source is
  currently the IR. A real IR is what a second backend and any
  optimiser would both need.
- **an optimiser.** Go's optimiser does the work; Veyl contributes none.
- object files, linking control, debug and release builds,
  optimisation levels, debug symbols, incremental compilation.

---

## 10. Native interoperability

**Nothing.** No FFI, no calling C, no C-compatible structs, no linking
native libraries, no exporting Veyl functions.

Blocked on the same thing as memory: the C backend.

---

## 11. Concurrency

**Have:** structured concurrency through `task`, with `task.all`,
`task.each`, `task.map` and `task.mapLimit`. Deliberately no raw
goroutines are exposed.

**Missing:** threads, mutexes, read/write locks, atomics, condition
variables, thread-local storage, channels, async/await, memory-order
semantics.

The current design is a deliberate simplification, not an oversight.
Whether to add the primitives is a design question, and it interacts
with the C backend, where `task` does not map cleanly.

---

## 12. Standard library

**Have:** strings and formatting, string parsing, lists, maps,
iterating helpers (`map`, `filter`, `reduce`, `find`, `any`, `all`,
`sort`, `sortBy`), math, random, statistics, files, directories, paths,
environment variables, processes, time and date, networking, HTTP,
JSON, CSV, hashing, regular expressions, compression through `zip`,
raw binary through `bytes`, terminal colour, logging, command-line
arguments, bit manipulation, and the Windows library.

**Missing:** sets, queues, stacks, deques, priority queues, ordered
maps, a real iterator abstraction (the helpers are eager, not lazy),
signals.

**Most of these are blocked on generics**, not on effort. A `Set`
written for one element type would be a worse thing to have than none.

---

## 13. Compile-time capabilities

**Have:** compile-time constants, and constant folding only where Go
does it.

**Missing:** compile-time evaluation, `static_assert`, conditional
compilation, macros or any metaprogramming, compile-time code
generation, reflection.

This is Level 5 and correctly untouched.

---

## 14. Tooling

**Have:** a formatter (`veyl fmt`, tested to never change what a
program does), warnings for unused variables and unreachable code,
syntax highlighting for four editors generated from the compiler's own
tables, a package manager, an interactive console (`veyl console`),
`veyl doctor`, a Windows installer bundling its own toolchain, and
compiler diagnostics that are genuinely good.

**Missing:** a language server (autocomplete, go-to-definition,
in-editor errors), a linter beyond the two warnings, debugger
integration, a documentation generator, **a test runner for Veyl code**,
benchmarking, a profiler, a build system.

**The language server is the highest-value item here.** Highlighting
without autocomplete or in-editor errors is the difference people feel
first.

---

## 15. Testing and correctness

**Have:** a golden test suite where a case is two files and no Go code,
compile-fail tests, runtime-error tests, unit tests for the lexer, the
type parser and the package manager, a formatter test that proves
formatting never changes behaviour, and regression tests written for
every bug found.

**Missing:** parser fuzzing, cross-platform tests actually *run* on
Linux and macOS (they cross-compile and vet, which proves it type
checks and nothing more), ABI and FFI tests, performance benchmarks.

---

## What to build next, in order

**1. Generics.** Unblocks sets, queues, stacks, iterators and any
serious library. The largest piece of type system work remaining.

**2. A language server.** The biggest felt improvement per hour spent.

**3. A test runner for Veyl code.** Right now the compiler is tested
and programs written in Veyl are not.

**4. The C backend.** Unblocks the whole of Level 3 at once: pointers,
manual memory, `sizeof`, layout, FFI, ABI, real threads and atomics.
Design the IR boundary so it is one module swapped rather than a
rewrite.

**5. Everything in Level 5**, once 1 to 4 exist and not before.

---

## The thing to be honest about

Veyl is a **real compiler** with a real type system, and it is **not a
systems language**. It is garbage collected, it has no pointers, and it
cannot call C. Those are not oversights; they follow from compiling
through Go, which is a normal technique with real prior art and real
costs.

Claiming otherwise is the one thing that would damage the project's
credibility, and every gap above is written down precisely so that it
does not have to be claimed.
