# veylasm - the assembly backend

A second backend for Veyl that emits x86-64 assembly instead of Go.

The compiler in `../src` translates Veyl to Go source and hands it to
the Go toolchain. That works, it produces genuinely native executables,
and it stays the default. What it costs is a garbage collector, a
runtime, a 2.4 MB floor on binary size, a 177 MB Go toolchain inside the
installer, and any hope of pointers or manual memory.

This backend is how those get bought back.

```
hello.vy  ->  [veylasm]  ->  hello.s  ->  [as, ld]  ->  hello.exe
```

## Status

A working subset, compiled all the way to a running `.exe` with no Go
anywhere in the pipeline:

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

Everything else is a compile error naming what is missing: maps,
structs, closures, the error type `T!`, nullable `?T`, and the
namespaced libraries.

It compiles 4 of the 24 programs in the Go backend's own test suite,
and every one of the 12 programs in `examples/` produces byte-identical
output through both backends.

```
$ veylasm run examples/floats.vy
5.5
-2.5
6
0.375
```

`examples/collatz.vy` is the benchmark - nested loops, 10,000 iterations
of real integer work - through both backends:

| | via Go | via assembly |
| --- | ---: | ---: |
| runtime, best of 5 | 67 ms | 81 ms |
| executable size | 2,524,160 bytes | 123,027 bytes |

**About 20% slower and 20x smaller**, and most of that 123 KB is
MinGW's C runtime rather than anything this compiler produced.

Do not read that gap as a finish line. Every value still round-trips
through a stack slot, and Go's optimiser is doing work nothing here
does. It will widen on bigger programs before a register allocator
closes it.

## What it does not have

**No collector.** Every string concatenation, every list and every
float-to-string conversion allocates, and nothing is ever freed. This
is the single largest thing between here and the Go backend, and it is
what the memory model on the roadmap is blocked on.

**No resolver.** A name that is neither a local, a function nor a
builtin is caught by the lowerer rather than the checker, so it is
missed when the checker has already failed on something else. Sharing
`resolve.go` the way `check.go` is now shared is the obvious fix.

**Three deliberate float gaps**, each a compile error rather than a
wrong answer:

- `NAN` - the comparisons lower to `comisd`, which reports an unordered
  pair as both below and equal, so a NaN would compare wrong.
- `INF` - this build links the legacy msvcrt, whose `printf` writes
  `1.#INF` where Go writes `+Inf`.
- `min`/`max` on floats - the integer versions work; the float ones need
  a float compare in the branch-select and were not worth it yet.

## Why assembly text and not machine code

Assembly and machine code are one to one. Every decision that is hard -
instruction selection, register allocation, the calling convention,
stack frame layout - is identical either way. Everything that assembly
text defers is mechanical: byte encoding, jump offset backpatching,
COFF object layout, relocations, PE headers, the import table.

So this does the thinking first and the typing later. When the encoder
and PE writer are written, they replace `x64.go` and nothing above it
changes. That is the point of the split, and it is why `x64.go` is
forbidden from using assembler conveniences: no macros, no
pseudo-instructions, nothing the assembler has to evaluate. Every line
must be one instruction whose bytes we could write ourselves.

## Layout

```
asm-src/
  go.mod            requires ../frontend
  compiler/
    frontend.go     type aliases onto the shared front end
    ir.go           AST -> three-address IR over virtual registers
    x64.go          IR  -> x86-64, GNU as with Intel syntax
    veylasm.go      the driver
    diff_test.go
  examples/
```

The lexer, parser, AST and types live in `../frontend` and are shared
with the Go backend, so both compile the same language from one
definition. `frontend.go` is nothing but Go type aliases, which is what
lets the rest of this package write `Expr` and `PLUS` unqualified.

`ir.go` is the boundary. Nothing in it knows an x86 register exists.

One real gap: the **type checker is not shared yet**, so this backend
does almost no checking. It tracks int against bool, purely so that
`print` of a comparison writes `true` rather than `1`, and that is all.
A program that is wrong will still compile to something. Run it through
the Go backend first. Sharing `check.go` is blocked on the builtin
table, which needs the checker and the code generator at once.

## Commands

```
veylasm run   f.vy    compile and run
veylasm build f.vy    write an executable next to the source
veylasm asm   f.vy    print the generated assembly
veylasm ir    f.vy    print the intermediate representation
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

MinGW's `as` and `gcc`, for assembling and linking. Found on `PATH`, at
the usual MSYS2 and MinGW install locations, or via `VEYL_MINGW`.

That dependency is exactly what this backend exists to remove. It is not
worse than the Go backend's dependency on the Go toolchain, and it goes
away when `x64.go` learns to write bytes.

## What comes next

In dependency order, with the reasoning rather than just the list.

**1. The object header.** Before structs and closures multiply the
number of places that allocate, decide the shape of a heap object: a
header word saying what it is and how big. A collector has to tell a
pointer from an integer at runtime, and right now every value is an
anonymous eight bytes, so it cannot. This is cheap now and expensive
later, and it does not require writing a collector - only making one
possible.

**2. Maps.** Four programs in the Go suite block on it. Same approach
as lists: a header plus a hash table built in the IR out of the memory
ops that already exist, with the byte ops in `strings.go` available for
hashing. One real design call - the Go backend sorts keys on iteration
so output is stable, and matching that means either sorting per
iteration or keeping insertion order.

**3. Structs.** Cheapest of the remaining big three: fixed field
offsets, no allocation strategy to invent, and `alloc`/`loadmem`/
`storemem` already do everything needed.

**4. A register allocator.** Where the 20% gap starts closing. Do it
after functions exist - which they now do - because the whole
difficulty is knowing which values are live across a call, and a
version written before calls existed would be written twice.

**5. The collector.** Needs step 1 done first. Budget a whole session
minimum; a collector that frees one live object produces a bug that
surfaces somewhere else entirely, hours later.

**6. The byte writer and PE emitter.** What finally removes the MinGW
dependency. Mechanical by then: `x64.go` is replaced and nothing above
it changes.

Two papercuts worth ten minutes whenever:

- `veylasm f.vy` should work as shorthand for `run`, matching `veyl`.
- The linker's real output should surface instead of being swallowed;
  that is what turned the PATH bug into a twenty-minute hunt.

A note on what this is not for. It will not be faster than the Go
backend for a long time, and probably starts out several times slower.
Go's optimiser is doing a great deal of work that nothing here does yet.
The reasons to build this are binary size, compile speed, dropping the
bundled toolchain, and unblocking pointers and manual memory. Speed
comes last, if at all.
