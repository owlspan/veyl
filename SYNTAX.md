# Quartz Language Reference

**Version 0.5** — the language as currently implemented.

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
| `int`     | whole numbers                | `42`, `-7`          |
| `float`   | numbers with a decimal point | `3.14`, `0.5`       |
| `str`     | text                         | `"hello"`           |
| `bool`    | `true` or `false`            | `true`              |
| `[]T`     | a list of `T`                | `[1, 2, 3]`         |
| `{K: V}`  | a map from `K` to `V`        | `{"a": 1}`          |

A number literal is an `int` unless it contains a `.`, in which case it
is a `float`.

Quartz does not convert between types automatically. Mixing them is an
error — use `str()`, `int()`, or `float()` to convert explicitly.

```qz
let n = 5
print("count: " + n)        // error: cannot add str and int
print("count: " + str(n))   // fine
print("count: {n}")         // better — interpolation handles it
```

### The rules

The compiler knows the type of every expression and checks it before
generating any code. The rules are short:

| Construct | Rule |
| --- | --- |
| `+` | two numbers of the same type, or two `str` |
| `- * /` | two numbers of the same type |
| `%` | two `int` — use `mod()` for floats |
| `< <= > >=` | two numbers of the same type, or two `str` |
| `== !=` | two values of the same type |
| `&& \|\| !` | `bool` only |
| `if` / `while` | the condition must be `bool` — `if 5` is an error |
| `let x: T = v` | `v` must be a `T` |
| `x = v` | `v` must match the type `x` was declared with |
| `return v` | `v` must match the function's return type |
| `f(a, b)` | each argument must match the parameter's type |
| `for i in a..b` | the bounds and the `step` must be `int` |

There is **no implicit conversion between two values**. This is an
error, and the message says so in Quartz's words:

```qz
let a = 1
let b = 2.5
let c = a + b   // error: cannot mix int and float in '+'
                //        convert one with int(...) or float(...)
```

### Integer literals are flexible

The one exception is a plain integer literal, which will happily become
a float where one is expected. This follows Go's untyped-constant rule
and keeps ordinary arithmetic readable:

```qz
let radius = 2.5
let area = PI * radius * radius   // fine
let wide  = radius * 2            // fine — 2 becomes 2.0
let ratio: float = 1              // fine
```

The flexibility belongs to the *literal*, not to the type. Once a value
is in a variable, its type is fixed:

```qz
let two = 2                       // an int variable
let nope = radius * two           // error: cannot mix float and int
let ok   = radius * float(two)    // fine
```

### Division

`/` between two `int` values is integer division, so `7 / 2` is `3`.
That is now a stated rule rather than a leaked backend detail — the
compiler knows both operands are `int` and emits integer division
deliberately.

For a fractional result, use `divf()` or make a side a float:

```qz
print(7 / 2)          // 3
print(divf(7, 2))     // 3.5
print(7.0 / 2.0)      // 3.5
```

### Lists and maps

`[]T` is a list of `T`. `{K: V}` is a map from `K` to `V`, where the key
must be `str` or `int`.

```qz
let nums  = [3, 1, 2]                  // []int
let words = ["beta", "alpha"]          // []str
let ages  = {"ada": 36, "alan": 41}    // {str: int}
```

They nest without any special rules:

```qz
let grid:   [][]int      = [[1, 2], [3, 4]]
let groups: {str: []str} = {}
```

An **empty literal carries no element type**, so it needs an annotation.
The compiler says exactly that if you forget:

```qz
let xs: []int = []          // fine
let m: {str: int} = {}      // fine
let oops = []               // error: cannot tell what kind of list
                            //        this is — annotate it
```

Printing uses Quartz's own notation, so a printed value is something you
could paste back into your program:

```qz
print([1, 2, 3])                   // [1, 2, 3]
print({"a": 1})                    // {"a": 1}
print(["x", "y"])                  // ["x", "y"]
```

**Planned:** `?T` nullable types, `struct`, and a `T!` error type with
`?` propagation.

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

### Iterating a collection

`for x in list` walks the elements. A second name gives the index too.

```qz
for w in words { print(w) }
for i, w in words { print("{i}: {w}") }
```

A map binds two names, key and value:

```qz
for name, age in ages { print("{name} is {age}") }
```

**Map iteration is in sorted key order**, not Go's randomised order. A
loop whose output changes between runs is a bad thing to hand a
beginner, so the keys are sorted first. `keys()` and `values()` are
sorted for the same reason.

A `str` is not directly iterable — use `chars(s)` or `split(s, sep)`.

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

### Lists

`xs[i]` reads an element and `xs[i] = v` writes one. An out-of-range
index stops the program with a plain message rather than a Go stack
trace.

The functions that **change** a list — `push`, `pop`, `insert`,
`removeAt`, `clear` — take the list itself as their first argument, and
that argument has to be a variable or an element, not a temporary. The
functions that **read** a list return a new one and leave the original
alone.

| Function                | Returns  | Description                                |
| ----------------------- | -------- | ------------------------------------------ |
| `push(xs, v, ...)`      | —        | append one or more values                   |
| `pop(xs)`               | element  | remove and return the last element          |
| `insert(xs, i, v)`      | —        | insert `v` at position `i`                  |
| `removeAt(xs, i)`       | element  | remove and return the element at `i`        |
| `clear(xs)`             | —        | remove everything                           |
| `first(xs)` `last(xs)`  | element  | the first or last element                   |
| `slice(xs, a, b)`       | list     | a copy of the range; indexes are clamped    |
| `reverse(xs)`           | list     | a reversed copy                             |
| `sort(xs)`              | list     | a sorted copy; numbers or strings           |
| `sum(xs)`               | number   | the total of a list of numbers              |
| `join(xs, sep)`         | `str`    | every element joined into one string        |
| `contains(xs, v)`       | `bool`   | whether `v` is in the list                  |
| `indexOf(xs, v)`        | `int`    | position of `v`, or `-1`                    |
| `len(xs)`               | `int`    | how many elements                           |

```qz
let xs: []int = []
push(xs, 3, 1, 2)
print(sort(xs))        // [1, 2, 3]
print(xs)              // [3, 1, 2] — sort returned a copy
```

### Maps

`m[k]` reads and `m[k] = v` writes. **A missing key reads as the zero
value** — `0`, `""`, `false` — so use `has()` when the difference
matters.

| Function        | Returns | Description                            |
| --------------- | ------- | -------------------------------------- |
| `has(m, k)`     | `bool`  | whether the key is present              |
| `remove(m, k)`  | —       | delete a key                            |
| `keys(m)`       | list    | the keys, sorted                        |
| `values(m)`     | list    | the values, in sorted key order         |
| `clear(m)`      | —       | remove everything                       |
| `len(m)`        | `int`   | how many entries                        |

```qz
let counts: {str: int} = {}
for w in split("a b a", " ") {
    counts[w] += 1        // a missing key starts at 0
}
print(counts)             // {"a": 2, "b": 1}
```

### Splitting strings

| Function          | Returns  | Description                          |
| ----------------- | -------- | ------------------------------------ |
| `split(s, sep)`   | `[]str`  | split on a separator                  |
| `chars(s)`        | `[]str`  | one entry per character               |
| `lines(s)`        | `[]str`  | split on line breaks                  |

### Utility

| Function        | Returns | Description                             |
| --------------- | ------- | --------------------------------------- |
| `len(x)`        | `int`   | length of a `str`, list, or map          |
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
| `winBuild()`                          | the Windows build number, as an `int`          |
| `isWin11()`                           | whether the build is 22000 or higher           |
| `openWindow(title, w, h)`             | opens a real window; returns whether corners were rounded |
| `openWindow(title, w, h, rounded)`    | same, with rounded corners requested or not    |

```qz
setTitle("My App")
beep(880, 150)
let rounded = openWindow("Hello from Quartz", 800, 500)
messageBox("Done", "Corners rounded: {rounded}")
```

`openWindow` **blocks** until the user closes the window — it runs the
Win32 message loop internally.

**Rounded corners need Windows 11** (build 22000+). They are requested
by default. On Windows 10 the attribute does not exist, so the window
stays square and `openWindow` returns `false` — it reports what actually
happened, never what was asked for.

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

Honest list of what v0.5 does not do yet.

- **No structs.** Lists and maps exist; user-defined types do not.
- **A missing map key is silent.** `m["absent"]` returns the zero value
  rather than an error. `has()` is the way to tell the difference. This
  gets revisited when nullable types arrive.
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
