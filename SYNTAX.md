# Quartz Language Reference

**Version 0.2** — the language as currently implemented.

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

**Planned:** `for x in list`, `for i in 0..10`, `break`, `continue`.

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

| Function       | Returns | Description                              |
| -------------- | ------- | ---------------------------------------- |
| `str(x)`       | `str`   | any value as text                         |
| `toInt(s)`     | `int`   | parses text; `0` if it isn't a number     |
| `toFloat(s)`   | `float` | parses text; `0` if it isn't a number     |

```qz
let age = toInt(input("Age: "))
```

### Utility

| Function        | Returns | Description                             |
| --------------- | ------- | --------------------------------------- |
| `len(s)`        | `int`   | length of a string                       |
| `min(a, b, ...)`| —       | smallest of its arguments                |
| `max(a, b, ...)`| —       | largest of its arguments                 |
| `sleep(ms)`     | —       | pauses for `ms` milliseconds             |
| `exit(code)`    | —       | ends the program with an exit code       |

Builtin names cannot be redefined.

---

## Reserved words

In use:

```
let const fn return if else while true false
```

Reserved but not yet implemented — the lexer recognises them, so they
cannot be used as names:

```
struct impl self match in pub defer own unsafe
import for break continue nil
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
- **No `for` loop.** Use `while`.
- **No `break` or `continue`.**
- **No modules.** One file per program; `import` does nothing.
- **No global variables.** Top-level variables belong to `main`.
- **Garbage collected.** Memory is managed automatically. Manual memory,
  pointers, and `unsafe` require a C backend and are not available.
- **Go must be installed** to compile a Quartz program, since Quartz
  hands the generated code to the Go toolchain.
