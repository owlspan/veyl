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
// escaping - which is also why neither ramp in mkascii contains a
// backtick, and why it asserts before writing here.
//
// This is generated from a photograph rather than drawn by hand. The
// hand-drawn one it replaces was, accurately, described as ugly: at
// this size a person picking characters cannot compete with sampling
// real luminance. Regenerate it with
//
//	go run ./tools/mkascii -in owl.jpg -w 54 -cut 0.05 -invert > art.txt
//
// -invert is what suits a console: on a dark background a dense
// character reads as bright, which is the opposite of ink on paper.
// -cut removes the sky by colour rather than brightness, because a
// snowy owl is *lighter* than the sky behind it and no brightness
// threshold can separate the two.

const owlArt = `
                              wUvnYcvLQq
                            mQZdbaWWWW@dd
                           wkMhXCM8B@pm%$%
                          mqa&&hdkko&B@@@@M
                         pwp*##okdO0M%@$$@W
                       qQOC#owLnxrJ0MB$$$@*
                    qLcvvYkowCnpurnzQqWB$@*
                  0cv/zujOQvUUmdYucvOqb*&8W
                qz/fujJpbzufmwOJJOuvmwaMW8#
               Lr|z1\LUYucujvCUmJpYYZ0L*Wa
              J1(]|YUtQ}rzc)zOLodpbCQpbo#q
             C(||Xn\-f/1ufjc0QmhmdmYmQa#qd
            dtcCUJ|\Xu/1fjYOqwkb0mZO00akZ
            cffJX/x1tjvJxCQ0pbbpOZCkmdqZ
            vvr)|frXUCLYQLJ0mLb*OOqZmZOq
           Yu}uf(fxYUYJL0XQQOw0wm00QLQwb
         qj\(|/|t\uvYJCQCQQJL0L0ZZOLCQq
        u1fuunx/jfYJJCZCmYQOQcmwqLcUQd
       }]jQYzUUzXYUzzUznc|1fr\cqp({Xw
     dtQrJLCCCL         z! ':,;!i!]Uqp
     Z|UOQUCOZ       wbbq!:Ii>+<<uJ mmCQQJZm
    rf/ruuQ         dd                     wJQZZZq
    wLJfUZ          m           W                p
`

// banner returns the greeting the console opens with.
func banner() string {
	var b strings.Builder

	b.WriteString(paint(owlArt, "36"))
	b.WriteString("\n")
	b.WriteString(paint(fmt.Sprintf("  Veyl %s", Version), "1"))
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
