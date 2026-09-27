# Changelog

Each release on GitHub carries the installer and the section below for
its version.

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
