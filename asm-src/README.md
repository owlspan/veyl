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

- integers, bools and strings; `let`, `const`, plain and compound
  assignment; block scoping with shadowing
- all arithmetic, comparisons, `&&`, `||`, `!`, and the bitwise
  operators including signed shifts
- `if` / `else`, `while`, `for` over a range with `step`, `for x in xs`,
  `break`, `continue`, `match`, nested to any depth
- functions with up to any number of arguments, recursion, and
  order-independent declaration
- string concatenation, comparison, and full `"{interpolation}"`
- lists: literals, indexing, index assignment, `push`, `len`, iteration,
  and bounds checks that print the Go backend's exact sentence
- `print`, `write`, `str`, `len`, `abs`, `min`, `max`, `upper`, `lower`,
  `substr`, `charAt`, `indexOf`, `contains`, `startsWith`, `endsWith`,
  `repeat`

Everything else is a clear compile error naming what is missing:
floats, maps, structs, closures, the error type `T!`, and the
namespaced libraries.

```
$ veylasm run examples/functions.vy
49
6765
21
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

**No type checker.** `check.go` lives in `../src` and is not shared,
because it and the builtin table need each other and the code
generator at once. This backend tracks int, bool, str and list only far
enough to pick the right instruction - which `print`, whether `+` means
add or concatenate. A wrong program still compiles to something. Run it
through the Go backend first.

**No collector.** Every string concatenation and every list allocates
and nothing is ever freed. This is the single largest thing between
here and the Go backend, and it is the reason the roadmap's memory
model sits at 11%.

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

Functions and the full calling convention, which is the next real piece
of design: argument registers, callee-saved registers, and a frame that
survives a nested call.

Then a register allocator, because every value currently round-trips
through a stack slot. That is where the 18% turns into something
better, and it is one function - `regAddr` - that has to change.

Then sharing the type checker, so this stops being a backend that
trusts its input.

Then the part that removes the toolchain: an encoder, and a PE writer
with an import table for kernel32.

A note on what this is not for. It will not be faster than the Go
backend for a long time, and probably starts out several times slower.
Go's optimiser is doing a great deal of work that nothing here does yet.
The reasons to build this are binary size, compile speed, dropping the
bundled toolchain, and unblocking pointers and manual memory. Speed
comes last, if at all.
