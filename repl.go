package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The interactive console: `quartz console`.
//
// Quartz is compiled, so there is no interpreter to feed one line at a
// time. The console works the way compiled-language REPLs generally
// have to: it keeps every line you have typed, and on each new one
// rebuilds and reruns the whole program.
//
// That has one consequence worth stating plainly, because it is
// surprising the first time: **side effects happen again on every
// line.** Appending to a file three times in a row appends more than
// three times. Pure code — the overwhelming majority of what anyone
// types into a console — behaves exactly as expected, and state is
// rebuilt deterministically rather than drifting.
//
// Rerunning everything would also reprint everything, so only the new
// output is shown. The previous run's output is a prefix of this one
// unless the program is nondeterministic, in which case the whole lot
// is printed rather than guessing at a diff.

type console struct {
	lines   []string // every accepted line, in order
	lastOut string   // what the previous build printed
	in      *bufio.Scanner
}

func runConsole() error {
	// Fail before the banner rather than after, so a missing toolchain
	// is not buried under twenty lines of owl.
	if _, err := findGo(); err != nil {
		return err
	}

	fmt.Print(banner())

	c := &console{in: bufio.NewScanner(os.Stdin)}
	c.in.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		line, ok := c.read()
		if !ok {
			fmt.Println()
			return nil
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if done := c.command(line); done {
			return nil
		}
	}
}

// read collects one input, continuing across lines while brackets are
// open so a function or a loop can be typed the way it is written.
func (c *console) read() (string, bool) {
	fmt.Print(paint("qz> ", "36"))
	if !c.in.Scan() {
		return "", false
	}
	line := c.in.Text()

	for openBrackets(line) > 0 {
		fmt.Print(paint("..> ", "36"))
		if !c.in.Scan() {
			return line, true
		}
		line += "\n" + c.in.Text()
	}
	return line, true
}

// openBrackets counts unclosed brackets, ignoring anything inside a
// string or a comment so a brace in a literal does not hold the prompt
// open forever.
func openBrackets(src string) int {
	depth := 0
	inStr, inRaw, inLine, inBlock := false, false, false, false
	var prev rune

	for i, r := range src {
		switch {
		case inLine:
			if r == '\n' {
				inLine = false
			}
		case inBlock:
			if prev == '*' && r == '/' {
				inBlock = false
			}
		case inStr:
			if r == '"' && prev != '\\' {
				inStr = false
			}
		case inRaw:
			if r == '`' {
				inRaw = false
			}
		case r == '/' && i+1 < len(src) && src[i+1] == '/':
			inLine = true
		case r == '/' && i+1 < len(src) && src[i+1] == '*':
			inBlock = true
		case r == '"':
			inStr = true
		case r == '`':
			inRaw = true
		case r == '{' || r == '(' || r == '[':
			depth++
		case r == '}' || r == ')' || r == ']':
			depth--
		}
		prev = r
	}
	if depth < 0 {
		return 0
	}
	return depth
}

// command handles the colon commands, and otherwise evaluates. It
// reports whether the console should exit.
func (c *console) command(line string) bool {
	trimmed := strings.TrimSpace(line)
	verb, rest, _ := strings.Cut(trimmed, " ")
	rest = strings.TrimSpace(rest)

	switch verb {
	case ":quit", ":q", ":exit":
		return true

	case ":help", ":h", ":?":
		c.help()

	case ":list", ":l":
		if len(c.lines) == 0 {
			fmt.Println("  nothing yet")
			break
		}
		for i, l := range c.lines {
			fmt.Printf("  %2d  %s\n", i+1, strings.ReplaceAll(l, "\n", "\n      "))
		}

	case ":clear", ":reset":
		c.lines = nil
		c.lastOut = ""
		fmt.Println("  session cleared")

	case ":undo":
		if len(c.lines) == 0 {
			fmt.Println("  nothing to undo")
			break
		}
		c.lines = c.lines[:len(c.lines)-1]
		// Rerun so lastOut matches the shortened session; otherwise the
		// next diff would be computed against output that no longer
		// exists and would reprint everything.
		out, _, _ := c.build(c.program())
		c.lastOut = out
		fmt.Println("  dropped the last line")

	case ":save":
		if rest == "" {
			fmt.Println("  usage: :save <file.qz>")
			break
		}
		if !strings.HasSuffix(rest, ".qz") {
			rest += ".qz"
		}
		if err := os.WriteFile(rest, []byte(c.program()), 0o644); err != nil {
			fmt.Printf("  could not save: %v\n", err)
			break
		}
		fmt.Printf("  saved %d line(s) to %s\n", len(c.lines), rest)

	case ":emit":
		_, goSrc, errs := c.build(c.program())
		if errs != "" {
			fmt.Print(errs)
			break
		}
		fmt.Println(goSrc)

	default:
		if strings.HasPrefix(trimmed, ":") {
			fmt.Printf("  no such command: %s  (try :help)\n", verb)
			break
		}
		c.eval(line)
	}
	return false
}

func (c *console) help() {
	fmt.Print(`
  :help            this
  :list            every line in the session
  :undo            drop the last line
  :clear           start over
  :save <file>     write the session out as a .qz program
  :emit            show the Go the session compiles to
  :quit            leave

  Brackets keep the prompt open, so functions and loops can be typed
  across lines. Everything you type is kept and rebuilt each time, so
  a side effect -- writing a file, sending a request -- happens again
  on every line. Pure code behaves exactly as you would expect.

`)
}

// program is the whole session as one Quartz source file.
func (c *console) program() string {
	return strings.Join(c.lines, "\n") + "\n"
}

// eval accepts a line only if the resulting program compiles, so one
// mistake does not poison the session.
func (c *console) eval(line string) {
	// An expression on its own should show its value, which is what a
	// console is for. Anything that is a statement is run as written.
	candidate := line
	if looksLikeExpression(line) {
		candidate = "print(" + line + ")"
	}

	trial := append(append([]string{}, c.lines...), candidate)
	out, _, errs := c.build(strings.Join(trial, "\n") + "\n")

	if errs != "" {
		// Retry as a statement: `x = 1` and `f()` are not expressions,
		// and neither is anything the wrapper made ungrammatical.
		if candidate != line {
			trial[len(trial)-1] = line
			out, _, errs = c.build(strings.Join(trial, "\n") + "\n")
			if errs == "" {
				candidate = line
			}
		}
		if errs != "" {
			fmt.Print(errs)
			return
		}
	}

	c.lines = append(c.lines, candidate)
	fmt.Print(newOutput(c.lastOut, out))
	c.lastOut = out
}

// looksLikeExpression is a deliberately shallow guess. Getting it wrong
// is cheap — eval retries the other way — so this only has to be right
// often enough to avoid a second compile on the common case.
func looksLikeExpression(line string) bool {
	t := strings.TrimSpace(line)
	if strings.Contains(t, "\n") {
		return false
	}
	for _, kw := range []string{
		"let ", "const ", "fn ", "struct ", "impl ", "import ", "pub ",
		"if ", "while ", "for ", "match ", "return", "break", "continue", "task ",
	} {
		if strings.HasPrefix(t, kw) {
			return false
		}
	}
	// An assignment is a statement. `==` and friends are not, so only a
	// bare `=` counts, and only outside a string.
	if hasBareAssign(t) {
		return false
	}
	return true
}

func hasBareAssign(s string) bool {
	inStr := false
	var prev rune
	for i, r := range s {
		if r == '"' && prev != '\\' {
			inStr = !inStr
		}
		if inStr || r != '=' {
			prev = r
			continue
		}
		next := byte(0)
		if i+1 < len(s) {
			next = s[i+1]
		}
		if next != '=' && prev != '=' && prev != '!' &&
			prev != '<' && prev != '>' && prev != '+' && prev != '-' &&
			prev != '*' && prev != '/' && prev != '%' {
			return true
		}
		prev = r
	}
	return false
}

// newOutput returns only what this run printed beyond the last one.
// When the previous output is not a prefix, the program is not
// deterministic and the honest thing is to show all of it rather than
// guess which part is new.
func newOutput(previous, current string) string {
	if strings.HasPrefix(current, previous) {
		return current[len(previous):]
	}
	return current
}

// build compiles and runs a whole session, returning its output, the
// generated Go, and any compiler errors as text.
//
// It shells out to this same executable rather than calling the
// pipeline in process. The compiler reports errors by writing to
// stderr and the driver exits on failure, so running it as a child is
// what keeps a broken line from taking the console down with it.
func (c *console) build(src string) (out, goSrc, errs string) {
	dir, err := os.MkdirTemp("", "quartz-console-")
	if err != nil {
		return "", "", fmt.Sprintf("  %v\n", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "session.qz")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		return "", "", fmt.Sprintf("  %v\n", err)
	}

	self, err := os.Executable()
	if err != nil {
		return "", "", fmt.Sprintf("  %v\n", err)
	}

	emit := exec.Command(self, "emit", path)
	emit.Env = quietEnv()
	emitted, emitErr := emit.Output()
	if emitErr != nil {
		if ee, ok := emitErr.(*exec.ExitError); ok {
			return "", "", tidyErrors(string(ee.Stderr))
		}
		return "", "", fmt.Sprintf("  %v\n", emitErr)
	}

	run := exec.Command(self, "run", path)
	run.Stdin = strings.NewReader("")
	run.Env = quietEnv()

	var stdout, stderr strings.Builder
	run.Stdout = &stdout
	run.Stderr = &stderr

	if runErr := run.Run(); runErr != nil {
		if _, ok := runErr.(*exec.ExitError); !ok {
			return "", "", fmt.Sprintf("  %v\n", runErr)
		}
		// A few things pass Quartz's checker and are still rejected by
		// Go — a constant `1 / 0` is caught there, not here. That is a
		// refused line, not output: accepting it would leave a session
		// that can never compile again.
		if strings.Contains(stderr.String(), "the Go backend rejected") {
			return "", "", tidyErrors(stderr.String())
		}
		// Otherwise it compiled and failed at runtime, which is output:
		// the crash handler has already explained it.
	}
	// Captured separately so a program writing to stderr — log.info, or
	// a traceback — is still diffed against the right stream.
	return stdout.String() + stderr.String(), string(emitted), ""
}

// quietEnv silences warnings in the child. They are about the session
// as a whole, so every rebuild would repeat them: being told "name is
// declared but never used" about a variable you are about to use on
// the next line is pure noise in a console.
func quietEnv() []string {
	return append(os.Environ(), "QUARTZ_QUIET=1")
}

// tidyErrors strips the parts of compiler output that only make sense
// for a file on disk: the temporary path, and the trailing count.
func tidyErrors(s string) string {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, "quartz: ") ||
			line == "# qzprog" || strings.HasPrefix(line, "# [") {
			continue
		}
		// A Go backend error names the temporary file it was compiling.
		// That path is an implementation detail of the console.
		if i := strings.Index(line, "session.qz:"); i > 0 {
			line = line[i:]
		}
		line = strings.TrimPrefix(line, "error: ")
		// "session.qz:12:5: message" -> "message", since neither the
		// file nor the line number of a synthesised session means
		// anything to the person typing.
		if i := strings.Index(line, "session.qz:"); i == 0 {
			if rest := strings.SplitN(line, ": ", 2); len(rest) == 2 {
				line = rest[1]
			}
		}
		kept = append(kept, "  "+paint(line, "31"))
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n") + "\n"
}
