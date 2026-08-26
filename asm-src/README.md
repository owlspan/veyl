# veyl - the Veyl compiler

Veyl compiled to x86-64, with no Go anywhere.

It used to work the other way. The old compiler translated Veyl to Go
source and handed that to the Go toolchain. It produced genuinely
native executables and it was the only thing that did for a long time.
What it cost was a runtime, a 2.4 MB floor on binary size, a 177 MB Go
toolchain inside the installer, and any hope of pointers or manual
memory.

This is how those got bought back. The old one is on the
[`veylgo`](../../../tree/veylgo) branch, discontinued, and is the
reference this compiler is checked against.

```
hello.vl  ->  [veyl]  ->  hello.exe
```

Nothing in that arrow is another program. veyl encodes the
instructions, links, and writes the PE itself, so a build needs nothing
installed.

## Status

Every one of the 24 programs in the Go backend's own test suite
compiles here, and every one prints the same bytes. So does every
program in `examples/`. The Go backend stays the definition of what
Veyl means; where the two disagree, this one is wrong.

Compiled all the way to a running `.exe` with no Go anywhere in the
pipeline:

- `int`, `float`, `bool`, `str`, and lists of any of them
- `let`, `const`, plain and compound assignment, block scoping with
  shadowing
- all arithmetic on ints and floats, comparisons, `&&`, `||`, `!`, and
  the bitwise operators including signed shifts
- `if` / `else`, `while`, `for` over a range with `step`, `for x in xs`,
  `break`, `continue`, `match`, nested to any depth
- functions with any number of arguments, recursion, order-independent
  declaration, and the full Windows x64 calling convention including
  mixed int/float argument lists
- string concatenation, comparison, and full `"{interpolation}"`
- lists: literals, indexing, index assignment, `push`, `len`,
  iteration, and bounds checks printing the Go backend's exact sentence
- `print`, `write`, `str`, `len`, `push`, `abs`, `min`, `max`, `int`,
  `float`, `divf`, `sqrt`, `mod`, `upper`, `lower`, `substr`, `charAt`,
  `indexOf`, `contains`, `startsWith`, `endsWith`, `repeat`
- the constants `PI` and `E`
- namespaced calls at any depth: a dotted name is looked up like any
  other, so a namespace is a naming convention rather than a scope
- maps `{K: V}` with int or str keys: literals, get, set, `len`,
  growth, a missing key reading the zero value, `keys`, `values`,
  `has`, `remove`, `clear`, and `for k, v in m`
- containers inside containers, to any depth: `[][]int`,
  `{str: []int}`, a list of maps, a struct holding a list
- the error type `T!`: `fail`, `ok`, `isOk`, `failed`, `errorOf`,
  `valueOr`, `must`, postfix `?` propagation, and `void!`
- structs: declarations, literals with zero values for omitted fields,
  nested structs, field read and write, compound assignment on a field,
  structs in lists and maps, value semantics, and printing
- `impl` methods, with the receiver passed by reference so a method that
  changes a field changes the value it was called on
- nullable `?T`: `nil`, narrowing through `if x != nil`, in a struct
  field, in a list, and `find(m, k)`
- deep equality: `==` on a list, a map, a struct or a nullable compares
  contents, generated from the static type
- `str()` of any of those - the same renderer that prints one, with its
  output pointed at a buffer, so the two cannot disagree
- the list library: `first`, `last`, `pop`, `sum`, `reverse`, `sort`,
  `slice`, `join`, `insert`, `removeAt`, `contains`, `indexOf`, and
  `split`, `chars`, `lines` turning a string into a list
- multi-file programs: `import "other.vl"`, with `pub`, and top-level
  consts as real globals in static storage
- the `os` library: `file.read`, `write`, `append`, `lines`, `exists`,
  `size`, `delete`, `rename`, `readOr`; `dir.is`, `list`, `make`,
  `delete`; `path.join`, `dir`, `base`, `ext`; `env.get`, `has`, `set`;
  `run`, `pid`, `cpus`, `hostname`; and `time.now`

- closures and first-class functions, captured by reference, and the
  higher-order builtins `map`, `filter`, `reduce`, `sortBy`, `any`,
  `all`, `each`
- structured concurrency: `task.map`, `task.mapLimit`, `task.each` and
  `task.all`, on real Windows threads, results in list order
- the `bytes` type, and `hash`: md5, sha1, sha256, sha512, crc32, hex
  and base64 both ways, and `hash.file`
- `re`: a backtracking engine written in Veyl, matching Go's
  leftmost-first semantics
- `json` encode and decode, and `csv` parse, write, read and save
- the math, time, bits, args, url, stats, term and rand libraries, all
  written in Veyl in the prelude
- a mark-and-sweep collector, conservative over roots

- `net` TCP sockets and an `http` server and client, through WinSock
- `win`: a window with a game loop, drawing, keyboard and mouse, and
  immediate-mode widgets

Missing against the Go backend: `zip`, and a handful of small builtins
(`input`, `pause`, `padLeft`, `padRight`, `toFloat`, `isFloat`,
`count`). There is also no resolver on this side, so a name that is
neither a local, a function nor a builtin is caught by the lowerer
rather than the checker. Everything absent is a compile error naming
it, never wrong output.

Byte-identical means error messages too, which is why the `os` library
is written against Win32 rather than the C runtime: Go's message for a
missing file is FormatMessage's sentence, and strerror's is a
different one.

```
$ veyl run examples/floats.vl
5.5
-2.5
6
0.375
```

`examples/collatz.vl` is the benchmark - nested loops, 10,000 iterations
of real integer work - through both backends:

The two assembly columns are the same compiler. Only the linking
differs: the middle one goes through gcc, the last is the PE this
compiler writes itself.

| | via Go | asm, via gcc | asm, self-linked |
| --- | ---: | ---: | ---: |
| runtime, best of 7 | 33 ms | 36 ms | 38 ms |
| executable size | 2,524,160 | 123,102 | 2,048 |

**Within about 15% of Go on this little program, and over a thousand
times smaller.** The 123 KB column is mostly MinGW's C runtime rather
than anything this compiler produced, which is what the last column
removes.

collatz is too small to say much about code quality - at forty
milliseconds a run, most of it is process startup - so the honest scale
numbers come from a bigger loop: fifteen million iterations of
straight-line integer arithmetic run 1.12 s compiled by the compiler as
it was before any of the passes landed, 0.60 s with folding,
dead-code elimination, load forwarding and slot packing alone, and
0.42 s with the register allocator on top. Across every example
program, executables average 36% smaller than before the passes.

## What it does not have

**Nothing collects on its own.** There is a mark-and-sweep collector,
but `mem.collect()` is the only thing that runs it. Automatic
collection would have to be sure no allocation site is holding a live
pointer in a register between the allocation and the store that parks
it, which is true of every site written so far and is a property
nobody is currently checking.

The practical consequence is worth stating plainly: a loop that
allocates and never terminates will consume all of memory rather than
being collected out of trouble. `scripts/saferun.ps1` runs a program
under a job object memory cap for exactly that case.

**No resolver.** A name that is neither a local, a function nor a
builtin is caught by the lowerer rather than the checker, so it is
missed when the checker has already failed on something else. Sharing
`resolve.go` the way `check.go` is now shared is the obvious fix.

**A string is NUL-terminated bytes**, with no length beside it. So a
file holding a zero byte reads back short where the Go backend reads it
whole, which is what the `bytes` type is for, and building a string by
repeated appending is quadratic, which wants a growable buffer and does
not have one yet.

**Printing a float rounds through msvcrt**, which stops at seventeen
significant digits, so a value needing sixteen can round the wrong way.
The fix is Go's `strconv/decimal.go` in the prelude.

**Two deliberate float gaps**, each a compile error rather than a
wrong answer:

- `NAN` - the comparisons lower to `comisd`, which reports an unordered
  pair as both below and equal, so a NaN would compare wrong.
- `INF` - this build links the legacy msvcrt, whose `printf` writes
  `1.#INF` where Go writes `+Inf`.

## Why assembly text and not machine code

Assembly and machine code are one to one. Every decision that is hard -
instruction selection, register allocation, the calling convention,
stack frame layout - is identical either way. Everything that assembly
text defers is mechanical: byte encoding, jump offset backpatching,
COFF object layout, relocations, PE headers, the import table.

So this did the thinking first and the typing later. The encoder, the
linker and the PE writer came afterwards and nothing above `x64.go`
changed when they did. That is the point of the split, and it is why
`x64.go` is forbidden from using assembler conveniences: no macros, no
pseudo-instructions, nothing the assembler has to evaluate. Every line
must be one instruction whose bytes we could write ourselves, and
`encode_test.go` checks every one of them against GNU `as`.

## Layout

```
asm-src/
  go.mod            requires ../frontend
  compiler/
    frontend.go     type aliases onto the shared front end
    ir.go           AST -> three-address IR over virtual registers
    x64.go          IR  -> x86-64, GNU as with Intel syntax
    library.go      asmLibrary - the builtin table, for the checker
    list.go         lists, built in the IR
    map.go          maps, sorted key and value blocks
    os.go           files and the environment, on Win32
    osdir.go        directory listing, making and removing
    result.go       the error type T!, built in the IR
    struct.go       struct layout, copying and printing
    strings.go      the string library, built in the IR
    veyl.go      the driver
    diff_test.go
  examples/
```

The lexer, parser, AST and types live in `../frontend` and are shared
with the Go backend, so both compile the same language from one
definition. `frontend.go` is nothing but Go type aliases, which is what
lets the rest of this package write `Expr` and `PLUS` unqualified.

`ir.go` is the boundary. Nothing in it knows an x86 register exists.

**The type checker is shared too.** What
unblocked it was the builtin table: the two backends do not have the
same set of builtins, so the checker could not assume either one.
`frontend/library.go` is the interface it asks instead, and
`library.go` here implements it as `asmLibrary`. A builtin this backend
lacks comes back as a clean "not on the assembly backend yet" error
rather than a checker crash.

So a wrong program is now caught here, with the same message the Go
backend would give. The remaining gap is the **resolver**, described
above - still `src`-only.

## Commands

```
veyl f.vl          compile and run
veyl run   f.vl    the same thing, spelled out
veyl build f.vl    write an executable next to the source
veyl asm   f.vl    print the generated assembly
veyl ir    f.vl    print the intermediate representation
```

`asm` and `ir` matter more here than `veyl emit` does on the Go
backend. When a register allocator produces something wrong, reading
the instruction stream is the only way to see it.

## Testing

```
go test ./...
```

There are no expected-output files. Every program in `examples/` is run
through both backends and the output compared byte for byte, with the Go
backend as the definition of what Veyl means. If they disagree, this one
is wrong.

It earned that on the first program ever compiled. The numbers were
correct and the line endings were not: the C runtime translates `\n` to
`\r\n` on stdout and Go does not. In a terminal the two were
indistinguishable. In bytes they were not. `x64.go` now puts stdout in
binary mode in the prologue.

The second test checks that everything outside the subset is a compile
error rather than wrong output. A backend that quietly mis-compiles what
it does not understand is worse than one that refuses.

## Requirements

To build a program: nothing. veyl encodes, links and writes the PE
itself.

To run the tests: Go, for the differential comparison against the other
backend, and MinGW's `as` and `gcc`. `encode_test.go` checks every byte
this compiler emits against GNU `as`, and `VEYL_LINK=mingw` takes the
old route through `gcc` so a program that runs one way and not the
other localises the bug to the half that changed. Both are found on
`PATH`, at the usual MSYS2 and MinGW locations, or via `VEYL_MINGW`.
Without them those two checks skip and everything else runs.

## Installing

```
scripts\make-installer.bat
```

Double-click it. It builds `dist\veyl-<version>-setup.exe`, which is
about 5 MB because there is no toolchain to bundle. The Go backend's
installer is roughly 90, most of it a trimmed copy of Go.

## What comes next

In dependency order, with the reasoning rather than just the list.

**1. The object header. Done.** Every heap allocation carries one word
in front of it: `size << 8 | tag`, where the tag says whether the block
is raw bytes, word slots holding no pointers, word slots that are all
pointers, or a list header. `allocObj` in `list.go`.

It buys nothing today. A collector has to tell a pointer from an
integer at runtime, and every value here is an anonymous eight bytes,
so nothing in the heap could say. The tag says it for a whole block at
once. Done before structs and closures multiplied the number of places
that allocate, because retrofitting it across all of them costs far
more than adding it to four sites.

One thing it does not cover: **string literals are static rather than
heap-allocated, so they have no header.** A collector will meet
pointers that do not point into the heap at all. That is a constraint
to design around, not an oversight.

**2. Namespaced calls. Done.** The lowerer required a plain identifier
as the callee, so nothing under `os.`, `time.`, `mem.` and the rest
could be called at all. It now flattens the callee with `DottedName`
and looks that string up like any other builtin, at any depth:
`os.file.write` arrives as one name. A namespace is a naming
convention, not a scope, exactly as on the Go backend.

Implemented so far: `time.now`, `os.env.get`, `os.env.has`. `os.env.set`
is not, because it returns `Void!`.

Adding another is two edits - a signature in `library.go` and a case in
`builtin` in `ir.go` - but it is not the Go backend's one-liner. There
`emit` returns a line of Go and the standard library does the work;
here you write IR and usually a libc call. A function absent from
`library.go` is a type error naming it, so the subset stays honest
without anyone maintaining a list of what is missing.

The one that had to come before writing many of them was the **error
type `T!`**, which is now done - see below. Every library function in
Veyl reports failure as `T!` rather than panicking, so without results
here they could only have been ported in a lying form.

**3. Maps. Done.** Sorted rather than hashed: the Go backend sorts keys
when it prints or iterates, so keeping the array sorted at all times
matches its output for free and moves the cost to insertion. O(n) per
lookup; the six functions in `map.go` are the seam for swapping in a
hash table later.

It did not move parity, which is worth knowing: all seven programs that
reported a map as their first error had something else behind it.

Neither did `T!` or structs. Parity is still 4 of 24. What did change
is what the other twenty are waiting on, and the list is worth
regenerating rather than trusting:

```bash
cd asm-src
for f in ../src/tests/ok/*.vl; do
  ./veyl.exe asm "$f" >/dev/null 2>&1 || echo "$f"
done
```

| blocker | n |
|---|---:|
| closures and first-class functions | 3 |
| `re.*` | 2 |
| unimplemented library functions | 2 |
| `json.decode` | 1 |
| `bytes` | 1 |
| `url.*`, `bits.*`, `args.*` | 1 |

Closures are the biggest single item left, and `tasks` needs structured
concurrency behind them, so realistically they buy two of the three.
Everything else on that list is one program each - which is what
progress looks like from here.

**4. The error type `T!`. Done.** A result is a two-word heap object:
the failure reason, then the value. The layout does not depend on what
it carries, which is what lets `?` propagate a failure out of a `str!`
and into an `int!` by handing back the object it was given rather than
unpacking and rebuilding one - a failure crosses any number of frames
as a single pointer.

Boxing rather than returning a pair is what keeps the IR
three-address and the calling convention unchanged. The Go backend
returns a two-field struct by value; doing that here would have meant
classifying an aggregate return, which is the corner of the Windows x64
ABI with the most rules and the least payoff. The cost is an allocation
per fallible call, unbounded while nothing is freed.

Boxing a plain value into the `T!` a function promises is not decided
by the lowerer. The checker already inserts a `Widen` node at every
such site, the way it does for nullables, so there is one place that
can forget rather than five.

**5. Structs. Done.** Fixed field offsets, so every access is a load or
a store at an offset known at compile time.

The header is where the interest is. A struct is the first thing here
that is genuinely mixed - some words pointers, some not - so the
block-at-a-time tags could not describe one. Fields are reordered to
put every pointer first and the count goes in the eight spare bits the
header already had, which keeps "is this word a pointer" a single
number at no per-instance cost. The reordering is invisible: fields are
reached by name through a compile-time offset.

Value semantics were the part needing care. A struct is a Go struct on
the Go backend, so it copies on assignment, on being passed and on being
returned; here it is a pointer, so the copy is written out. `rvalue`
copies only when the source is a place - a literal or a call result is
already a fresh object nobody else holds. Getting this wrong would not
have crashed, it would have been `let b = a` quietly aliasing.

Still missing on structs: `impl` methods, `str()` of one, and a `T!`
field.

**6. A foreign call op. Done.** `OpCall` emitted `call __vy_<Sym>` and
so could only reach functions this compiler wrote. Anything wanting a
libc or Win32 function had to become its own opcode with its own case
in `x64.go` - which meant the cost of porting the library was one
*opcode* per function, not one edit.

`Instr` now carries `Extern`, `Ret32` and `Variadic`. `Ret32`
sign-extends `eax`, which the many C functions returning an `int` need
because the top half of `rax` is undefined on such a return.
`Variadic` puts a float in both the xmm and the integer register, which
the convention requires when the callee reads a `va_list` and has no
prototype telling it which file to look in. Both are silent when wrong,
which is why they are flags rather than something to remember at each
call site. The three ops that existed only to reach C are gone.

**7. Containers inside containers. Done.** A `vty` carried its element
as a bare kind, so it could say "list of int" but not "list of list of
int". It carries a whole type now. The catch: a vty holds a pointer, so
`==` is no longer type equality, and every place meaning "the same
type" has to go through `eq()`.

**8. `str()` of a container. Done.** The printing code with its output
redirected into a buffer, so `print(xs)` and `str(xs)` cannot disagree
about a character - there is one renderer, not two kept in step. It
allocates per piece appended, so a long list is quadratic and, with no
collector, permanent. That is an argument for a growable buffer, not
for a second renderer.

**9. The os library. Done.** Files, directories and the environment, in
`os.go` and `osdir.go`, entirely through the foreign call op - no new
opcode. Written against Win32 rather than the C runtime because the
failure text has to match Go's, and Go's comes from FormatMessage.

`os.env.set` is the exception that goes through the CRT: a process has
two environments, the block Win32 keeps and the copy the CRT made at
startup, and `SetEnvironmentVariable` updates only the one `getenv`
does not read.

**10. A register allocator.** Where the 20% gap starts closing. Do it
after functions exist - which they now do - because the whole
difficulty is knowing which values are live across a call, and a
version written before calls existed would be written twice.

It also shrinks stack frames, which are large here because nothing is
reused: a 60-line program can need 3,000 virtual registers. That is a
size problem, not a correctness one - a frame bigger than a page has to
touch every page on the way down, or Windows never moves the guard page
and the first write below it faults. `reserve()` in `x64.go` does the
probing, and has to keep doing it however small frames get.

**11. The collector.** Needs step 1, which is done. Budget a whole
session minimum; a collector that frees one live object produces a bug
that surfaces somewhere else entirely, hours later.

**12. The byte writer and PE emitter.** What finally removes the MinGW
dependency. Mechanical by then: `x64.go` is replaced and nothing above
it changes.

The two papercuts that used to sit here are done: `veyl f.vl` is
shorthand for `run`, and the linker's real output surfaces instead of
being swallowed. The one still open is sharing `resolve.go` the way
`check.go` is shared, so that an undefined name is caught here by a
resolver rather than late, by the lowerer, and missed entirely when the
checker has already failed on something else.

A note on what this is not for. It will not be faster than the Go
backend for a long time, and probably starts out several times slower.
Go's optimiser is doing a great deal of work that nothing here does yet.
The reasons to build this are binary size, compile speed, dropping the
bundled toolchain, and unblocking pointers and manual memory. Speed
comes last, if at all.
