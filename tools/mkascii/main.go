// Command mkascii turns a photograph into ASCII art.
//
//	go run ./tools/mkascii -in owl.jpg -w 78
//
// The first console banner was drawn by hand and looked it. This does
// what the detailed ASCII art people actually share does: sample the
// image, measure how bright each cell is, and pick a character of
// matching visual weight from a ramp.
//
// Three things separate a readable result from mush, and all three
// were found by looking at the output rather than by reasoning:
//
//   - Console characters are about twice as tall as they are wide, so a
//     cell has to cover twice as many rows as columns or the picture
//     comes out squashed.
//   - Photographs rarely use the full brightness range. Stretching the
//     range that is actually present is the difference between a
//     picture and a grey rectangle.
//   - Gamma. Perceived brightness is not linear in pixel value, and
//     without correcting for it the midtones all collapse onto the same
//     character.
package main

import (
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"strings"
)

// Ramps run from darkest to lightest. The long one has finer gradation
// and suits a large rendering; the short one holds up better when the
// output is small, because adjacent characters stay distinguishable.
const (
	// No backtick in either ramp. The output gets pasted into a Go raw
	// string, and a single backtick would end it early - a break that
	// depends on which image was converted, which is the kind of bug
	// that appears once and baffles everyone.
	rampLong  = "$@B%8&WM#*oahkbdpqwmZO0QLCJUYXzcvunxrjft/\\|()1{}[]?-_+~<>i!lI;:,\"^'. "
	rampShort = "@%#*+=-:. "
)

func main() {
	in := flag.String("in", "", "image to convert (png or jpeg)")
	width := flag.Int("w", 78, "output width in characters")
	invert := flag.Bool("invert", false, "swap dark and light")
	short := flag.Bool("short", false, "use the coarse ramp")
	gamma := flag.Float64("gamma", 1.0, "gamma correction; >1 brightens midtones")
	trim := flag.Bool("trim", true, "crop uniform borders before converting")
	cut := flag.Float64("cut", 0, "remove the background: colour tolerance, 0 disables (try 0.12)")
	flag.Parse()

	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: mkascii -in image.jpg [-w 78] [-invert] [-short]")
		os.Exit(64)
	}

	f, err := os.Open(*in)
	if err != nil {
		fail(err)
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		fail(fmt.Errorf("cannot read %s: %v", *in, err))
	}
	fmt.Fprintf(os.Stderr, "read %s (%s, %dx%d)\n",
		*in, format, img.Bounds().Dx(), img.Bounds().Dy())

	ramp := rampLong
	if *short {
		ramp = rampShort
	}
	fmt.Print(convert(img, *width, ramp, *invert, *gamma, *trim, *cut))
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "mkascii: %v\n", err)
	os.Exit(1)
}

// luminance is the perceptual brightness of a pixel, 0 to 1. The
// coefficients are the usual ITU-R BT.601 weights: the eye is far more
// sensitive to green than to blue, and averaging the channels instead
// makes blue regions read as much lighter than they look.
func luminance(img image.Image, x, y int) float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0
}

// rgb returns a pixel's colour, 0 to 1 per channel.
func rgb(img image.Image, x, y int) [3]float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	return [3]float64{float64(r) / 65535, float64(g) / 65535, float64(b) / 65535}
}

// dist is how far apart two colours are, as a plain Euclidean distance
// in RGB. Not perceptually uniform, but the job here is only to decide
// whether a cell is the same flat background as the corner, and for
// that it is entirely sufficient.
func dist(a, b [3]float64) float64 {
	d := 0.0
	for i := range a {
		d += (a[i] - b[i]) * (a[i] - b[i])
	}
	return math.Sqrt(d)
}

// markBackground flood-fills inwards from the edges, marking every cell
// close in colour to the border it came from.
//
// A brightness threshold cannot do this job. In the photograph that
// prompted it the owl is *lighter* than the sky behind it, so any cut
// that removes the sky removes the bird as well. What separates them is
// colour: the sky is saturated blue, the owl is not. Filling from the
// edge also means an object of any shape is kept, rather than assuming
// the subject sits in a rectangle.
func markBackground(colours [][][3]float64, tolerance float64) [][]bool {
	rows, cols := len(colours), len(colours[0])
	bg := make([][]bool, rows)
	for i := range bg {
		bg[i] = make([]bool, cols)
	}

	type point struct{ row, col int }
	var queue []point
	seed := func(r, c int) {
		if !bg[r][c] {
			bg[r][c] = true
			queue = append(queue, point{r, c})
		}
	}

	// Every edge cell starts the fill, each judged against the colour
	// where it began rather than one global sample, so a gradient sky
	// does not stop the fill halfway down.
	for c := 0; c < cols; c++ {
		seed(0, c)
		seed(rows-1, c)
	}
	for r := 0; r < rows; r++ {
		seed(r, 0)
		seed(r, cols-1)
	}

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		here := colours[p.row][p.col]
		for _, n := range []point{
			{p.row - 1, p.col}, {p.row + 1, p.col},
			{p.row, p.col - 1}, {p.row, p.col + 1},
		} {
			if n.row < 0 || n.row >= rows || n.col < 0 || n.col >= cols {
				continue
			}
			if bg[n.row][n.col] {
				continue
			}
			if dist(colours[n.row][n.col], here) <= tolerance {
				bg[n.row][n.col] = true
				queue = append(queue, n)
			}
		}
	}
	return bg
}

// convert samples the image into a grid and maps each cell to a
// character.
func convert(img image.Image, cols int, ramp string, invert bool, gamma float64, trim bool, cut float64) string {
	bounds := img.Bounds()
	if trim {
		bounds = trimBorders(img, bounds)
	}
	w, h := bounds.Dx(), bounds.Dy()
	if cols < 1 || w < 1 || h < 1 {
		return ""
	}

	// Two rows of pixels per column of pixels, because a character cell
	// is roughly twice as tall as it is wide.
	const cellAspect = 2.0
	rows := int(float64(cols) * float64(h) / float64(w) / cellAspect)
	if rows < 1 {
		rows = 1
	}

	// Average each cell, keeping its colour as well as its brightness:
	// the background test needs the colour, the ramp needs the
	// brightness.
	cells := make([][]float64, rows)
	colours := make([][][3]float64, rows)
	for row := 0; row < rows; row++ {
		cells[row] = make([]float64, cols)
		colours[row] = make([][3]float64, cols)
		for col := 0; col < cols; col++ {
			x0 := bounds.Min.X + col*w/cols
			x1 := bounds.Min.X + (col+1)*w/cols
			y0 := bounds.Min.Y + row*h/rows
			y1 := bounds.Min.Y + (row+1)*h/rows

			sum, n := 0.0, 0
			var csum [3]float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					sum += luminance(img, x, y)
					c := rgb(img, x, y)
					for i := range csum {
						csum[i] += c[i]
					}
					n++
				}
			}
			if n == 0 {
				continue
			}
			cells[row][col] = sum / float64(n)
			for i := range csum {
				colours[row][col][i] = csum[i] / float64(n)
			}
		}
	}

	var bg [][]bool
	if cut > 0 {
		bg = markBackground(colours, cut)
	}

	// Stretch the contrast across the subject only. Including the
	// background would spend most of the ramp on a flat field and leave
	// the subject crammed into a few characters.
	lo, hi := 1.0, 0.0
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			if bg != nil && bg[row][col] {
				continue
			}
			lo = math.Min(lo, cells[row][col])
			hi = math.Max(hi, cells[row][col])
		}
	}
	span := hi - lo
	if span < 1e-6 {
		span = 1 // a uniform image; leave it alone rather than dividing by zero
	}

	var b strings.Builder
	for row := 0; row < rows; row++ {
		line := make([]byte, cols)
		for col := 0; col < cols; col++ {
			if bg != nil && bg[row][col] {
				line[col] = ' '
				continue
			}
			v := (cells[row][col] - lo) / span
			if gamma != 1.0 {
				v = math.Pow(v, 1.0/gamma)
			}
			if invert {
				v = 1 - v
			}
			// The ramp runs dark to light, so a bright cell takes a
			// character from the far end.
			i := int(v * float64(len(ramp)-1))
			if i < 0 {
				i = 0
			}
			if i >= len(ramp) {
				i = len(ramp) - 1
			}
			line[col] = ramp[i]
		}
		b.WriteString(strings.TrimRight(string(line), " "))
		b.WriteByte('\n')
	}
	return b.String()
}

// trimBorders crops the uniform margin many photographs carry - the
// letterboxing on a screenshot, or a flat band of sky. Without this the
// subject is squeezed into the middle of the output while the edges
// spend characters saying nothing.
func trimBorders(img image.Image, b image.Rectangle) image.Rectangle {
	const flat = 0.02 // a row this uniform carries no detail

	rowVaries := func(y int) bool {
		lo, hi := 1.0, 0.0
		for x := b.Min.X; x < b.Max.X; x++ {
			v := luminance(img, x, y)
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
		return hi-lo > flat
	}
	colVaries := func(x int) bool {
		lo, hi := 1.0, 0.0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			v := luminance(img, x, y)
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
		return hi-lo > flat
	}

	top, bottom := b.Min.Y, b.Max.Y-1
	for top < bottom && !rowVaries(top) {
		top++
	}
	for bottom > top && !rowVaries(bottom) {
		bottom--
	}
	left, right := b.Min.X, b.Max.X-1
	for left < right && !colVaries(left) {
		left++
	}
	for right > left && !colVaries(right) {
		right--
	}

	cropped := image.Rect(left, top, right+1, bottom+1)
	if cropped.Dx() < 8 || cropped.Dy() < 8 {
		return b // the whole image was flat; better to keep it
	}
	return cropped
}
