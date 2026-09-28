# Changelog

Each release on GitHub carries the installer and the section below for
its version.

## 0.26.0

**PNG images, with transparency.** `win.image` loads `.png` as well as
`.bmp`, and `win.draw` blends a PNG by each pixel's alpha, so a sprite
with a transparent background draws the way it looks in an editor.
`win.drawScaled` blends too.

The decoder is written in Veyl, in the prelude: DEFLATE, the PNG
chunks and the five row filters. Every colour type is supported at 8
bits, and palette images at 1, 2, 4 and 8 bits with a tRNS chunk.
Interlaced and 16-bit images are refused with a message. Fixtures for
each case are in `tests/png`, checked against the exact pixels.

An image handle is now a block holding the bitmap, its size and
whether it has alpha, outside the collector like a window.

## 0.25.0

**Images.** `win.image` loads a `.bmp`; `win.draw`, `win.drawScaled`
and `win.drawKeyed` put it in the window - at its own size, stretched,
or with one colour left out so a sprite has a transparent background.

**Measuring text.** `win.textWidth` and `win.textHeight`, so a label
can be centred instead of guessed at.

**Canvases.** `win.canvas(width, height)` is a back buffer with no
window, and `win.pixel` reads a colour back from it or from a window.
Every drawing function works on one.

**Sound.** `sound.play`, `sound.loop` and `sound.stop` play a WAV in
the background.

`examples/gui/sprites.vl` uses all of it.

Fixed: a window's state lived on the collected heap behind an `int`
handle, so since automatic collection a window kept in a struct or a
list could have been freed while still open.

## 0.24.0

**Global variables.** A top-level `var` is visible inside every
function and can be changed from any of them:

```veyl
var score = 0
fn award(points: int) { score += points }
```

A function reaching for a top-level `let` now gets an error that says
so and suggests `var`, instead of "undefined variable".

**Enums.**

```veyl
enum State { Idle, Running, Done }
var state = State.Idle
print(state)                // Idle
```

Values compare with `==`, print by name, and cannot be mixed with ints
or other enums. A `match` on an enum without an `else` has to handle
every value, and names the ones it misses.

Fixed:

- `pub` was never enforced on this backend: private functions,
  structs and constants of an imported file could be used freely.
- Every type error in an imported file was reported against the main
  file's name. It names the file it is in now.
- Assigning to a `const` reported "undefined variable"; it says the
  name was declared const.

## 0.23.0

**Automatic garbage collection.** The collector runs by itself at the
start of a statement once the live heap passes 4 MB, and after that
once it doubles what survived the last collection. A loop that makes
garbage forever now runs in bounded memory. It waits while `task`
threads run, since it only scans its own stack, and it stays off in a
DLL. `VEYL_GC=off` turns it off; `VEYL_GC=eager` collects at nearly
every statement, and CI runs the whole suite that way.

**Maps are fast.** Lookups are a binary search instead of a scan, and
inserts and removals move entries with one `memmove`. 100,000 inserts
and lookups: 35 s before, 1.5 s now. Integer keys at opposite ends of
the range compared by subtraction, which overflowed and sorted them
wrong; they compare directly now.

**`join` is linear.** It measured the result, allocated once and copied
each piece in, instead of concatenating: 200,000 pieces went from 121 s
to under one. Every byte copy - substrings, file reads, `bytes` - is a
`memmove` instead of a loop.

`mem.goroutines()` counts running task batches instead of always
answering 1.

## 0.22.0

**Callbacks.** A function type on an `extern fn` parameter takes a Veyl
function that native code calls back:

```veyl
extern fn EnumWindows(each: fn(ptr, ptr) -> bool, data: ptr) -> bool from "user32"
fn onWindow(hwnd: int, data: int) -> bool { return true }
EnumWindows(onWindow, 0)
```

Native code is handed a stub that sign-extends the arguments the
callback type declares as C `int`, normalises each `bool`, and jumps to
the function, which then returns straight to its caller. A callback
may allocate and run the collector.

**DLLs.** `veyl build --dll mod.vl` writes `mod.dll`. The top level
runs once, on load; every `export fn` is in the export table. The image
is relocatable with no fix-ups, since nothing in it holds an absolute
address, so a DLL loads wherever Windows puts it.

**`mem.protect`** changes what pages may be used for, and **`mem.scan`**
finds a byte pattern such as `"48 8B ?? ?? 89"` - the two things a mod
needs to find and patch code.

**Strings order**: `<`, `>`, `<=` and `>=` on two strings compare byte
by byte, as on the Go backend. The checker accepted them and the
backend refused them.

`examples/ffi/plugin.vl` is a DLL to start from.

## 0.21.0

**Raw memory.** `mem.alloc`, `mem.resize` and `mem.free` give blocks
outside the collector. `mem.readU8` through `mem.readI64`,
`mem.readF32` and `mem.readF64` read at every width, signed or not;
`mem.write8` through `mem.write64`, `mem.writeF32` and `mem.writeF64`
write. `mem.copy`, `mem.fill`, `mem.str`, `mem.strN`, `mem.bytes` and
`mem.addr` round it out. An address is an `int`, so pointer arithmetic
is ordinary arithmetic.

**`extern struct`**, a C layout over an address:

```veyl
extern struct Player {
    hp: i32
    pos: Vec3
    name: [16]u8
    ammo: u16 at 0x40
}
let p = Player(address)
p.hp -= 10
```

Fields are laid out the way a C compiler would, or pinned with `at`.
They read and write at their own width, nest, and print. An extern
struct can be passed to and returned from an `extern fn`, and a
literal allocates zeroed memory for filling in a Win32 structure.

**`bytes` crosses into `extern fn`** as a pointer to its data, for
functions that fill in a buffer.

**`examples/ffi/memreader.vl`**: lists running programs, finds where
one is loaded, and follows a pointer chain through its memory.

Under the hood: width-specific loads and stores in the IR, and the
byte encoder learned `movsx`, word operands, and the single-precision
conversions, each checked against GNU as.

## 0.20.0

**Reading input.** `input()`, `input(prompt)` and `pause()`. A line
comes back without its ending, and the end of input reads as `""`.

```veyl
let name = input("Your name: ")
print("hi, {name}")
```

**Text builtins** the Go backend had and this one did not: `toFloat`
and `isFloat`, which follow Go's `ParseFloat` including underscores,
`inf` and `nan`; `count`; and `padLeft` and `padRight`, with an
optional fill.

**Characters, not bytes.** `charAt`, `substr` and `chars` count
characters, as they always did on the Go backend. They counted bytes
here, so anything outside ASCII came out cut in half.

**`\xHH` and `\u{...}` string escapes**, so any byte or character can
be written in plain ASCII source: `"caf\u{e9}"`.

**`INF` and `NAN`.** Float comparisons are NaN-correct now - every
comparison with a NaN is false except `!=` - and infinities and NaN
print as `+Inf`, `-Inf` and `NaN` rather than msvcrt's `1.#INF`.

Fixed:

- `floor`, `ceil`, `round` and `trunc` returned 0 for most arguments
  since 0.18.1: load forwarding turned a float bitcast into a plain
  copy, and once the register allocator put the int in a register the
  float return read the wrong register file. They also return `int`
  now, as the docs and the Go backend always said.
- `isNan` was false for a NaN.
- Compile errors whose message contained a colon were sorted into the
  wrong order.
- A float constant equal to another under `==` shared its storage, so
  `-0.0` could become `0.0`.

Tests:

- `tests/` holds programs the Go backend cannot run - the new escapes,
  and programs fed standard input - with the output they must print.

## 0.19.0

**`extern fn`: native functions declared in Veyl source.**

```veyl
extern fn Beep(freq: int, ms: int) -> bool
extern fn mz_extract(zip: str, dest: str) -> int from "miniz"
```

The declaration compiles to a real import-table entry and an x64 call.
There is no wrapper to write and nothing to install, which turns a
package into a plain `.vl` file of declarations. See
[Native functions](docs/SYNTAX.md#native-functions-extern).

Fixed:

- `veyl run app.vl --verbose` now passes `--verbose` to the program.
  The usage text promised it; `os.args()` came back empty.
- The installer now ships `examples/mod`, which `examples/imports.vl`
  needs, and `examples/ffi`.
- Every version string agrees. The installer and the VS Code extension
  said 0.18.1 and the extension's install instructions said 0.10.0.
  A test now fails when they drift.
- The docs no longer describe the Go backend as the compiler, or
  mention `veyl fmt`, `veyl emit`, `veyl builtins` and warnings, none
  of which this compiler has.

Tooling:

- `veyl run` starts the program through `wine` on a non-Windows host,
  so the whole test suite runs on Linux.
- CI runs every test on Windows against the reference backend, and a
  version tag builds the installer and publishes the release.

## 0.18.1

Optimisation passes: constant and branch folding, dead code
elimination, redundant load elimination, slot packing, an assembly
peephole, merging `.rdata` into `.text`, and a register allocator for
short-lived integer values. Executables average 36% smaller, and a
straight-line arithmetic loop runs 2.7x faster.

## 0.18.0

The first release that compiles straight to x86-64 with no Go, no
assembler and no linker. Windows you can draw in, a package manager,
SQLite, HTTPS, a web server, sockets, and editor highlighting.
