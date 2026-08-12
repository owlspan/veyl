package main

import (
	"fmt"
	"os"
	"strings"
)

// The console banner.
//
// A snowy owl, drawn in plain 7-bit ASCII. Block-drawing characters
// would look better but they depend on the console's code page, and a
// banner that arrives as mojibake is worse than a plain one. Every
// character here renders the same everywhere.
//
// It is a raw string, so the backslashes are literal and need no
// escaping — which is also why the art must not contain a backtick.
//
// The speckling is what makes it read as a snowy owl rather than a
// generic bird, and the straight sides keep the body full instead of
// tapering the way a first attempt did.

const owlArt = `
                 __________
              .-'          '-.
            .'   .-.    .-.   '.
           /    ( o )  ( o )    \
          ;      '-'    '-'      ;
          |           v          |
          ;     '  .  '  .  '    ;
          |   .  '  .  '  .  '   |
          ;  '  .  '  .  '  .  ' ;
          |   .  '  .  '  .  '   |
           \ '  .  '  .  '  .  '/
            '.  .  '  .  '  .  .'
              '-.______________.-'
                   /        \
          ~~~~~~~~'          '~~~~~~~~
`

// banner returns the greeting the console opens with.
func banner() string {
	var b strings.Builder

	b.WriteString(paint(owlArt, "36"))
	b.WriteString("\n")
	b.WriteString(paint(fmt.Sprintf("  Quartz %s", Version), "1"))
	b.WriteString("  -  an interactive console\n\n")
	b.WriteString("  Type an expression to see its value, or a statement to run it.\n")
	b.WriteString("  " + paint(":help", "1") + " for the commands, " +
		paint(":quit", "1") + " to leave.\n\n")
	return b.String()
}

// paint wraps text in an ANSI attribute, unless colour would be noise.
// Decided once at startup on the same terms the term library uses: a
// real console, and NO_COLOR unset.
func paint(s, code string) string {
	if !consoleColour {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

var consoleColour = colourWanted()

func colourWanted() bool {
	if _, off := os.LookupEnv("NO_COLOR"); off {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
