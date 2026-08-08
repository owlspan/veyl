# Quartz Language Reference

**Version 0.3** — the language as currently implemented.

Quartz compiles to Go, which compiles to a native executable. A finished
program is a single self-contained binary with no runtime to install.

> Anything marked **planned** is not implemented yet. If you write it,
> the compiler will reject it.

---

## Contents

1. [Hello, world](#hello-world)
2. [Comments](#comments)
3. [Statements and lines](#statements-and-lines)
4. [Variables](#variables)
5. [Types](#types)
6. [Operators](#operators)
7. [Strings and interpolation](#strings-and-interpolation)
8. [Control flow](#control-flow)
9. [Functions](#functions)
10. [Builtin library](#builtin-library)
11. [Reserved words](#reserved-words)
12. [Compiler commands](#compiler-commands)
13. [Known limitations](#known-limitations)

---

## Hello, world

```qz
print("Hello, world!")
```

```
quartz run hello.qz
```

Source files use the `.qz` extension. There is no required `main`
function — statements at the top level of a file run in order.

---

## Comments

```qz
// a line comment

/* a block comment
   /* which can nest */
   all the way to here */
```

Nesting matters in practice: you can comment out a region that already
contains a block comment without the inner `*/` ending it early.

---

## Statements and lines

Statements end at a line break. There are no semicolons.

```qz
let a = 1
let b = 2
```

Blocks use braces, not indentation. Indentation is style only — the
compiler ignores it.

```qz
if a < b {
    print("smaller")
}
```

Inside brackets — `(...)` in a call or a grouped expression — line
breaks are ignored, so long argument lists can wrap:

```qz
print(
    "this",
    "spans",
    "lines"
)
```

---

## Variables

`let` declares a variable. `const` declares one that cannot be reassigned.

```qz
let count = 0
const limit = 10

count = 5          // fine
count += 1         // fine
limit = 11         // error: cannot assign to "limit" because it was declared const
```

The type is inferred from the value. Annotate it explicitly with `: Type`
when you want to be specific:

```qz
let name: str = "Quartz"
let ratio: float = 0.5
```

A name may be declared only once per scope, but an inner block may shadow
an outer name:

```qz
let x = 1
{
    let x = 2       // a different variable
    print(x)        // 2
}
print(x)            // 1
```

Reading an undeclared variable is an error, not an implicit declaration:

```qz
total = 5           // error: undefined variable "total"
```

---

## Types

| Quartz  | Holds                        | Example        |
| ------- | ---------------------------- | -------------- |
| `int`   | whole numbers                | `42`, `-7`     |
| `float` | numbers with a decimal point | `3.14`, `0.5`  |
| `str`   | text                         | `"hello"`      |
| `bool`  | `true` or `false`            | `true`         |

A number literal is an `int` unless it contains a `.`, in which case it
is a `float`.

Quartz does not convert between types automatically. Mixing them is an
error — use `str()`, `toInt()`, or `toFloat()` to convert explicitly.

```qz
let n = 5
print("count: " + n)        // error: mismatched types
print("count: " + str(n))   // fine
print("count: {n}")         // better — interpolation handles it
```

**Planned:** `?T` nullable types, `[]T` lists, `{K: V}` maps, `struct`,
and a `T!` error type with `?` propagation.

---

## Operators

Highest precedence first:

| Precedence | Operators           | Meaning                       |
| ---------- | ------------------- | ----------------------------- |
| 7          | `!x`, `-x`          | logical not, negation         |
| 6          | `*` `/` `%`         | multiply, divide, remainder   |
| 5          | `+` `-`             | add, subtract                 |
| 4          | `<` `<=` `>` `>=`   | comparison                    |
| 3          | `==` `!=`           | equality                      |
| 2          | `&&`                | logical and                   |
| 1          | `\|\|`              | logical or                    |

All binary operators are left-associative: `a - b - c` means `(a - b) - c`.

Use `( )` to group explicitly:

```qz
let a = 2 + 3 * 4        // 14
let b = (2 + 3) * 4      // 20
```

`+` also joins strings. `&&` and `||` short-circuit — the right side is
not evaluated if the left already decides the result.

Compound assignment: `+=`, `-=`, `*=`, `/=`.

**Note:** `int / int` truncates toward zero, so `7 / 2` is `3`. For a
fractional result, make one side a `float`.

---

## Strings and interpolation

String literals use double quotes. Escapes: `\n`, `\t`, `\r`, `\\`,
`\"`, `\0`.

Any expression inside `{ }` is evaluated and inserted:

```qz
let name = "world"
let n = 3

print("hello, {name}")
print("n squared is {n * n}")
print("big: {n > 2 && name != ""}")
```

String literals may appear inside an interpolation, so this works:

```qz
print("shouting: {upper("hello")}")
print("fixed: {replace(path, "\\", "/")}")
```

For a literal brace, double it:

```qz
print("{{literal braces}}")     // {literal braces}
```

Interpolation compiles to a formatting call, so a bare `%` in a string
is safe — it needs no escaping.

---

## Control flow

### if / else if / else

```qz
if score >= 90 {
    print("A")
} else if score >= 80 {
    print("B")
} else {
    print("C")
}
```

The condition needs no parentheses. Braces are required even for a
single statement.

### while

```qz
let i = 0
while i < 10 {
    print(i)
    i += 1
}
```

### for

Counted loops use a range. `..` excludes the end, `..=` includes it.

```qz
for i in 0..5 { print(i) }      // 0 1 2 3 4
for i in 1..=5 { print(i) }     // 1 2 3 4 5
```

`step` sets the increment. A negative step counts down.

```qz
for i in 0..=100 step 25 { print(i) }   // 0 25 50 75 100
for i in 10..0 step -2 { print(i) }     // 10 8 6 4 2
```

The bounds are evaluated once, before the loop starts, so a function
call in the range is not re-run on every iteration.

The loop variable is scoped to the loop and may shadow an outer name
without disturbing it.

### break and continue

```qz
for i in 0..100 {
    if i % 2 == 0 { continue }   // skip to the next iteration
    if i > 20 { break }          // leave the loop
    print(i)
}
```

Both work in `for` and `while`, and both apply to the innermost loop.
Using either outside a loop is a compile error.

**Planned:** `for x in list` once collections exist.

---

## Functions

```qz
fn add(a: int, b: int) -> int {
    return a + b
}
```

- Parameter types are **required**.
- `-> Type` is the return type; omit it for a function that returns nothing.
- Functions must be declared at the top level, not nested.
- Declaration order does not matter — a function may call one defined
  later in the file.
- Recursion works.

```qz
fn fib(n: int) -> int {
    if n < 2 {
        return n
    }
    return fib(n - 1) + fib(n - 2)
}

fn greet(name: str) {
    print("hello, {name}")
    return              // bare return is optional here
}
```

A function with a return type must return on **every** path. This is an
error:

```qz
fn bad(n: int) -> int {
    if n > 0 {
        return 1
    }
    // error: function "bad" must return a value of type int on every path
}
```

### Scope

Each function has its own scope. Top-level variables are local to the
implicit `main`, so functions cannot see them — pass values in as
parameters instead.

```qz
let total = 10

fn show() {
    print(total)        // error: undefined variable "total"
}
```

**Planned:** global constants, default parameters, multiple return values.

---

## Builtin library

Available everywhere; no import needed.

### Output

| Function     | Description                              |
| ------------ | ---------------------------------------- |
| `print(...)` | writes its arguments, then a line break   |
| `write(...)` | writes its arguments with no line break   |

```qz
write("Loading")
write("...")
print("done")           // Loading...done
```

### Input

| Function          | Returns | Description                                    |
| ----------------- | ------- | ---------------------------------------------- |
| `input()`         | `str`   | reads one line                                  |
| `input(prompt)`   | `str`   | writes `prompt`, then reads one line            |
| `pause()`         | —       | waits for Enter; useful before a program exits  |

```qz
let name = input("Your name: ")
print("hi, {name}")
pause()
```

### Conversion

| Function              | Returns | Description                                     |
| --------------------- | ------- | ----------------------------------------------- |
| `str(x)`              | `str`   | any value as text                                |
| `toInt(s)`            | `int`   | parses text; `0` if it isn't a number            |
| `toInt(s, fallback)`  | `int`   | parses text; `fallback` if it isn't a number     |
| `toFloat(s)`          | `float` | parses text; `0` if it isn't a number            |
| `toFloat(s, fallback)`| `float` | parses text; `fallback` if it isn't a number     |
| `isInt(s)`            | `bool`  | whether the text parses as a whole number        |
| `isFloat(s)`          | `bool`  | whether the text parses as a number              |

Surrounding whitespace is ignored, so `toInt(" 7 ")` gives `7`.

**Careful:** `toInt` never fails — it returns the fallback instead. If bad
input should not be silently accepted, check it first:

```qz
let raw = input("Age: ")
while !isInt(raw) {
    print("that isn't a number")
    raw = input("Age: ")
}
let age = toInt(raw)
```

Or pick a fallback that cannot be mistaken for a real answer:

```qz
let age = toInt(input("Age: "), -1)
if age < 0 {
    print("I'll take that as a no.")
}
```

### Math

Every numeric builtin accepts `int` or `float` arguments.

| Function                 | Returns | Description                          |
| ------------------------ | ------- | ------------------------------------ |
| `sqrt(x)` `cbrt(x)`      | `float` | square and cube roots                 |
| `pow(x, y)`              | `float` | x to the power of y                   |
| `exp(x)`                 | `float` | e to the power of x                   |
| `hypot(x, y)`            | `float` | length of the hypotenuse              |
| `log(x)` `log2(x)` `log10(x)` | `float` | logarithms                     |
| `abs(x)`                 | `float` | absolute value                        |
| `mod(x, y)`              | `float` | remainder that works on floats        |
| `floor(x)` `ceil(x)`     | `int`   | round down / up                       |
| `round(x)` `trunc(x)`    | `int`   | round to nearest / toward zero        |
| `clamp(x, lo, hi)`       | `float` | constrain to a range                  |
| `sign(x)`                | `int`   | `-1`, `0` or `1`                      |
| `isNan(x)`               | `bool`  | whether x is not-a-number             |
| `sin` `cos` `tan`        | `float` | trigonometry, in radians              |
| `asin` `acos` `atan`     | `float` | inverse trigonometry                  |
| `atan2(y, x)`            | `float` | angle of the point (x, y)             |

Constants: `PI`, `E`, `INF`, `NAN`.

```qz
print("sqrt(2) is {sqrt(2)}")
print("a full turn is {2 * PI} radians")
print("{floor(3.7)} {ceil(3.2)} {round(2.6)}")
```

### Numbers and conversion

| Function        | Returns | Description                              |
| --------------- | ------- | ---------------------------------------- |
| `int(x)`        | `int`   | drops the fractional part                 |
| `float(x)`      | `float` | converts to a float                       |
| `divf(a, b)`    | `float` | true division, even for two ints          |

**Important:** `/` between two `int` values truncates, so `7 / 2` is `3`.
This is inherited from the backend. Use `divf(7, 2)` for `3.5`, or make
one side a float.

### Randomness

| Function            | Returns | Description                             |
| ------------------- | ------- | --------------------------------------- |
| `random()`          | `float` | between 0 and 1                          |
| `randomInt(lo, hi)` | `int`   | between `lo` and `hi`, both included     |

### Strings

| Function                        | Returns | Description                      |
| ------------------------------- | ------- | -------------------------------- |
| `upper(s)` `lower(s)`           | `str`   | change case                       |
| `trim(s)`                       | `str`   | remove surrounding whitespace     |
| `contains(s, sub)`              | `bool`  | whether `sub` occurs in `s`       |
| `startsWith(s, p)` `endsWith(s, p)` | `bool` | prefix / suffix test         |
| `indexOf(s, sub)`               | `int`   | position of `sub`, or `-1`        |
| `count(s, sub)`                 | `int`   | how many times `sub` occurs       |
| `replace(s, old, new)`          | `str`   | replace every occurrence          |
| `repeat(s, n)`                  | `str`   | `s` joined to itself `n` times    |
| `charAt(s, i)`                  | `str`   | one character; `""` if out of range |
| `substr(s, start, end)`         | `str`   | a slice; indexes are clamped      |
| `padLeft(s, width)` `padRight(s, width)` | `str` | pad with spaces        |
| `padLeft(s, width, fill)`       | `str`   | pad with a chosen character       |

Index-based functions never crash — an out-of-range index gives `""`
rather than stopping the program.

```qz
print(padLeft(str(7), 3, "0"))    // 007
print(substr("Hello, world", 7, 12))
```

### Utility

| Function        | Returns | Description                             |
| --------------- | ------- | --------------------------------------- |
| `len(s)`        | `int`   | number of bytes in a string              |
| `min(a, b, ...)`| —       | smallest of its arguments                |
| `max(a, b, ...)`| —       | largest of its arguments                 |
| `sleep(ms)`     | —       | pauses for `ms` milliseconds             |
| `exit(code)`    | —       | ends the program with an exit code       |

Builtin names cannot be redefined.

---

## Windows library

These call into Win32 and are available only when building for Windows.
Using one on another target is a compile error, not a runtime crash.

| Function                              | Description                                   |
| ------------------------------------- | --------------------------------------------- |
| `setTitle(s)`                         | sets the console window title                  |
| `beep(freq, ms)`                      | plays a tone at `freq` Hz for `ms` ms          |
| `messageBox(title, text)`             | shows a native dialog and waits                |
| `hideConsole()`                       | hides the console window                       |
| `openWindow(title, w, h)`             | opens a real window and waits until it closes  |
| `openWindow(title, w, h, rounded)`    | same, with rounded corners on or off           |

```qz
setTitle("My App")
beep(880, 150)
openWindow("Hello from Quartz", 800, 500)
messageBox("Done", "The window was closed.")
```

`openWindow` **blocks** until the user closes the window — it runs the
Win32 message loop internally. Rounded corners default to on; they need
Windows 11, and on Windows 10 the request is ignored and the window is
square.

`hideConsole()` combined with `openWindow` gives a GUI-only program with
no console behind it.

These compile down to `syscall` calls that load DLLs at runtime, so a
Quartz program with a window is still one self-contained `.exe` with no
DLLs to ship and no C compiler involved.

### Cross-compiling

Set `QUARTZ_TARGET` to build for a different OS than the one you are on:

```
QUARTZ_TARGET=windows quartz build app.qz
```

`quartz run` needs the target to match your machine; use `quartz build`
otherwise.

---

## Reserved words

In use:

```
let const fn return if else while for in step break continue true false
```

Reserved but not yet implemented — the lexer recognises them, so they
cannot be used as names:

```
struct impl self match pub defer own unsafe import nil
```

---

## Compiler commands

| Command                  | Effect                                        |
| ------------------------ | --------------------------------------------- |
| `quartz run f.qz`        | compile and run                                |
| `quartz build f.qz`      | write an executable next to the source         |
| `quartz emit f.qz`       | print the generated Go                         |
| `quartz tokens f.qz`     | print the token stream                         |
| `quartz f.qz`            | same as `run`                                  |

`emit` and `tokens` are debugging aids — `emit` in particular is the
fastest way to understand what the compiler did with your program.

**On Windows:** a program built with `build` and then double-clicked will
open a console, run, and close instantly. That is normal for a console
program. Either run it from a terminal, or end the program with
`pause()`.

---

## Known limitations

Honest list of what v0.2 does not do yet.

- **No type checker.** Quartz checks names, arity, constants and return
  paths, but not types. Type errors are caught by the Go backend and
  reported against your `.qz` line numbers. The messages use Go's
  vocabulary, so `str` appears as `string` and `float` as `float64`.
- **No collections.** No lists, maps, or structs.
- **No modules.** One file per program; `import` does nothing.
- **No global variables.** Top-level variables belong to `main`.
- **Garbage collected.** Memory is managed automatically. Manual memory,
  pointers, and `unsafe` require a C backend and are not available.
- **Windows library is Windows-only.** There is no Linux or macOS
  equivalent for the window and console functions yet.
- **Windows are blank.** `openWindow` opens and manages a real window,
  but there is no drawing or event API yet — no buttons, no input
  handling, no canvas.
- **Go must be installed** to compile a Quartz program, since Quartz
  hands the generated code to the Go toolchain.
