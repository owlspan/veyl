# Learn Quartz in 20 minutes

This is the fast tour. It assumes you can already program in something -
Python, JavaScript, a bit of C - and just want to know how Quartz does
it.

For the full rules see [SYNTAX.md](SYNTAX.md). For how the compiler
works see [ARCHITECTURE.md](ARCHITECTURE.md).

---

## Contents

1. [Hello](#hello)
2. [Values](#values)
3. [Deciding and repeating](#deciding-and-repeating)
4. [Functions](#functions)
5. [Lists and maps](#lists-and-maps)
6. [Structs](#structs)
7. [When there is nothing](#when-there-is-nothing)
8. [When it goes wrong](#when-it-goes-wrong)
9. [Doing real work](#doing-real-work)
10. [Several files](#several-files)
11. [Where to go next](#where-to-go-next)

---

## Hello

Put this in `hello.qz`:

```qz
print("Hello, world!")
```

```
quartz run hello.qz
```

No `main`, no imports, no semicolons. Top-level statements run in order.

`quartz build hello.qz` gives you `hello.exe` instead - one file, no
runtime to install, nothing to ship alongside it.

> Quartz compiles through Go, so **Go has to be installed**. Your
> finished `.exe` does not need it; only building does.

---

## Values

```qz
let count = 0            // can change
const LIMIT = 10         // cannot
let name: str = "ada"    // say the type if you want to
```

Six types you will use constantly:

```qz
let n     = 42            // int
let ratio = 2.5           // float
let word  = "hello"       // str
let ok    = true          // bool
let xs    = [1, 2, 3]     // []int
let ages  = {"ada": 36}   // {str: int}
```

**Quartz never converts between types for you.** This is the rule that
surprises people first:

```qz
let n = 5
print("count: " + n)        // error: cannot add str and int
```

Two ways to fix it, and the second is nicer:

```qz
print("count: " + str(n))
print("count: {n}")         // anything in {} is inserted
```

That `{}` works with any expression:

```qz
print("{n} squared is {n * n}")
print("shouting: {upper(word)}")
```

One exception to the no-conversion rule: a plain number *literal* will
become a float where one is needed. A variable will not.

```qz
let radius = 2.5
let area = PI * radius * radius   // fine
let wide = radius * 2             // fine, 2 becomes 2.0

let two = 2
let nope = radius * two           // error: cannot mix float and int
let ok   = radius * float(two)    // fine
```

**Text with backslashes goes in backticks**, where nothing is escaped
and nothing is interpolated:

```qz
const PATTERN = `\d{4}-\d{2}-\d{2}`
```

---

## Deciding and repeating

No parentheses around conditions. Braces always.

```qz
if score >= 90 {
    print("A")
} else if score >= 80 {
    print("B")
} else {
    print("C")
}
```

The condition has to be a `bool`. `if 5` is an error, not a shortcut.

```qz
for i in 0..5 { }             // 0 1 2 3 4
for i in 1..=5 { }            // 1 2 3 4 5
for i in 10..0 step -2 { }    // 10 8 6 4 2

for word in words { }         // over a list
for i, word in words { }      // with the index
for name, age in ages { }     // over a map, in sorted key order

while running {
    tick()
}
```

`match` when you are choosing between values. Arms do not fall through,
so there is no `break` to forget:

```qz
match code {
    200      => print("ok")
    301, 302 => print("redirect")
    else     => print("something else")
}
```

---

## Functions

Parameter types are required. The return type comes after `->`, and is
left off when there isn't one.

```qz
fn add(a: int, b: int) -> int {
    return a + b
}

fn greet(name: str) {
    print("hello, {name}")
}
```

Order does not matter - a function can call one written below it. A
function with a return type must return on **every** path, and the
compiler checks.

Functions are values, so they can be passed around and written inline:

```qz
let double = fn(n: int) -> int { return n * 2 }

fn twice(f: fn(int) -> int, start: int) -> int {
    return f(f(start))
}

print(twice(double, 3))     // 12
```

---

## Lists and maps

```qz
let nums = [3, 1, 2]
push(nums, 4)
print(nums[0], len(nums), sort(nums))
```

`sort` returns a **new** list. Everything that reads leaves the original
alone; everything that changes it - `push`, `pop`, `insert`, `clear` -
says so by taking the list as its first argument.

```qz
let ages = {"ada": 36}
ages["alan"] = 41
ages["ada"] += 1

print(has(ages, "grace"))   // false
print(keys(ages))           // sorted
```

A missing key reads as the zero value - `0`, `""`, `false` - which is
what makes counting a one-liner:

```qz
let counts: {str: int} = {}
for word in split("a b a", " ") {
    counts[word] += 1       // no need to check first
}
```

Use `has()` when "absent" and "zero" are different things, or `find()`,
which gives you nothing rather than a zero.

An **empty literal has no element type**, so it needs an annotation:

```qz
let xs: []int = []
let m: {str: int} = {}
```

And the family that takes a function:

```qz
print(map(nums, fn(n: int) -> int { return n * n }))
print(filter(nums, fn(n: int) -> bool { return n > 1 }))
print(reduce(nums, 0, fn(acc: int, n: int) -> int { return acc + n }))
print(sortBy(nums, fn(a: int, b: int) -> bool { return a > b }))
```

---

## Structs

```qz
struct Point {
    x: float
    y: float
}

impl Point {
    fn length(self) -> float {
        return sqrt(self.x * self.x + self.y * self.y)
    }

    fn scale(self, by: float) {
        self.x *= by
        self.y *= by
    }
}

let p = Point{x: 3.0, y: 4.0}
print(p.length())           // 5
p.scale(2.0)
```

Fields you leave out take their zero value, so `Point{}` is the origin.

**Assigning a struct copies it.** The two are independent afterwards:

```qz
let a = Point{x: 1.0, y: 1.0}
let b = a
b.x = 99.0
print(a.x)      // still 1
```

A method is the exception - it acts on the original, which is what lets
`scale` work at all.

---

## When there is nothing

A plain type can **never** be nil. `?T` is the one that can.

```qz
let note: ?str = nil
```

You cannot use it until you have proved it is there, and checking it
narrows the type inside the block:

```qz
if note != nil {
    print(upper(note))      // note is a plain str in here
}
print(upper(note))          // error: ?str might be nil
```

This is the main thing Quartz offers over most languages: there is no
such thing as a nil `str`, so there is no such thing as a
nil-dereference crash.

---

## When it goes wrong

`T!` is either a `T` or a reason it is missing. Anything in the library
that can fail returns one:

```qz
let text = os.file.read("notes.txt")    // str!, not str
```

Four ways to deal with it:

```qz
must(text)                  // the value, or stop with the reason
valueOr(text, "")           // the value, or a fallback
isOk(text)                  // check first
errorOf(text)               // the reason, "" if it worked
```

And the fifth, which is why the type is worth having. Inside a function
that can itself fail, `?` unwraps or hands the failure upward:

```qz
fn wordCount(path: str) -> int! {
    let text = os.file.read(path)?
    return len(split(trim(text), " "))
}
```

That single character is the whole `if err != nil` dance. Write your own
failures with `fail`:

```qz
fn parsePort(text: str) -> int! {
    if !isInt(text) {
        return fail("{text} is not a number")
    }
    return toInt(text)
}
```

---

## Doing real work

No imports, ever. The library is grouped under dotted names:

```qz
os.file.write("notes.txt", "hello")
let page = valueOr(http.get("https://example.com"), "")
let stamp = time.stamp()
let sum = hash.sha256("abc")
print(re.findAll(`\d+`, "a1 b22 c333"))
```

`os` files and processes · `http` fetching · `net` DNS and ports ·
`json` · `csv` · `time` · `hash` · `re` patterns · `task` concurrency ·
`mem` · `win` Windows-only.

Doing several things at once is `map` with a different name:

```qz
let pages = task.map(urls, fn(u: str) -> str {
    return valueOr(http.get(u), "")
})
```

Everything has finished by the time it returns.

**A whole program** is in `examples\wordfreq.qz` - arguments, a file
that might not be there, a pattern, a map, a struct, and sorting with
your own comparison, in about sixty lines:

```
quartz run examples\wordfreq.qz SYNTAX.md 10
```

---

## Several files

```qz
import "helpers.qz"
```

The path is relative to the file that writes it. A declaration is
private unless marked `pub`:

```qz
// helpers.qz
pub const TAU = 6.283185307179586

pub fn shout(text: str) -> str {
    return "{upper(text)}!"
}

fn internal() -> int {      // not pub: this file only
    return 1
}
```

A top-level `const` is a global, visible inside functions. A top-level
`let` belongs to the program body and is not.

---

## Where to go next

```
quartz fmt yourfile.qz      tidy the formatting
quartz emit yourfile.qz     see the Go it generates
quartz builtins             list everything available
```

`emit` is the best debugging tool here. When something behaves oddly,
read what it actually compiled to.

The compiler warns about things that are legal but probably wrong -
a variable you never read, code that can never run - after your program
compiles, so warnings never bury a real error.

- **[SYNTAX.md](SYNTAX.md)** - every rule, every builtin
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - how the compiler is built
- **`examples\`** - programs that run
- **`editors\vscode\`** - syntax highlighting
