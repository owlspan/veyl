# Veyl for VS Code

Syntax highlighting for `.vl` files: keywords, types, strings with
interpolation, the dotted libraries, all 90 bare builtins, and the
`?`/`!` type markers.

---

## Installing it

There is no marketplace listing. Copy the folder into your extensions
directory and restart VS Code:

**Windows**

```
xcopy /E /I "editors\vscode" "%USERPROFILE%\.vscode\extensions\veyl-lang-0.10.0"
```

**macOS and Linux**

```
cp -r editors/vscode ~/.vscode/extensions/veyl-lang-0.10.0
```

The folder name matters: VS Code expects `name-version` matching
`package.json`. Restart VS Code, open a `.vl` file, and the language
indicator in the status bar should read **Veyl**.

If it does not, run **Developer: Inspect Editor Tokens and Scopes** from
the command palette and put the cursor on a keyword. The scope should
start with `source.veyl`.

---

## What it does and does not do

**Does:** highlighting, comment toggling, bracket matching and
auto-closing, 4-space indentation, and it strips trailing whitespace and
adds a final newline on save - matching what `veyl fmt` produces, so
saving and formatting do not fight each other.

**Does not:** completion, go-to-definition, inline errors, or running
the formatter on save. Those need a language server, which does not
exist. Run `veyl fmt file.vl` by hand.

Reserved-but-unimplemented words - `defer`, `own`, `unsafe` - are
highlighted as errors on purpose. The compiler will refuse them, and
finding that out from the editor beats finding out from a build.

---

## Keeping the builtin list honest

The grammar hard-codes the builtin names, and a hard-coded list drifts.
The compiler can print the real one:

```
veyl builtins
```

Bare names come first, then the dotted library paths. After adding a
builtin, update the `builtins` and `libraries` patterns in
`syntaxes/veyl.tmLanguage.json` from that output.
