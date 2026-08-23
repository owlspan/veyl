# Veyl Language Reference

**Version 0.18.0** - the language as currently implemented.

Veyl compiles to Go, which compiles to a native executable. A finished
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
11. [`win` - windows, drawing and input](#win---windows-drawing-and-input)
12. [Reserved words](#reserved-words)
13. [Compiler commands](#compiler-commands)
14. [Known limitations](#known-limitations)

---

## Hello, world

```veyl
print("Hello, world!")
```

```
veyl run hello.vl
```

Source files use the `.vl` extension. There is no required `main`
function - statements at the top level of a file run in order.

---

## Comments

```veyl
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

```veyl
let a = 1
let b = 2
```

Blocks use braces, not indentation. Indentation is style only - the
compiler ignores it.

```veyl
if a < b {
    print("smaller")
}
```

Inside brackets - `(...)` in a call or a grouped expression - line
breaks are ignored, so long argument lists can wrap:

```veyl
print(
    "this",
    "spans",
    "lines"
)
```

---

## Variables

`let` declares a variable. `const` declares one that cannot be reassigned.

```veyl
let count = 0
const limit = 10

count = 5          // fine
count += 1         // fine
limit = 11         // error: cannot assign to "limit" because it was declared const
```

The type is inferred from the value. Annotate it explicitly with `: Type`
when you want to be specific:

```veyl
let name: str = "Veyl"
let ratio: float = 0.5
```

A name may be declared only once per scope, but an inner block may shadow
an outer name:

```veyl
let x = 1
{
    let x = 2       // a different variable
    print(x)        // 2
}
print(x)            // 1
```

Reading an undeclared variable is an error, not an implicit declaration:

```veyl
total = 5           // error: undefined variable "total"
```

---

## Types

| Veyl  | Holds                        | Example        |
| ------- | ---------------------------- | -------------- |
| `int`     | whole numbers                | `42`, `-7`          |
| `float`   | numbers with a decimal point | `3.14`, `0.5`       |
| `str`     | text                         | `"hello"`           |
| `bool`    | `true` or `false`            | `true`              |
| `[]T`     | a list of `T`                | `[1, 2, 3]`         |
| `{K: V}`  | a map from `K` to `V`        | `{"a": 1}`          |
| `?T`      | a `T`, or nothing            | `nil`               |
| `T!`      | a `T`, or a reason it failed | `fail("bad input")` |
| `fn(A) -> B` | a function                | `fn(n: int) -> int { ... }` |

A number literal is an `int` unless it contains a `.`, in which case it
is a `float`.

Integers can be written in decimal, hex or binary, and `_` may be used
to group digits anywhere:

```veyl
let mask  = 0xFF          // 255
let bits  = 0b1010_1010   // 170
let big   = 1_000_000
let ratio = 1_234.5
```

Veyl does not convert between types automatically. Mixing them is an
error - use `str()`, `int()`, or `float()` to convert explicitly.

```veyl
let n = 5
print("count: " + n)        // error: cannot add str and int
print("count: " + str(n))   // fine
print("count: {n}")         // better - interpolation handles it
```

### The rules

The compiler knows the type of every expression and checks it before
generating any code. The rules are short:

| Construct | Rule |
| --- | --- |
| `+` | two numbers of the same type, or two `str` |
| `- * /` | two numbers of the same type |
| `%` | two `int` - use `mod()` for floats |
| `< <= > >=` | two numbers of the same type, or two `str` |
| `== !=` | two values of the same type; lists, maps and structs compare by contents |
| `&& \|\| !` | `bool` only |
| `if` / `while` | the condition must be `bool` - `if 5` is an error |
| `let x: T = v` | `v` must be a `T` |
| `x = v` | `v` must match the type `x` was declared with |
| `return v` | `v` must match the function's return type |
| `f(a, b)` | each argument must match the parameter's type |
| `for i in a..b` | the bounds and the `step` must be `int` |

There is **no implicit conversion between two values**. This is an
error, and the message says so in Veyl's words:

```veyl
let a = 1
let b = 2.5
let c = a + b   // error: cannot mix int and float in '+'
                //        convert one with int(...) or float(...)
```

### Integer literals are flexible

The one exception is a plain integer literal, which will happily become
a float where one is expected. This follows Go's untyped-constant rule
and keeps ordinary arithmetic readable:

```veyl
let radius = 2.5
let area = PI * radius * radius   // fine
let wide  = radius * 2            // fine - 2 becomes 2.0
let ratio: float = 1              // fine
```

The flexibility belongs to the *literal*, not to the type. Once a value
is in a variable, its type is fixed:

```veyl
let two = 2                       // an int variable
let nope = radius * two           // error: cannot mix float and int
let ok   = radius * float(two)    // fine
```

### Division

`/` between two `int` values is integer division, so `7 / 2` is `3`.
That is now a stated rule rather than a leaked backend detail - the
compiler knows both operands are `int` and emits integer division
deliberately.

For a fractional result, use `divf()` or make a side a float:

```veyl
print(7 / 2)          // 3
print(divf(7, 2))     // 3.5
print(7.0 / 2.0)      // 3.5
```

### Lists and maps

`[]T` is a list of `T`. `{K: V}` is a map from `K` to `V`, where the key
must be `str` or `int`.

```veyl
let nums  = [3, 1, 2]                  // []int
let words = ["beta", "alpha"]          // []str
let ages  = {"ada": 36, "alan": 41}    // {str: int}
```

They nest without any special rules:

```veyl
let grid:   [][]int      = [[1, 2], [3, 4]]
let groups: {str: []str} = {}
```

An **empty literal carries no element type**, so it needs an annotation.
The compiler says exactly that if you forget:

```veyl
let xs: []int = []          // fine
let m: {str: int} = {}      // fine
let oops = []               // error: cannot tell what kind of list
                            //        this is - annotate it
```

Printing uses Veyl's own notation, so a printed value is something you
could paste back into your program:

```veyl
print([1, 2, 3])                   // [1, 2, 3]
print({"a": 1})                    // {"a": 1}
print(["x", "y"])                  // ["x", "y"]
```

### Nullable types

A plain type can **never** be nil. `?T` is the type that can.

```veyl
let name: str  = "ada"    // always holds a string
let note: ?str = nil      // holds a string, or nothing
```

This is the main thing Veyl offers over its own backend: there is no
such thing as a nil `str`, so there is no such thing as a nil-dereference
crash. The cost is that a `?T` has to be checked before it can be used.

**Checking narrows the type.** Inside a proven `!= nil`, the value is a
plain `T`:

```veyl
let note: ?str = maybeLoad()

if note != nil {
    print(upper(note))     // note is a str in here
}
print(upper(note))         // error: ?str might be nil
```

It works in either direction, and through `&&`:

```veyl
if note == nil {
    print("nothing")
} else {
    print(note)            // narrowed in the else branch
}

if note != nil && len(note) > 3 {
    print("long enough")
}
```

The narrowing is deliberately literal-minded: it understands
`x != nil`, `x == nil`, and `&&` chains of them. Anything cleverer would
be harder to predict than it is worth.

**Widening is automatic.** A `T` goes into a `?T` without ceremony:

```veyl
let n: ?int = 5           // fine
```

Going the other way needs the check - that is the whole point.

**A bare `nil` needs a type to aim at**, so a binding that starts empty
must say what it will hold:

```veyl
let x: ?int = nil         // fine
let y = nil               // error: cannot tell what "y" can hold
```

Nullables work anywhere a type does - struct fields, list elements, map
values, parameters and return types:

```veyl
struct Config {
    name:    str
    timeout: ?int
}

fn lookup(m: {str: int}, key: str) -> ?int {
    if has(m, key) {
        return m[key]
    }
    return nil
}
```

`find(m, k)` is the nil-safe counterpart to `m[k]`: it returns `?V`, so
a missing key is distinguishable from a key holding zero.

### Results - things that can fail

`T!` is either a `T`, or a reason it is missing. Where `?T` says
"there might be nothing here", `T!` says "this might not have worked,
and here is why".

```veyl
fn parsePort(text: str) -> int! {
    if !isInt(text) {
        return fail("{text} is not a number")
    }
    return toInt(text)
}
```

Inside a function returning `T!`, `return value` succeeds and
`return fail("...")` does not. There is nothing else to write.

**A result is not a value until you unwrap it.** Using one directly is
an error, which is the whole point:

```veyl
let port = parsePort("8080")
print(port + 1)         // error: int! might have failed
```

Four ways to get at it:

| Way | Meaning |
| --- | --- |
| `must(r)` | the value, or stop the program with the reason |
| `valueOr(r, alt)` | the value, or `alt` if it failed |
| `isOk(r)` / `failed(r)` | test it first |
| `errorOf(r)` | the reason, or `""` if it worked |
| `r?` | the value, or return the failure from *this* function |

### The `?` operator

`?` is the reason the error type is worth having. It unwraps a result,
or returns its failure from the enclosing function:

```veyl
fn addressFor(host: str, port: str) -> str! {
    let n = parsePort(port)?      // on failure, addressFor returns it
    return "{host}:{n}"
}
```

That is Go's four-line `if err != nil` dance in one character.

It works mid-expression, and chains - the first failure wins and
nothing after it runs:

```veyl
fn sumPorts(a: str, b: str) -> int! {
    return parsePort(a)? + parsePort(b)?
}
```

`?` only makes sense in a function that can itself fail, so the
enclosing function must return a `T!`. The compiler says so if not.

### Composing with nullables

`?T` and `T!` stack, and the order means different things:

- `?int!` - it might have failed; if it worked, there might be no value.
- `int!` inside a `?` - not a thing; wrap the other way round.

```veyl
fn maybePort(text: str) -> ?int! {
    if text == "" {
        return nil        // worked, and there is nothing
    }
    return parsePort(text)?
}
```

The standard library uses all of this: `os.file.read` returns `str!`,
`http.get` returns `str!`, and so on. See [Libraries](#libraries).

---

## Structs

A `struct` groups named values into a new type.

```veyl
struct Point {
    x: float
    y: float
}
```

Fields go one per line, or separated by commas - whichever reads better.

### Making one

Name the struct, then give the fields in any order. **Fields you leave
out take their zero value**, so `Point{}` is a valid origin.

```veyl
let origin = Point{}
let p = Point{x: 3.0, y: 4.0}
let q = Point{y: 1.0, x: 2.0}    // order does not matter
```

Read and write fields with `.`:

```veyl
print(p.x)
p.x = 6.0
```

### Methods

Methods go in an `impl` block. The first parameter is always `self`.

```veyl
impl Point {
    fn length(self) -> float {
        return sqrt(self.x * self.x + self.y * self.y)
    }

    fn scale(self, by: float) {
        self.x *= by
        self.y *= by
    }
}

print(p.length())
p.scale(0.5)
```

A method may change `self`, as `scale` does. A method and a field
cannot share a name.

### Copying

**Assigning a struct copies it.** The two values are independent
afterwards - there are no references and no aliasing to reason about.

```veyl
let a = Point{x: 1.0, y: 1.0}
let b = a
b.x = 99.0
print(a.x)      // still 1
```

The exception is a method call, which acts on the original rather than a
copy - otherwise `scale` could not work at all.

### Structs and collections

They nest in both directions:

```veyl
struct User {
    name: str
    tags: []str
}

let people: []User = []
push(people, User{name: "ada", tags: ["maths"]})
push(people[0].tags, "computing")     // changes the element in place

let byName: {str: Point} = {}
byName["origin"] = Point{}
```

A struct cannot contain **itself** by value - the type would need
infinite space. Through a list it is fine:

```veyl
struct Node {
    value:    int
    children: []Node    // fine
}
```

### One syntax wrinkle

`if p {` is ambiguous: `p` could be a variable, or the start of a struct
literal `p{...}`. Veyl resolves it the way Go does - a struct literal
cannot appear unparenthesised in an `if`, `while` or `for` header:

```veyl
if ready { }                                  // fine
if (Point{x: 1.0, y: 0.0}).length() > 0.5 { } // parens needed
```

### Printing

Structs print in the same notation you would write them in:

```veyl
print(Point{x: 3.0, y: 4.0})    // Point{x: 3, y: 4}
```

---

## Operators

Highest precedence first:

| Precedence | Operators           | Meaning                       |
| ---------- | ------------------- | ----------------------------- |
| 11         | `!x` `-x` `~x`      | not, negation, bitwise not    |
| 10         | `*` `/` `%`         | multiply, divide, remainder   |
| 9          | `+` `-`             | add, subtract                 |
| 8          | `<<` `>>`           | bit shifts                    |
| 7          | `<` `<=` `>` `>=`   | comparison                    |
| 6          | `==` `!=`           | equality                      |
| 5          | `&`                 | bitwise and                   |
| 4          | `^`                 | bitwise xor                   |
| 3          | `\|`                | bitwise or                    |
| 2          | `&&`                | logical and                   |
| 1          | `\|\|`              | logical or                    |

All binary operators are left-associative: `a - b - c` means `(a - b) - c`.

Use `( )` to group explicitly:

```veyl
let a = 2 + 3 * 4        // 14
let b = (2 + 3) * 4      // 20
```

`+` also joins strings. `&&` and `||` short-circuit - the right side is
not evaluated if the left already decides the result.

Compound assignment: `+=` `-=` `*=` `/=` `%=` `&=` `|=` `^=` `<<=` `>>=`.

### Bitwise operators

`&` `|` `^` `~` `<<` `>>` work on `int` only. Masks read better in hex
or binary - `0xFF`, `0b1010` - and `_` can group the digits.

```veyl
let flags = 0
flags |= 4          // set a bit
flags &= ~4         // clear it
print(flags & 1)    // test it
```

**The precedence ladder is C's**, which means comparison binds *tighter*
than `&`, `^` and `|`. So this does not do what it looks like:

```veyl
if flags & 4 == 4 { }      // parses as flags & (4 == 4)
if (flags & 4) == 4 { }    // what you meant
```

Veyl keeps C's ordering so the operators behave the way people expect
from elsewhere, and the type checker catches the mistake with a message
that says to add parentheses.

**Note:** `int / int` truncates toward zero, so `7 / 2` is `3`. For a
fractional result, make one side a `float`.

---

## Strings and interpolation

String literals use double quotes. Escapes: `\n`, `\t`, `\r`, `\\`,
`\"`, `\0`.

### Raw strings

Backticks give a string with **no escapes and no interpolation**, which
may span lines:

```veyl
const pattern = `\d{4}-\d{2}-\d{2}`
const table = `name,age
ada,36`
```

Use them for text that is already full of backslashes and braces - a
regular expression, a block of CSV, a chunk of JSON. Quoting that twice
is what goes wrong: in an ordinary string `\d` is an unknown escape and
`{4}` is an interpolation.

There is no way to put a backtick inside one. Use an ordinary string
for that.

Any expression inside `{ }` is evaluated and inserted:

```veyl
let name = "world"
let n = 3

print("hello, {name}")
print("n squared is {n * n}")
print("big: {n > 2 && name != ""}")
```

String literals may appear inside an interpolation, so this works:

```veyl
print("shouting: {upper("hello")}")
print("fixed: {replace(path, "\\", "/")}")
```

For a literal brace, double it:

```veyl
print("{{literal braces}}")     // {literal braces}
```

Interpolation compiles to a formatting call, so a bare `%` in a string
is safe - it needs no escaping.

---

## Control flow

### if / else if / else

```veyl
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

### match

A multi-way branch on one value. Arms may list several values, and
**they do not fall through** - there is no `break` to forget.

```veyl
match code {
    200      => print("ok")
    301, 302 => print("redirect")
    404, 410 => print("gone")
    else     => print("something else")
}
```

An arm's body is a single statement, or a block:

```veyl
match n % 3 {
    0 => {
        print("divisible by three")
        total += n
    }
    else => print("not")
}
```

The subject must be an `int`, `float`, `str` or `bool` - match compares
values, so lists, maps and structs cannot be matched on. Every arm has
to have the same type as the subject, and repeating a value is an error
rather than dead code.

`else` is optional. Without one, a value that matches nothing simply
does nothing. With one, and if every arm returns, the compiler counts
the match as returning on every path.

### while

```veyl
let i = 0
while i < 10 {
    print(i)
    i += 1
}
```

### for

Counted loops use a range. `..` excludes the end, `..=` includes it.

```veyl
for i in 0..5 { print(i) }      // 0 1 2 3 4
for i in 1..=5 { print(i) }     // 1 2 3 4 5
```

`step` sets the increment. A negative step counts down.

```veyl
for i in 0..=100 step 25 { print(i) }   // 0 25 50 75 100
for i in 10..0 step -2 { print(i) }     // 10 8 6 4 2
```

The bounds are evaluated once, before the loop starts, so a function
call in the range is not re-run on every iteration.

The loop variable is scoped to the loop and may shadow an outer name
without disturbing it.

### break and continue

```veyl
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

```veyl
for w in words { print(w) }
for i, w in words { print("{i}: {w}") }
```

A map binds two names, key and value:

```veyl
for name, age in ages { print("{name} is {age}") }
```

**Map iteration is in sorted key order**, not Go's randomised order. A
loop whose output changes between runs is a bad thing to hand a
beginner, so the keys are sorted first. `keys()` and `values()` are
sorted for the same reason.

A `str` is not directly iterable - use `chars(s)` or `split(s, sep)`.

---

## Functions

```veyl
fn add(a: int, b: int) -> int {
    return a + b
}
```

- Parameter types are **required**.
- `-> Type` is the return type; omit it for a function that returns nothing.
- Functions must be declared at the top level, not nested.
- Declaration order does not matter - a function may call one defined
  later in the file.
- Recursion works.

```veyl
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

```veyl
fn bad(n: int) -> int {
    if n > 0 {
        return 1
    }
    // error: function "bad" must return a value of type int on every path
}
```

### Functions are values

A declared function can be handed around like anything else, and
written down anonymously where it is needed:

```veyl
fn double(n: int) -> int {
    return n * 2
}

let f = double                                       // a named function
let triple = fn(n: int) -> int { return n * 3 }      // an anonymous one
print(f(21), triple(5))
```

The type is written the way the signature is:

```veyl
let op: fn(int) -> int = double
let show: fn(str) = fn(s: str) { print(s) }     // returns nothing
```

Parameter types are always required, even in a literal - Veyl does
not infer them.

**Taking and returning them** is what makes callbacks work:

```veyl
fn applyTwice(g: fn(int) -> int, start: int) -> int {
    return g(g(start))
}

fn adder(by: int) -> fn(int) -> int {
    return fn(n: int) -> int { return n + by }
}

print(applyTwice(adder(10), 1))     // 21
```

**Literals close over what is around them**, and can change it:

```veyl
let seen = 0
let tally = fn(n: int) { seen += n }
tally(3)
tally(4)
print(seen)     // 7
```

They go in lists, maps and struct fields like any other value:

```veyl
struct Rule {
    label: str
    test:  fn(int) -> bool
}
```

**A builtin is not a value.** `let p = print` is an error - wrap it:
`fn(s: str) { print(s) }`. Builtins are compiler-known shapes rather
than real functions, and several are variadic or polymorphic in ways no
single signature describes.

### Scope

Each function has its own scope. A top-level `let` is local to the
implicit `main`, so functions cannot see it - pass it in as a
parameter, or make it a `const`, which is global.

```veyl
let total = 10
const LIMIT = 100

fn show() {
    print(LIMIT)        // fine - a top-level const is global
    print(total)        // error: "total" belongs to the program body
}
```

See [Globals](#globals) for the rule and why it works that way.

**Planned:** default parameters, multiple return values.

---

## Builtin library

Available everywhere; no import needed.

### Output

| Function     | Description                              |
| ------------ | ---------------------------------------- |
| `print(...)` | writes its arguments, then a line break   |
| `write(...)` | writes its arguments with no line break   |

```veyl
write("Loading")
write("...")
print("done")           // Loading...done
```

### Input

| Function          | Returns | Description                                    |
| ----------------- | ------- | ---------------------------------------------- |
| `input()`         | `str`   | reads one line                                  |
| `input(prompt)`   | `str`   | writes `prompt`, then reads one line            |
| `pause()`         | -       | waits for Enter; useful before a program exits  |

```veyl
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

**Careful:** `toInt` never fails - it returns the fallback instead. If bad
input should not be silently accepted, check it first:

```veyl
let raw = input("Age: ")
while !isInt(raw) {
    print("that isn't a number")
    raw = input("Age: ")
}
let age = toInt(raw)
```

Or pick a fallback that cannot be mistaken for a real answer:

```veyl
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

```veyl
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

Index-based functions never crash - an out-of-range index gives `""`
rather than stopping the program.

```veyl
print(padLeft(str(7), 3, "0"))    // 007
print(substr("Hello, world", 7, 12))
```

### Lists

`xs[i]` reads an element and `xs[i] = v` writes one. An out-of-range
index stops the program with a plain message rather than a Go stack
trace.

The functions that **change** a list - `push`, `pop`, `insert`,
`removeAt`, `clear` - take the list itself as their first argument, and
that argument has to be a variable or an element, not a temporary. The
functions that **read** a list return a new one and leave the original
alone.

| Function                | Returns  | Description                                |
| ----------------------- | -------- | ------------------------------------------ |
| `push(xs, v, ...)`      | -        | append one or more values                   |
| `pop(xs)`               | element  | remove and return the last element          |
| `insert(xs, i, v)`      | -        | insert `v` at position `i`                  |
| `removeAt(xs, i)`       | element  | remove and return the element at `i`        |
| `clear(xs)`             | -        | remove everything                           |
| `first(xs)` `last(xs)`  | element  | the first or last element                   |
| `slice(xs, a, b)`       | list     | a copy of the range; indexes are clamped    |
| `reverse(xs)`           | list     | a reversed copy                             |
| `sort(xs)`              | list     | a sorted copy; numbers or strings           |
| `sum(xs)`               | number   | the total of a list of numbers              |
| `join(xs, sep)`         | `str`    | every element joined into one string        |
| `contains(xs, v)`       | `bool`   | whether `v` is in the list                  |
| `indexOf(xs, v)`        | `int`    | position of `v`, or `-1`                    |
| `len(xs)`               | `int`    | how many elements                           |

### Working over a list

These take a function, which is why they only became possible once
functions were values. Like `sort` and `reverse`, they return a new
list and leave the original alone.

| Function                | Returns  | Description                                |
| ----------------------- | -------- | ------------------------------------------ |
| `map(xs, f)`            | list     | `f` applied to each element; the type may change |
| `filter(xs, keep)`      | list     | the elements `keep` says yes to             |
| `reduce(xs, start, f)`  | any      | fold into one value, starting from `start`  |
| `sortBy(xs, less)`      | list     | sorted by your own comparison               |
| `any(xs, test)` `all(xs, test)` | `bool` | whether some or every element passes  |
| `each(xs, do)`          | -        | run `do` for each element                   |

```veyl
let nums = [5, 3, 8, 1]

print(map(nums, fn(n: int) -> int { return n * 2 }))
print(filter(nums, fn(n: int) -> bool { return n > 4 }))
print(reduce(nums, 0, fn(acc: int, n: int) -> int { return acc + n }))
print(sortBy(nums, fn(a: int, b: int) -> bool { return a > b }))
```

`reduce` starts from a value of the type you want back, so it can build
something other than a list of the same thing:

```veyl
let words = ["fig", "pear"]
print(reduce(words, "", fn(acc: str, w: str) -> str { return acc + charAt(w, 0) }))
```

```veyl
let xs: []int = []
push(xs, 3, 1, 2)
print(sort(xs))        // [1, 2, 3]
print(xs)              // [3, 1, 2] - sort returned a copy
```

### Maps

`m[k]` reads and `m[k] = v` writes. **A missing key reads as the zero
value** - `0`, `""`, `false` - so use `has()` when the difference
matters.

| Function        | Returns | Description                            |
| --------------- | ------- | -------------------------------------- |
| `has(m, k)`     | `bool`  | whether the key is present              |
| `find(m, k)`    | `?V`    | the value, or nil if the key is absent  |
| `remove(m, k)`  | -       | delete a key                            |
| `keys(m)`       | list    | the keys, sorted                        |
| `values(m)`     | list    | the values, in sorted key order         |
| `clear(m)`      | -       | remove everything                       |
| `len(m)`        | `int`   | how many entries                        |

```veyl
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
| `min(a, b, ...)`| -       | smallest of its arguments                |
| `max(a, b, ...)`| -       | largest of its arguments                 |
| `sleep(ms)`     | -       | pauses for `ms` milliseconds             |
| `exit(code)`    | -       | ends the program with an exit code       |

Builtin names cannot be redefined.

---

## Multiple files

`import` loads another `.vl` file and folds its declarations into your
program. The path is relative to the file that writes it - there is no
search path, no registry, and no package names to learn.

```veyl
import "geometry.vl"
import "shapes/circle.vl"
```

### `pub` decides what escapes

A declaration is private to its own file unless it is marked `pub`:

```veyl
// geometry.vl
pub const TAU = 6.283185307179586

pub struct Vec {
    x: float
    y: float
}

pub fn circleArea(radius: float) -> float {
    return (TAU / 2.0) * radius * radius
}

fn helper() -> int {      // no pub: this file only
    return 1
}
```

Using something private from another file is an error that says so:

```
error: "helper" is private to geometry.vl
       - mark it 'pub fn helper' to use it from another file
```

**Methods are as visible as their struct.** `pub` inside an `impl`
block is an error - a `pub struct` brings its methods with it.

### What an imported file may contain

Declarations only: `import`, `const`, `struct`, `fn` and `impl`. A
loose statement is refused, because there is no sensible moment for it
to run:

```veyl
// in an imported file
print("hello")     // error: an imported file can only declare things
```

Importing the same file twice is harmless - it is folded in once. An
import cycle is an error rather than a hang.

### Globals

**A top-level `const` is a genuine global.** It is visible inside
functions, and across files if it is `pub`.

A top-level `let` is not: it belongs to the implicit `main`, so
functions cannot see it. That is the difference between the two at the
top level, and the compiler explains it when you trip:

```veyl
let count = 10
const LIMIT = 100

fn check() -> bool {
    return LIMIT > 0      // fine, LIMIT is global
}

fn broken() -> bool {
    return count > 0      // error: "count" belongs to the program body
}
```

Because globals are initialised before the program body runs, a `const`
cannot use a `let`:

```veyl
let name = "ada"
const GREETING = "hi {name}"   // error: use 'let' instead of 'const' here
```

---

## Libraries

Everything above is a bare name. The rest of the standard library lives
under a dotted path:

```veyl
let text = os.file.read("notes.txt")
let page = http.get("https://example.com")
print(time.stamp())
```

These need no `import` - the names are always available, and the dots
group them rather than loading anything. `import` is for your own files
and for packages other people wrote; see [PACKAGES.md](PACKAGES.md).

If you get a path wrong the compiler suggests the near misses:

```
error: there is no builtin called os.file.slurp
       - did you mean one of: os.file.append, os.file.delete,
         os.file.exists, os.file.lines, os.file.read, ...
```

### How failure is reported

**Nothing in the library stops your program.** Anything that can fail
says so in its type:

| Shape | On failure |
| --- | --- |
| `os.file.read(p)` → `str!` | a failure carrying the reason |
| `os.file.readOr(p, fallback)` → `str` | returns the fallback |
| `os.file.write(p, text)` → `bool` | returns `false` |

An operation that **produces a value** returns `T!`, so you unwrap it
with `?`, `must()`, `valueOr()` or a check - see
[Results](#results--things-that-can-fail).

```veyl
let text = must(os.file.read("notes.txt"))    // or stop, saying why

fn wordCount(path: str) -> int! {
    let text = os.file.read(path)?            // or hand the reason up
    return len(split(trim(text), " "))
}
```

An operation that **only acts** returns a `bool`. There is no unit type
to put inside a result, so the reason is lost in that one case - a real
gap, and the last thing left to tidy here.

---

### `void!` - an action that can fail but returns nothing

Most fallible things hand back a value, so `T!` covers them. Writing a
file does not: it either worked, or it did not. That is `void!`.

```veyl
fn save(path: str, text: str) -> void! {
    os.file.write(path, text)?
    os.file.append(path, "!")?
    return ok()
}
```

`ok()` is the counterpart to `fail("...")`: it says the action
succeeded. `?`, `isOk`, `errorOf` and `must` all work as they do on any
other result.

Used as a plain statement, nothing changes - the result is simply
ignored:

```veyl
os.file.write("notes.txt", "hello")
```

That is deliberate. Reaching for the reason should be easy, not
mandatory.

**Why this exists.** These operations used to return a plain `bool`, so
"permission denied", "no such directory" and "the disk is full" were
all just `false`. There was no way to say "this failed and here is why"
for something with no value to carry. Now there is:

```veyl
let r = os.file.write("/nowhere/a.txt", "x")
if !isOk(r) {
    print(errorOf(r))   // open /nowhere/a.txt: no such file or directory
}
```

The operations that return `void!`: `os.file.write`, `os.file.append`,
`os.file.delete`, `os.file.rename`, `os.dir.make`, `os.dir.delete`,
`os.dir.change`, `os.env.set`.

`os.file.exists`, `os.dir.is` and `os.env.has` are still `bool`, because
asking a question is not the same as performing an action - there is
nothing for them to fail at.

### `os` - files, directories, paths, processes

| Function | Returns | Description |
| --- | --- | --- |
| `os.file.read(p)` | `str!` | the whole file |
| `os.file.readOr(p, alt)` | `str` | the file, or `alt` if it cannot be read |
| `os.file.lines(p)` | `[]str!` | the file split into lines |
| `os.file.write(p, text)` | `bool` | write, replacing what was there |
| `os.file.append(p, text)` | `bool` | add to the end |
| `os.file.size(p)` | `int!` | size in bytes |
| `os.file.exists(p)` | `bool` | whether it is there |
| `os.file.delete(p)` | `bool` | remove it |
| `os.file.rename(a, b)` | `bool` | move or rename |
| `os.dir.list(p)` | `[]str!` | entry names, sorted |
| `os.dir.make(p)` | `bool` | create, including parents |
| `os.dir.delete(p)` | `bool` | remove a directory and its contents |
| `os.dir.is(p)` | `bool` | whether it is a directory |
| `os.dir.current()` | `str` | the working directory |
| `os.dir.change(p)` | `bool` | change directory |
| `os.dir.home()` `os.dir.temp()` | `str` | well-known directories |
| `os.path.join(a, b, ...)` | `str` | join with the right separator |
| `os.path.base(p)` `os.path.dir(p)` `os.path.ext(p)` | `str` | split a path |
| `os.path.clean(p)` `os.path.absolute(p)` | `str` | tidy or resolve |
| `os.env.get(n)` | `str` | an environment variable, `""` if unset |
| `os.env.set(n, v)` | `bool` | set one |
| `os.env.has(n)` | `bool` | whether it is set at all |
| `os.args()` | `[]str` | command-line arguments |
| `os.run(cmd, args)` | `str!` | run a program, return its output |
| `os.name()` `os.arch()` | `str` | the OS and CPU architecture |
| `os.cpus()` `os.pid()` | `int` | processor count, process id |
| `os.hostname()` | `str` | this machine's name |

The path functions never touch the disk - they are string manipulation.

```veyl
os.file.write("notes.txt", "hello")
for line in os.file.lines("notes.txt") {
    print(line)
}
```

**Shorthand.** `os.read.file(p)`, `os.write.file(p, text)` and a few
others are aliases for the noun-first spellings. Both compile to the
same call; the noun-first form is documented because it groups better as
the library grows.

---

### `http` - a web server, and fetching

Writing a server is the main thing here. `http.serve` takes a port and
a function, calls it once per request, and sends back whatever it
returns.

```veyl
http.serve(8080, fn(req: Request) -> Response {
    if req.path == "/" {
        return http.ok("<h1>hello from Veyl</h1>")
    }
    if req.path == "/time" {
        return http.text(time.stamp())
    }
    return http.notFound()
})
```

That is a complete program. Run it and open `http://localhost:8080`.

**What you are given.** A `Request` is a struct:

| Field | Type | |
| --- | --- | --- |
| `method` | `str` | `"GET"`, `"POST"`, ... |
| `path` | `str` | `"/users"`, with no query string |
| `query` | `str` | everything after `?`, empty if none |
| `body` | `str` | the request body |
| `headers` | `{str: str}` | header names lowercased |

`http.header(req, "user-agent")` reads one header and gives `""` when
it is absent, which saves checking the map first.

**What you send back.** A `Response` is `status`, `contentType` and
`body`, and these build one for you:

| Function | Sends |
| --- | --- |
| `http.ok(body)` | 200, `text/html` |
| `http.text(body)` | 200, `text/plain` |
| `http.json(body)` | 200, `application/json` |
| `http.status(code, body)` | any code, `text/plain` |
| `http.notFound()` | 404 |

You can also build one yourself: `Response{status: 302, contentType:
"text/plain", body: ""}`.

**Fetching.** `http.get(url)` returns the body as `str!`.

```veyl
let page = http.get("http://example.com")?
```

Plain HTTP only. There is no TLS, so an `https://` URL will not work
yet - see [Known limitations](#known-limitations).

**One request at a time.** The server handles a request, finishes it,
then accepts the next. That is fine for a tool, a local dashboard or
something on your own network. It is not a production web server, and a
slow handler blocks everything behind it.

---

### `net` - TCP sockets

`http` is written on top of these, and they are there when you want the
socket itself. A socket is an `int` handle.

| Function | Returns | Description |
| --- | --- | --- |
| `net.listen(port)` | `int!` | a listening socket |
| `net.accept(sock)` | `int!` | blocks until someone connects |
| `net.recv(sock)` | `str!` | read once; `""` when the peer closes |
| `net.send(sock, data)` | `int!` | writes all of it, returns the length |
| `net.connect(host, port)` | `int!` | connect out, by name or address |
| `net.close(sock)` | | |

An echo server:

```veyl
let server = must(net.listen(9000))
print("listening on 9000")

while true {
    let conn = must(net.accept(server))
    let line = must(net.recv(conn))
    must(net.send(conn, "you said: {line}"))
    net.close(conn)
}
```

Two things to know. `net.recv` reads **once** and gives you whatever
had arrived - TCP is a stream, so a message may come back in pieces and
deciding where one ends is yours to do. And a string here is
NUL-terminated, so binary data with a zero byte in it reads back short;
that is what `bytes` is for.

---

### `json` - reading and writing JSON

Two ways to use it, because two different things are usually wanted.

**Whole values.** `json.encode` turns any Veyl value into text, and
`json.decode` turns text back into a declared type. The type comes from
the annotation on the binding - Veyl has no type arguments, so this is
how the decoder is told what to build:

```veyl
struct Point {
    x: float
    y: float
}

let text = json.encode(Point{x: 1.0, y: 2.0})   // {"x":1,"y":2}
let p: Point! = json.decode(text)               // decoding can fail
print(must(p).x)
```

The annotation names the shape *and* says it can fail. It travels
through a wrapper too, so this reads the way you would expect:

```veyl
let p: Point = must(json.decode(text))
```

Field names are encoded exactly as you wrote them. Lists, maps, nested
structs and lists of structs all work.

**Single values.** When you only want one field out of a response,
reach in with a dotted path. Numbers step into arrays:

```veyl
let name  = json.get(body, "user.name")
let age   = json.int(body, "user.age")
let first = json.get(body, "members.0.name")
```

| Function | Returns | Description |
| --- | --- | --- |
| `json.encode(v)` | `str` | any value as JSON |
| `json.pretty(v)` | `str` | the same, indented |
| `json.decode(text)` | the annotated type | decode; fails if the text is bad |
| `json.decodeOr(text, alt)` | `alt`'s type | decode, or `alt` if the text is bad |
| `json.get(text, path)` | `str` | a string field; other values come back as JSON |
| `json.int(text, path)` | `int` | a whole number |
| `json.num(text, path)` | `float` | a number |
| `json.bool(text, path)` | `bool` | a boolean |
| `json.has(text, path)` | `bool` | whether the path exists |
| `json.keys(text, path)` | `[]str` | the keys of an object, sorted |
| `json.count(text, path)` | `int` | length of an array, object or string |
| `json.valid(text)` | `bool` | whether it parses at all |

The `path` is optional everywhere - leave it out to act on the whole
document. A path that does not exist reads as `""`, `0` or `false`
rather than failing; use `json.has` when the difference matters.

**Writing JSON by hand needs doubled braces.** `{` starts an
interpolation inside a string, so a literal one is written `{{`:

```veyl
let text = "{{"x": 1}}"        // the string {"x": 1}
```

Usually it is easier to build the value and encode it than to write the
text out.

---

### `task` - doing several things at once

`task.map` is `map`, run concurrently. Same arguments, same ordered
results - switching between them is a one-word edit.

```veyl
let pages = task.map(urls, fn(u: str) -> str {
    return valueOr(http.get(u), "")
})
```

| Function | Returns | Description |
| --- | --- | --- |
| `task.map(xs, f)` | list | `f` over each element, at once; results stay in order |
| `task.mapLimit(xs, n, f)` | list | the same, at most `n` running at a time |
| `task.each(xs, do)` | - | run `do` for each element, at once |
| `task.all(fns)` | - | run a list of `fn()` at once and wait for the last |

Everything has finished by the time the call returns. There is no way
to start work that outlives the statement that started it.

**There are no goroutines or channels**, deliberately. Veyl has no
mutexes, no atomics and no way to talk about ownership, so raw shared
memory would be the one place the compiler stops helping - every other
sharp edge in the language is either checked or removed.

**The one thing it cannot check is what your function touches.** A
function passed to `task.map` runs on several threads at once, so it
should compute a value from its argument rather than change something
outside itself:

```veyl
// Fine: each call produces a value.
let sizes = task.map(paths, fn(p: str) -> int {
    return valueOr(os.file.size(p), 0)
})

// Not fine: every call writes to the same list.
let out: []int = []
task.each(paths, fn(p: str) {
    push(out, 1)        // a race, and nothing will tell you
})
```

That is a real limit of this design, not an oversight - enforcing it
needs an ownership system Veyl does not have.

---

### `re` - regular expressions

Patterns belong in raw strings. In an ordinary string `{4}` is an
interpolation, so a quantifier would have to be written `{{4}}`, and
`\d` would be an unknown escape.

```veyl
const DATE = `\d{4}-\d{2}-\d{2}`

print(re.find(DATE, log))
print(re.findAll(DATE, log))
```

| Function | Returns | Description |
| --- | --- | --- |
| `re.matches(p, text)` | `bool` | whether the pattern occurs |
| `re.find(p, text)` | `str` | the first match, `""` if none |
| `re.findAll(p, text)` | `[]str` | every match |
| `re.groups(p, text)` | `[]str` | the first match's capture groups |
| `re.count(p, text)` | `int` | how many matches |
| `re.replace(p, text, with)` | `str` | replace every match |
| `re.split(p, text)` | `[]str` | split on the pattern |
| `re.escape(text)` | `str` | quote text to match it literally |
| `re.valid(p)` | `bool` | whether the pattern compiles |

**A pattern that does not compile stops the program.** That is the one
place the library still does this, and it is deliberate: patterns are
almost always literals the author wrote, so a bad one is a bug rather
than a runtime condition, and `must(...)` on every call would be noise.
Use `re.valid` for a pattern that came from input.

Compiled patterns are cached, so using one inside a loop does not
recompile it each time.

---

### `hash` - digests and encodings

| Function | Returns | Description |
| --- | --- | --- |
| `hash.md5(s)` `hash.sha1(s)` | `str` | lower-case hex digest |
| `hash.sha256(s)` `hash.sha512(s)` | `str` | lower-case hex digest |
| `hash.crc32(s)` | `int` | checksum |
| `hash.file(path)` | `str!` | sha256 of a file, read in chunks |
| `hash.base64(s)` `hash.hex(s)` | `str` | encode |
| `hash.fromBase64(s)` `hash.fromHex(s)` | `str!` | decode |

Digests are hex because that is what every other tool prints - a
checksum you cannot compare by eye is not much use. Decoding can fail,
since the input comes from outside; encoding cannot.

---

### `csv` - tables

| Function | Returns | Description |
| --- | --- | --- |
| `csv.parse(text)` | `[][]str!` | rows of fields |
| `csv.write(rows)` | `str` | back to text, quoting as needed |
| `csv.read(path)` | `[][]str!` | parse a file |
| `csv.save(path, rows)` | `bool` | write a file |

Rows may have different lengths. Real files are ragged, and refusing to
read one is less useful than handing it over.

---

### `time` - clocks and formatting

Format strings use readable tokens rather than a reference date:

| Token | Means | Token | Means |
| --- | --- | --- | --- |
| `YYYY` `YY` | year | `HH` | hour, 24-hour |
| `MMMM` `MMM` `MM` | month | `hh` | hour, 12-hour |
| `DDDD` `DDD` `DD` | day | `mm` `ss` | minute, second |

| Function | Returns | Description |
| --- | --- | --- |
| `time.now()` | `int` | seconds since 1970 |
| `time.millis()` `time.nanos()` | `int` | finer resolution, for timing |
| `time.format(unix, fmt)` | `str` | render a timestamp |
| `time.parse(text, fmt)` | `int` | read one back; `-1` if it does not match |
| `time.date()` `time.clock()` `time.stamp()` | `str` | now, preformatted |
| `time.since(unix)` | `int` | seconds elapsed |
| `time.year()` `time.month()` `time.day()` | `int` | parts of today |
| `time.weekday()` | `str` | the day's name |
| `time.sleep(ms)` | - | pause |

```veyl
let started = time.millis()
work()
print("took {time.millis() - started} ms")
```

---

### `mem` - what the program is using

Veyl is garbage collected, so these report and nudge rather than
allocate and free. Manual memory needs the C backend and does not exist.

| Function | Returns | Description |
| --- | --- | --- |
| `mem.used()` | `int` | bytes currently allocated |
| `mem.total()` | `int` | bytes allocated over the whole run |
| `mem.system()` | `int` | bytes obtained from the OS |
| `mem.objects()` | `int` | live object count |
| `mem.collections()` | `int` | how many times the collector has run |
| `mem.collect()` | - | run the collector now |
| `mem.goroutines()` | `int` | concurrent tasks in flight |

### `bytes` - raw binary

`bytes` is a real type, alongside `int`, `str` and the rest. It holds
binary: what comes off a socket, out of a file, or into a hash.

```veyl
let b: bytes = bytes.of("hello")
print(len(b))        // 5
print(b[0])          // 104
print(bytes.hex(b))  // 68656c6c6f
print(b)             // bytes(68656c6c6f)
```

`len` and indexing work as they do on a list. **Indexing gives an `int`
from 0 to 255** - there is no separate `byte` type, because a language
that already has `int` has somewhere to put a small number, and a
second numeric type would infect every arithmetic rule to buy nothing.
`==` compares contents, not identity.

| Function | Returns | Description |
| --- | --- | --- |
| `bytes.of(s)` | `bytes` | encode text |
| `bytes.str(b)` | `str` | decode back to text |
| `bytes.list(b)` / `bytes.ofList(ns)` | `[]int` / `bytes!` | as numbers |
| `bytes.hex(b)` / `bytes.fromHex(s)` | `str` / `bytes!` | hexadecimal |
| `bytes.base64(b)` / `bytes.fromBase64(s)` | `str` / `bytes!` | base64 |
| `bytes.slice(b, a, z)` | `bytes` | a copy of part of it |
| `bytes.concat(a, b, ...)` | `bytes` | join |
| `bytes.find(hay, needle)` | `int` | position, or −1 |
| `bytes.fill(n, value)` | `bytes!` | `n` copies of one byte |
| `bytes.getInt(b, off, size)` | `int!` | read a number |
| `bytes.putInt(n, size)` | `bytes!` | write one |
| `bytes.getIntBE` / `bytes.putIntBE` | | big-endian versions |
| `bytes.read(path)` / `bytes.write(path, b)` | `bytes!` / `void!` | files |
| `bytes.hash(b, algorithm)` | `str!` | md5, sha1, sha256, sha512 |

Integers are little-endian by default, which is what x86 and ARM both
use; the `BE` variants exist because network protocols went the other
way. Sizes are 1, 2, 4 or 8 bytes, and anything else is an error rather
than a guess.

`bytes.slice` clamps instead of failing - reading past the end of a
buffer is ordinary when parsing, and a truncated result is more useful
than a crash. `bytes.concat` and `bytes.slice` both copy, so a later
write cannot reach backwards into something you already handed away.

**Why a separate type.** Binary used to live in a `str`, and the
conversion is in fact lossless. What breaks is everything that assumes
text. On four bytes that are not valid UTF-8:

| Operation | Result |
| --- | --- |
| `len`, `trim` | unchanged |
| `upper` | corrupted |
| `json.encode` | every invalid byte becomes `�` |

The data survives right up until something touches it, and then it is
quietly wrong. A separate type makes that impossible rather than merely
unlikely.

### `rand` - randomness

| Function | Returns | Description |
| --- | --- | --- |
| `rand.int(lo, hi)` | `int` | between `lo` and `hi`, both included |
| `rand.float()` | `float` | between 0 and 1 |
| `rand.bool()` | `bool` | a coin flip |
| `rand.hex(n)` | `str` | `n` random hex digits |
| `rand.uuid()` | `str` | a version 4 UUID |
| `rand.pick(list)` | `T` | one element |
| `rand.shuffle(list)` | `[]T` | a reordered copy |
| `rand.sample(list, n)` | `[]T` | `n` elements, no repeats |
| `rand.seed(n)` | - | fix the sequence |

Programs differ from run to run without seeding. `rand.seed` is for
when you want the same sequence every time - a test, or a puzzle with a
daily number.

```veyl
rand.seed(7)
print(rand.int(1, 100))       // 87, every time
print(rand.pick(["a", "b"]))
```

`rand.shuffle` returns a **new** list and leaves the original alone,
because assignment in Veyl copies and a function that quietly
reordered its argument would not fit that. `rand.pick` on an empty list
gives the zero value; `rand.int(9, 2)`, where the range is backwards,
gives `9` rather than failing.

### `stats` - summarising numbers

Every one of these takes `[]int` or `[]float` and returns `float`.

| Function | Description |
| --- | --- |
| `stats.mean(xs)` | the average |
| `stats.median(xs)` | the middle value, or the middle pair averaged |
| `stats.var(xs)` | the **sample** variance, dividing by n−1 |
| `stats.stdev(xs)` | the square root of that |
| `stats.percentile(xs, p)` | interpolated, `p` from 0 to 100 |

```veyl
let scores = [2, 4, 4, 4, 5, 5, 7, 9]
print(stats.mean(scores))            // 5
print(stats.median(scores))          // 4.5
print(stats.percentile(scores, 90))  // 7.6
```

`stats.var` is the sample variance, dividing by n−1 rather than n. That
is what you want when your numbers are measurements rather than an
entire population, which is the usual case and the one people get
wrong. An empty list gives 0, and so does a single value, which has no
spread to measure.

### `term` - colour and the console

| Function | Returns | Description |
| --- | --- | --- |
| `term.red(s)` and `green` `yellow` `blue` `magenta` `cyan` `grey` | `str` | coloured |
| `term.bold(s)` and `dim` `underline` `invert` | `str` | styled |
| `term.bar(done, total, width)` | `str` | a progress bar |
| `term.clear()` | - | clear the screen |
| `term.colour()` | `bool` | whether colour will actually show |

```veyl
print(term.green("passed") + " " + term.grey("0.4s"))
print(term.bar(7, 10, 20))           // [##############------]
```

**Colour turns itself off when it would be noise.** If output is being
piped into a file or another program, or `NO_COLOR` is set, the styling
functions return the text unchanged rather than filling the file with
escape sequences. `term.colour()` tells you which is happening, so a
program can pick a different layout rather than relying on colour that
is not there.

On Windows the console has to be switched into virtual-terminal mode or
the escape codes appear literally as `←[31m`. Veyl does that
automatically at startup for any program using `term`.

### `url` - taking a URL apart

| Function | Returns | Description |
| --- | --- | --- |
| `url.scheme(u)` `host` `port` `path` `fragment` | `str!` | one piece of it |
| `url.query(u)` | `{str: str}!` | the query string as a map |
| `url.build(base, params)` | `str` | append an encoded query |
| `url.join(base, ref)` | `str!` | resolve a relative reference |

```veyl
let u = "https://api.example.com:8443/v2/items?limit=10"
print(must(url.host(u)))               // api.example.com
print(must(url.port(u)))               // 8443
print(must(url.query(u))["limit"])     // 10
print(url.build("https://x.dev/s", {"q": "veyl"}))
```

A missing piece is an empty string, not a failure - a URL with no port
is perfectly valid. Failure is reserved for a URL that will not parse.
`url.build` sorts its parameters, so the same map always produces the
same URL; that matters for caching and for tests. A repeated query key
keeps its first value, since a map cannot hold both.

### `args` - the command line

| Function | Returns | Description |
| --- | --- | --- |
| `args.flag(name)` | `bool` | was `--name` given |
| `args.value(name, fallback)` | `str` | the value after `--name` |
| `args.rest()` | `[]str` | everything that is not a flag |

```veyl
if args.flag("verbose") {
    print("starting")
}
for file in args.rest() {
    print(must(os.file.read(file)))
}
```

```
veyl run tool.vl --verbose --out=result.txt notes.txt
```

Both `--name=value` and `--name value` work, and one dash or two are
the same. `args.rest()` skips a flag's value, so the list is only the
positional arguments. `os.args()` is still there for the raw list.

### `bits` - bit twiddling and bases

| Function | Returns | Description |
| --- | --- | --- |
| `bits.count(n)` | `int` | how many bits are set |
| `bits.length(n)` | `int` | bits needed to represent it |
| `bits.leading(n)` / `trailing(n)` | `int` | zeros at each end |
| `bits.reverse(n)` | `int` | reverse the low 32 bits |
| `bits.rotate(n, by)` | `int` | rotate the low 32 bits left |
| `bits.toBase(n, base)` | `str` | 2 to 36; `""` if out of range |
| `bits.fromBase(s, base)` | `int!` | parse in that base |

```veyl
print(bits.count(255))            // 8
print(bits.toBase(255, 16))       // ff
print(must(bits.fromBase("1010", 2)))  // 10
```

`& | ^ ~ << >>` are operators in the language already; these are the
operations that need a function.

---

## `win` - windows, drawing and input

Veyl opens a real window you can draw in. The shape is a **game loop**:
open once, then poll, draw, present, repeat.

```veyl
let w = must(win.open("my game", 800, 500))

while win.poll(w) {
    if win.pressed(w, "esc") { break }

    win.clear(w, win.rgb(18, 20, 28))
    win.circle(w, 400, 250, 40, win.rgb(120, 200, 140))
    win.text(w, 20, 20, "hello", win.rgb(235, 238, 245))

    win.present(w)
    sleep(16)
}
win.close(w)
```

That is a complete program. `sleep(16)` is roughly 60 frames a second.

### The loop

| Function | Returns | Description |
| --- | --- | --- |
| `win.open(title, w, h)` | `int!` | a window handle; `w` and `h` are the **drawing area** |
| `win.poll(w)` | `bool` | handle input; `false` once the window is closed |
| `win.present(w)` | | show everything drawn since the last present |
| `win.close(w)` | | |

`win.poll` is what makes the window respond at all. Without calling it
the window freezes, because nothing is reading its messages.

Drawing goes into a back buffer and `win.present` copies it to the
screen in one go. That is why nothing flickers, and why you should draw
a whole frame before presenting rather than presenting as you go.

### Drawing

| Function | Description |
| --- | --- |
| `win.rgb(r, g, b)` | a colour, each 0 to 255 |
| `win.clear(w, colour)` | fill the whole window |
| `win.rect(w, x, y, width, height, colour)` | filled rectangle |
| `win.circle(w, x, y, radius, colour)` | filled circle, centred on `x, y` |
| `win.line(w, x1, y1, x2, y2, colour)` | one-pixel line |
| `win.text(w, x, y, s, colour)` | text, `x, y` is the top-left |
| `win.frame(w, x, y, width, height, colour)` | a one-pixel outline |

`0, 0` is the top-left corner and y grows downwards, which is the usual
convention for screens and the opposite of graph paper.

### Input

| Function | Returns | Description |
| --- | --- | --- |
| `win.pressed(w, name)` | `bool` | is this key held down |
| `win.key(w, code)` | `bool` | the same, by virtual key code |
| `win.mouseX(w)` `win.mouseY(w)` | `int` | pointer position |
| `win.mouseDown(w)` | `bool` | is the left button held |
| `win.clicked(w)` | `bool` | was it clicked **this frame** |

Key names: `"left"`, `"right"`, `"up"`, `"down"`, `"space"`, `"esc"`,
`"enter"`, `"tab"`, `"backspace"`, `"shift"`, `"ctrl"`, `"alt"`, and
any single character - `win.pressed(w, "a")`, `win.pressed(w, "7")`.

The difference between `mouseDown` and `clicked` matters. `mouseDown`
is true for every frame the button is held, so a button wired to it
fires sixty times a second. `clicked` is true for one frame per press.

### Widgets

Built on the above, and written in Veyl. They are **immediate mode**:
there is no widget tree and nothing to keep in sync, so a button draws
itself and tells you whether it was clicked in the same call.

| Function | Returns | Description |
| --- | --- | --- |
| `win.button(w, x, y, width, height, label)` | `bool` | draws a button, true when clicked |
| `win.bar(w, x, y, width, height, frac, colour)` | | a fill bar, `frac` from 0.0 to 1.0 |
| `win.hover(w, x, y, width, height)` | `bool` | is the pointer inside this box |

```veyl
let count = 0
while win.poll(w) {
    win.clear(w, win.rgb(22, 24, 32))
    if win.button(w, 24, 24, 130, 40, "count up") {
        count = count + 1
    }
    win.text(w, 24, 80, "count is {count}", win.rgb(235, 238, 245))
    win.present(w)
    sleep(16)
}
```

Because the button is drawn fresh every frame, moving it is just
changing the numbers - there is nothing to update or lay out again.

### Size and the title

| Function | Description |
| --- | --- |
| `win.width(w)` `win.height(w)` | the drawing area, in pixels |
| `win.title(w, s)` | change the title bar |
| `win.resizable(w, on)` | add or remove the drag border |

**Windows open at a fixed size.** No drag border, no maximise button.
That is almost always what a game wants, and it matches the back buffer
being made once at a known size.

`win.resizable(w, true)` turns it on, and the buffer is rebuilt when
the drawing area changes, so `win.width(w)` and `win.height(w)` are
always current. Draw in terms of them rather than the numbers you
passed to `win.open` and the layout follows the window:

```veyl
win.resizable(w, true)
while win.poll(w) {
    win.clear(w, bg)
    // stays centred however the window is dragged
    win.circle(w, win.width(w) / 2, win.height(w) / 2, 40, fg)
    win.present(w)
    sleep(16)
}
```

The size you pass to `win.open` is the **drawing area**, not the window
including its title bar, so `win.open("x", 640, 400)` gives you exactly
640 by 400 pixels to draw in.

### What it does not do yet

No images, no sound, no text measurement, and one font. Text is drawn
with the system font at a fixed size, so centring a label means
guessing at its width. There is one window per program.

Working examples are in `examples/gui/`: `pong.vl` is a playable game
in about 130 lines, `widgets.vl` exercises the widget set.

## Reserved words

In use:

```
let const fn return if else while for in step break continue true false
struct impl self match nil import pub
```

Reserved but not yet implemented - the lexer recognises them, so they
cannot be used as names:

```
defer own unsafe
```

---

## Compiler commands

| Command | Effect |
| --- | --- |
| `veyl run f.vl` | compile and run |
| `veyl build f.vl` | write `f.exe` next to the source |
| `veyl asm f.vl` | print the generated assembly |
| `veyl ir f.vl` | print the intermediate representation |
| `veyl version` | print the version |
| `veyl f.vl` | same as `run` |

Anything after the `.vl` file goes to your program rather than to the
compiler, so this reaches `args.flag("verbose")`:

```
veyl run tool.vl --verbose
```

### Reading what it compiled to

`asm` and `ir` are the debugging tools, and they are the most useful
thing here when a program does something you did not expect. `ir` is
the readable one - three-address code over virtual registers, before
anything knows an x86 register exists.

```
veyl ir hello.vl
veyl asm hello.vl
```

### Right-clicking a .vl file

The installer puts these on the menu:

- **Run with Veyl**
- **Compile to .exe**
- **Veyl ▸** - the generated assembly, the IR, or a prompt in that
  folder

Right-clicking any folder gives **Open Veyl prompt here**. Double-
clicking a `.vl` file runs it. All of them open a console that stays
up, so you can read what it said whether that was output or errors.

### Errors

Every error names a file, a line and a column, and the compiler reports
all of them rather than stopping at the first:

```
hello.vl:3:11: undefined variable "nmae"
hello.vl:7:5: "count" is declared but never used
veyl: 2 error(s)
```

---

## Known limitations

Honest list of what v0.18.0 does not do yet.

**The language**

- **A missing map key is silent.** `m["absent"]` returns the zero
  value. `has()` and `find()` distinguish it; the bare index was left
  alone because making every map read return `?V` would mean a nil
  check on each one.
- **No namespacing on imports.** Everything `pub` in an imported file
  lands in one flat namespace, so two files exporting the same name
  collide. The error names both files.
- **No global *variables*.** A top-level `const` is global, but a
  top-level `let` belongs to the program body and functions cannot see
  it. Pass it in, or make it a `const`.
- **No generics.** A function works on one set of types.
- **Garbage collected**, and manual memory, pointers and `unsafe` are
  not available.

**The library**

- **No HTTPS.** `http.get` speaks plain HTTP only. TLS needs either
  Windows' own Schannel or a TLS implementation, and neither is
  written, so an `https://` URL will not work.
- **No SQL or database of any kind.** See below.
- **No `zip`.** It exists on the `veylgo` branch and has not been
  ported.
- **The web server handles one request at a time.** Fine for a tool or
  something on your own network, not a production server.
- **A string is NUL-terminated bytes.** Binary data containing a zero
  byte reads back short - use `bytes` for that - and building a string
  by repeated appending is quadratic.
- **Printing a float rounds through the C library**, which stops at
  seventeen significant digits, so a value needing sixteen can round
  the wrong way in the last place.

**Windows**

- **Windows-only.** The whole `win` library, and the compiler's output,
  target Windows on x86-64. There is no cross-compilation.
- **One window per program**, no images, no sound, one font at one
  size, and no way to measure text.

**Memory**

- **Nothing is collected automatically.** There is a mark-and-sweep
  collector, but `mem.collect()` is the only thing that runs it. A loop
  that allocates and never terminates will consume all of memory rather
  than being collected out of trouble.




