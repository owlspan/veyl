# Night log

Unattended session, 11 August 2026. Read this first in the morning.

Everything below is committed locally on `master`. Nothing was pushed —
there is still no remote. `git log --oneline` is the short version;
`git reset --hard v0.3` puts everything back the way it was.

---

## What landed

| Tag | What |
| --- | --- |
| `v0.4` | Type checker, test suite, honest Windows version detection |
| `v0.5` | Lists and maps |
| — | Dotted library namespaces, and the `os` library |
| — | `http`, `net`, `time` and `mem` libraries |
| `v0.6` | Structs, `impl` blocks and methods |
| `v0.7` | JSON |
| — | Bitwise operators and `match` |

The compiler is now six stages: lex, parse, resolve, **check**, codegen,
Go. `go.mod` is still empty — no dependency was added.

Try it:

```
quartz run examples\tools.qz
```

The roadmap through v0.7 is done, plus the libraries you asked for that
were not on it. What is left from your list is manual memory, which
genuinely needs the C backend and cannot be faked on the Go one.

---

## Decisions I made without you

These are real language design forks. Each was decided the way the docs
recommended, or with a stated reason for departing. All are reversible —
say the word and I will flip any of them.

**1. No implicit conversion between values.** `a + b` with an `int` and
a `float` is an error. This is what both CLAUDE.md and ROADMAP.md
recommended.

**2. Integer literals stay untyped.** This one is a refinement, not
what the docs said. Strictly following rule 1 means `radius * 2` fails
when radius is a float, and you would write `float(...)` constantly.
Go solves this with untyped constants, so Quartz does the same: a plain
integer literal adapts to a float, but a variable never does.

```qz
let radius = 2.5
let a = radius * 2        // fine — 2 is a literal
let two = 2
let b = radius * two      // error — two is an int variable
```

**3. Map iteration is sorted.** Go randomises it. A loop whose output
changes between runs is a bad thing to hand a beginner. Costs a sort
per loop; `keys()` and `values()` are sorted for the same reason.

**4. A missing map key reads as the zero value.** `m["absent"]` gives
`0` or `""` rather than failing. `has(m, k)` tells the difference. This
is the thing most worth revisiting when `T!` lands.

**5. Collections print in Quartz notation.** `[1, 2, 3]` and
`{"a": 1}`, not Go's `[1 2 3]` and `map[a:1]`, so a printed value is
something you can paste back into your program.

**6. Library failure convention.** No error type exists yet, so:
a call returning a value is fatal on failure with a one-line message,
unless its name ends in `Or`; a call that only acts returns a `bool`.
Visible in the name, and the first thing to convert at v0.7.

**7. `os.file.read` is the documented spelling.** Your `os.read.file`
works too — it is registered as an alias. Noun-first groups better as
the library grows, but both compile to the same call.

**8. Struct methods can change the struct.** The roadmap recommended
value semantics, and assignment does copy — `let b = a` gives two
independent values. But methods take a pointer receiver, so `scale()`
and `birthday()` work. Value semantics is about assignment; without
this, no method could ever modify anything.

**9. Struct literals cannot appear bare in an `if` header.** `if p {`
is ambiguous between a variable and the start of `p{...}`. Same rule
and same reason as Go. Parentheses lift it.

**10. Bitwise operators use C's precedence.** Which means comparison
binds tighter than `&`, so `flags & 4 == 4` is C's classic trap. I kept
C's ordering rather than inventing a third convention, and made the
checker explain the fix when it sees the resulting bool.

**11. `json.decode` reads its target from the annotation.**
`let p: Point = json.decode(text)` — Quartz has no type arguments, so
the binding is what tells the decoder what to build. Without an
annotation it is a compile error that says so.

---

## Things worth your attention

**Floats print without a decimal point.** `print(2.0)` shows `2`,
because that is what Go's `%v` does. Quartz now draws a hard line
between `int` and `float`, so hiding the difference when printing is
arguably wrong — but changing it is a design decision, not a bug fix,
so I left it. Worth a ruling.

**`quartz.exe` is committed as ignored.** `.gitignore` already covered
it. `tests/` is tracked; the built `examples/*.exe` are not.

**GUI runtime behaviour is still unverified** beyond version detection.
`winBuild()` returns 19045 on this machine and `isWin11()` correctly
returns false, both confirmed against the real OS — so rounded corners
will now honestly report failure here rather than claiming success.
But I did not open a window, because `openWindow` blocks until someone
closes it and there was nobody to close it.

**Roadmap order changed.** The test suite (item 10.1) was pulled all the
way forward to v0.4, because making sweeping unattended changes to a
compiler with three examples as its only safety net was not defensible.
It caught real regressions the same night.

---

## Where I got to, and what is next

The roadmap is done through v0.7. Remaining, in the order I would take
them:

**Nullable types `?T` and the error type `T!`** (roadmap v0.7 in the
original numbering). This is the biggest remaining language feature and
it retires the ugliest thing in the current design: the fatal-on-failure
convention across `os`, `http` and `json`. It also fixes the silent
missing-map-key.

**Modules.** `import` still does nothing, and every program is one file.

**Polish**: a formatter, warnings for unused variables and unreachable
code, `--version`, a VS Code grammar.

**The C backend**, and only then manual memory, pointers and `unsafe`.
Everything low-level on your list depends on this, and it is a genuinely
large piece of work rather than an afternoon.
