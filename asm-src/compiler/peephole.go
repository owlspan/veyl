package main

// The assembly peephole: three tiny rewrites over the text Emit
// produced, applied before either linking route sees it, so the byte
// encoder and GNU as stay comparable references for each other.
//
//   - A store to a frame slot immediately followed by reloading that
//     same slot into rax loses the reload. The store already left the
//     value there.
//   - A line identical to the line before it loses its copy. Data
//     movement only: two calls in a row are two calls, and a repeated
//     compare would tempt rewrites that depend on flags.
//   - mov rax, small positive becomes mov eax, which encodes five
//     bytes shorter. Zeroing the high half is what the wide form was
//     already holding, and mov writes no flags, so compares and jumps
//     around it are untouched.
//
// Nothing here thinks about the program. The IR passes do that; this
// file sweeps up the shapes their output leaves behind.

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	peepStore = regexp.MustCompile(`^    mov qword ptr \[rbp-(\d+)\], rax$`)
	peepLoad  = regexp.MustCompile(`^    mov rax, qword ptr \[rbp-(\d+)\]$`)
	peepImm   = regexp.MustCompile(`^    mov rax, (\d+)$`)
)

func peephole(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))

	stored := "" // the slot the last kept line stored to, if any
	for _, ln := range lines {
		if m := peepLoad.FindStringSubmatch(ln); m != nil && stored == m[1] {
			continue // the value is in rax already
		}

		if m := peepImm.FindStringSubmatch(ln); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil && v <= 0x7fffffff {
				ln = "    mov eax, " + m[1]
			}
		}

		if dupableAsm(ln) && len(out) > 0 && out[len(out)-1] == ln {
			continue
		}

		out = append(out, ln)
		if m := peepStore.FindStringSubmatch(ln); m != nil {
			stored = m[1]
		} else {
			stored = ""
		}
	}
	return strings.Join(out, "\n")
}

// dupableAsm reports whether running a line twice back to back does
// what running it once does. Pure data movement qualifies; anything
// with an effect or a flag reading does not.
func dupableAsm(ln string) bool {
	for _, p := range []string{"    mov ", "    lea ", "    movsd "} {
		if strings.HasPrefix(ln, p) {
			return true
		}
	}
	return false
}
