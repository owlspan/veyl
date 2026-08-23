# Packages

Libraries that are not built into the compiler. `veyl get <name>`
installs one from here:

```
veyl get totp
```

It lands in `./veyl_modules/totp/` next to the program that uses it,
and `import "totp"` finds it.

| Package | What it is |
| --- | --- |
| `totp` | TOTP and HOTP, the six-digit codes an authenticator app shows |

## Why these are not built in

The installer is 5 MB and the point is keeping it that way. Anything
that is not needed by most programs lives here instead, so a program
that never generates a 2FA code never carries the code that does.

The same goes for anything with a native library behind it: a package
may ship a `.dll`, and `veyl build` copies it next to the executable
only when the package is installed.

## Installing from anywhere

The name form reads from this repository, but a package is just files
at a URL:

```
veyl get totp                        this repository
veyl get github.com/you/thing        any GitHub repository
veyl get https://example.com/x.vl    a single file
```

`github.com/user/repo` reads from that repository's default branch,
whatever it is called.

## Writing one

A package is a directory with a `veyl.pkg` listing its files:

```
# totp/veyl.pkg
totp.vl
```

Every file named there is fetched. A package with no manifest is a
single `<name>.vl`.

Two rules worth knowing:

**Prefix every name.** An import lands in one flat namespace - there is
no `totp.` qualifier yet - so `totpNow` rather than `now`. Two packages
that both export `parse` will collide, and the error names both files.

**`pub` decides what escapes.** Anything without it is private to the
package.

A manifest may name a `.dll` alongside the source. It is installed with
the rest and copied next to the executable at build time, which is how
a package can wrap a native library without the compiler shipping it.
