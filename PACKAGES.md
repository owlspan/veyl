# Quartz packages

How to use a library, and how to write one other people can use.

---

## Contents

- [Using a package](#using-a-package)
- [quartz.json](#quartzjson)
- [quartz.lock](#quartzlock)
- [Writing a package](#writing-a-package)
- [Publishing](#publishing)
- [Working on a package locally](#working-on-a-package-locally)
- [How it works underneath](#how-it-works-underneath)
- [Design decisions, and what they cost](#design-decisions-and-what-they-cost)
- [Known limitations](#known-limitations)

---

## Using a package

Start a project, add a dependency, import it.

```
quartz init myapp
quartz add github.com/someone/quartz-strutil@v1.0.0
```

Everything a package exports is reached through the name you imported
it as:

```qz
import "strutil"

print(strutil.titleCase("hello world"))
```

The import name defaults to the repository name. A repository is often
called something like `quartz-strutil`, which cannot be a name in code —
`quartz-strutil.titleCase` reads as a subtraction. `quartz add` says so
and tells you to pick one:

```
quartz add github.com/someone/quartz-strutil@v1.0.0 as strutil
```

**A bare name is a package; a name ending in `.qz` is a file.** That is
the whole rule, and it is why the two can never be confused:

```qz
import "helpers.qz"    // a file next to this one
import "strutil"       // a package from quartz.json
```

The commands:

| Command | Effect |
| --- | --- |
| `quartz init [name]` | create `quartz.json` here |
| `quartz add <source>` | fetch a package and record it |
| `quartz add <source> as <name>` | ...under a different import name |
| `quartz remove <name>` | drop a dependency |
| `quartz install` | fetch everything the manifest lists |
| `quartz packages` | list dependencies and whether they are installed |

You rarely need `quartz install` yourself — a missing package is fetched
on demand when you compile. It exists for setting up a freshly cloned
project, and for checking every dependency against the lock file at
once.

---

## quartz.json

The same format describes an application and a library. A package is
just a project somebody else depends on.

```json
{
  "name": "strutil",
  "version": "1.0.0",
  "main": "strutil.qz",
  "description": "String helpers Quartz does not ship with",
  "dependencies": {
    "greet": "github.com/someone/greet@v2.1.0"
  }
}
```

| Field | Meaning |
| --- | --- |
| `name` | **Required.** What the package calls itself. |
| `version` | What this release is. Required to publish; ignored for an app. |
| `main` | The file an importer gets. Defaults to `<name>.qz`. |
| `description` | One line, for humans. |
| `dependencies` | Import name to source. |

A **source** is either a repository at a tag or branch:

```
github.com/owner/repo@v1.0.0
github.com/owner/repo@main
```

or a path, for something you are writing yourself:

```
../mylib
```

A tag is tried before a branch of the same name. Prefer tags: a branch
moves, and while `quartz.lock` will notice, being told your dependency
changed is worse than it not changing.

---

## quartz.lock

Written automatically. Records exactly what was installed:

```json
{
  "packages": {
    "strutil": {
      "source": "github.com/someone/quartz-strutil@v1.0.0",
      "version": "v1.0.0",
      "sha256": "9f2c1a..."
    }
  }
}
```

The hash covers every file name and every byte in the package. It
exists because **a git tag can be moved.** Someone can publish
`v1.0.0`, have a hundred people depend on it, then repoint the tag at
different code. `quartz install` recomputes the hash and stops if it
does not match:

```
strutil: the contents of github.com/someone/quartz-strutil@v1.0.0 changed
    since it was locked.
    expected 9f2c1a0b3d4e
    got      771be3c8a012
    The tag was moved. Check what changed before trusting it;
    'quartz add ...' again if the new code is what you want.
```

**Commit `quartz.lock`.** It is what makes a checkout on another machine
build the same program.

---

## Writing a package

A package is a directory with a `quartz.json` and at least one `.qz`
file. There is nothing else to it — no build step, no registration, no
account.

```
quartz-strutil/
  quartz.json
  strutil.qz
  README.md
```

```json
{
  "name": "strutil",
  "version": "1.0.0",
  "main": "strutil.qz",
  "description": "String helpers Quartz does not ship with"
}
```

```qz
// strutil.qz

pub fn titleCase(s: str) -> str {
    let out: []str = []
    for word in split(s, " ") {
        if len(word) > 0 {
            push(out, upper(substr(word, 0, 1)) + lower(substr(word, 1, len(word))))
        }
    }
    return join(out, " ")
}

// No pub, so this is private to the package.
fn isVowel(c: str) -> bool {
    return contains("aeiou", lower(c))
}
```

### `pub` is the whole API surface

Anything marked `pub` is visible to whoever imports the package.
Anything else is yours to rename or delete without breaking anybody.
This applies to functions, structs, and top-level constants.

Being deliberate about `pub` is the single most useful thing you can do
for the people using your library.

### Splitting across files

The `main` file can import others next to it, the ordinary way:

```qz
// strutil.qz
import "internal/casing.qz"
```

Those files are part of your package. Importers never name them, and
only your `pub` declarations reach them.

### What to name things

Everything you export is reached through your package's name, so
`strutil.get()` reads fine even though `get()` alone would not. Name
things for how they read **after** the dot, and do not prefix them with
the package name: `strutil.strTitleCase()` is a stutter.

Two packages exporting the same name is fine and always has been safe —
the namespace keeps them apart.

### Depending on other packages

A package can have its own `dependencies`. They are fetched
transitively. Keep the list short: every dependency you take is one
your users also take, and its public names land in their namespace too.

---

## Publishing

There is no registry and no account. Publishing is:

1. Push the repository to GitHub.
2. Tag a release.

```bash
git tag v1.0.0
```

```bash
git push --tags
```

That is genuinely all. Anyone can now:

```
quartz add github.com/you/quartz-strutil@v1.0.0
```

### Versioning

Use `vMAJOR.MINOR.PATCH`, and keep the `v`.

- **patch** — a fix, no API change
- **minor** — new `pub` things, nothing existing broken
- **major** — you removed or changed something `pub`

Since there is no version resolver, a major bump is simply a different
package as far as anyone's manifest is concerned. They upgrade when
they edit the version in `quartz.json` and not before, which is the
behaviour most people want from a dependency anyway.

**Never move a tag that has been published.** Cut a new one. The lock
file will catch you, loudly, in your users' builds.

### A checklist before tagging

- `quartz.json` has `name`, `version`, `main`, `description`
- The `version` matches the tag you are about to push
- Every intended export is `pub`, and nothing else is
- The repository name works as an identifier, or your README shows the
  `as` form people should use
- A `README.md` showing the three lines someone needs to get started
- It compiles from a clean checkout: `quartz run` something that uses it

---

## Working on a package locally

Point a dependency at a directory instead of a repository:

```
quartz add ../quartz-strutil
```

```json
"dependencies": {
  "strutil": "../quartz-strutil"
}
```

Now edits to the library show up in the next compile with no fetching,
no cache, and no version. This is the right way to develop a library
and the program using it at the same time.

Local dependencies are deliberately **not** written to `quartz.lock` —
there is nothing stable to record. Swap the path for a real source
before you publish.

---

## How it works underneath

Worth knowing, because it explains the failure modes.

**Fetching.** A package is downloaded as a `.tar.gz` from
`codeload.github.com` over HTTPS and unpacked. Git is never invoked, so
nobody needs it installed. Downloads are capped at 64 MB.

**The cache** is shared between all your projects:

```
%LOCALAPPDATA%\quartz\pkg\github.com\owner\repo@v1.0.0\
```

Same version, same bytes, so one copy is enough. Delete the folder to
force a refetch. Packages are unpacked to a `.partial` directory and
renamed on success, so an interrupted download never leaves something
behind that looks cached.

**Archive safety.** Entry paths are checked before anything is written:
an entry naming `../` or an absolute path is refused outright, and
symlinks are skipped. An archive that could write outside its own
directory could overwrite anything you can write.

**Resolution.** `import "name"` looks up `name` in the nearest
`quartz.json` at or above the file doing the importing, resolves it to
a source, fetches it if the cache lacks it, reads that package's own
manifest, and loads its `main` file.

**Namespacing.** A package's declarations are recorded under
`name.thing` rather than merged into your program. That is what lets two
packages export the same `hello` without colliding. Inside the package a
bare name still means that package's own declaration, so a library calls
its own helpers without qualifying them — and cannot reach into the
program that imported it. The compiler emits `greet.hello` as
`greet__hello` in the generated Go, since Go would otherwise read the
dot as a package selector.

A relative `import "helpers.qz"` is **not** namespaced. Files in the
same project merge, which is what you want for code you wrote yourself.

---

## Design decisions, and what they cost

**No central registry.** A registry is a service somebody has to run
forever, and it is the wrong thing for a language at this stage to
depend on. The cost: no search, no discovery, no `quartz search json`.
You find packages the way you find Go modules — someone tells you.

**Tarballs over HTTPS, not git.** Costs nothing in dependencies and
nobody needs git. The cost: no commit-hash pinning, only tags and
branches.

**Exact versions, no resolver.** There are no ranges and no
`^1.2.0`. If two of your dependencies need different versions of the
same package, you are told and the build stops. The cost is obvious and
it is the point: a resolver quietly picking a version nobody tested is
a worse failure than one you have to fix by hand.

**Trust on first use.** The first `quartz add` records whatever it
downloads. Nothing verifies that the author is who they claim to be —
there are no signatures. What the hash protects is *change*: code that
shifts under a version you already locked. Read a package before you
depend on it, the same as anywhere else.

---

## Known limitations

**Only GitHub.** `gitlab.com` and the rest are rejected with a clear
message rather than half-working. Adding another host is a small change
to `tarballURLs` in `pkg.go`.

**No `quartz search`, no discovery.** Consequence of having no
registry.

**No transitive version conflict report yet.** Two packages depending on
different versions of a third are fetched independently; the collision
surfaces as duplicate names rather than as a version conflict.

**No signatures.** See trust on first use, above.
