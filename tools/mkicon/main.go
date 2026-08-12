// Command mkicon draws the Quartz logo and writes it out as a Windows
// icon, plus a COFF resource object so the compiled quartz.exe carries
// it in Explorer and the taskbar.
//
//	go run ./tools/mkicon
//
// The mark is drawn in code rather than stored as art on purpose. An
// icon needs to exist at nine sizes from 16 to 256 pixels, and a shape
// scaled down from one big drawing turns to mush at 16. Drawing each
// size from the same geometry keeps the facet edges landing on whole
// pixels.
//
// The shape is a quartz crystal reduced to three flat faces: a light
// one catching the light down the middle, a mid tone on the left, a
// dark one on the right. No gradients, no bevels, no glow. At 16 pixels
// all that survives is the silhouette and the tonal split, which is
// exactly what should survive.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// The three faces. Cool blue-violet: quartz is not grey, and a real
// hue survives downscaling better than near-white does.
var (
	faceLight = color.NRGBA{0xAF, 0xC0, 0xF0, 0xFF}
	faceMid   = color.NRGBA{0x6B, 0x7F, 0xC7, 0xFF}
	faceDark  = color.NRGBA{0x3B, 0x4E, 0x8C, 0xFF}
)

// pt is a point in a 0..1 square, so the geometry is resolution-free.
type pt struct{ X, Y float64 }

// crystal returns the three faces of the mark, left to right.
//
// The silhouette is a double-terminated quartz point: a hexagonal
// prism coming to a pyramid at both ends. The first attempt had a flat
// base and a pointed top, which at any size reads unmistakably as a
// house. A point at both ends cannot be read as a building, and it is
// what quartz actually does when it grows without a substrate.
func crystal() [][]pt {
	const (
		topApexY = 0.030 // tip of the upper termination
		topShY   = 0.290 // where that termination meets the vertical sides
		botShY   = 0.720 // and the lower one
		botApexY = 0.970
		leftX    = 0.225
		rightX   = 0.775
		innerL   = 0.415 // the two vertical facet edges
		innerR   = 0.585
		centerX  = 0.500
	)

	// Where an inner facet edge crosses a sloping termination. Solved
	// on the line from the apex out to the shoulder, so the end facets
	// meet the side facets exactly and no seam opens up between them.
	cross := func(x, shoulderX, apexY, shoulderY float64) float64 {
		t := (centerX - x) / (centerX - shoulderX)
		return apexY + t*(shoulderY-apexY)
	}
	topL := pt{innerL, cross(innerL, leftX, topApexY, topShY)}
	topR := pt{innerR, cross(innerR, rightX, topApexY, topShY)}
	botL := pt{innerL, cross(innerL, leftX, botApexY, botShY)}
	botR := pt{innerR, cross(innerR, rightX, botApexY, botShY)}

	// Nine facets: three columns, each cut into a termination cap at
	// the top, the prism body, and a cap at the bottom. Cutting at the
	// shoulders is what stops the mark reading as three flat stripes -
	// the caps catch different light from the body, which is the whole
	// reason a real crystal looks like a crystal.
	return [][]pt{
		// upper terminations, left to right
		{{leftX, topShY}, topL, {innerL, topShY}},
		{topL, {centerX, topApexY}, topR, {innerR, topShY}, {innerL, topShY}},
		{topR, {rightX, topShY}, {innerR, topShY}},

		// prism body
		{{leftX, topShY}, {innerL, topShY}, {innerL, botShY}, {leftX, botShY}},
		{{innerL, topShY}, {innerR, topShY}, {innerR, botShY}, {innerL, botShY}},
		{{innerR, topShY}, {rightX, topShY}, {rightX, botShY}, {innerR, botShY}},

		// lower terminations
		{{leftX, botShY}, {innerL, botShY}, botL},
		{{innerL, botShY}, {innerR, botShY}, botR, {centerX, botApexY}, botL},
		{{innerR, botShY}, {rightX, botShY}, botR},
	}
}

// shade brightens or darkens a face. Flat tones only - a gradient here
// would be the exact thing this mark is trying not to be.
func shade(c color.NRGBA, f float64) color.NRGBA {
	clamp := func(v float64) uint8 {
		if v > 255 {
			return 255
		}
		if v < 0 {
			return 0
		}
		return uint8(v)
	}
	return color.NRGBA{
		clamp(float64(c.R) * f),
		clamp(float64(c.G) * f),
		clamp(float64(c.B) * f),
		c.A,
	}
}

// faceTones pairs each facet with its colour, in the order crystal()
// returns them: caps are lit, the body is the base tone, the underside
// falls away.
func faceTones() []color.NRGBA {
	const (
		lit   = 1.14
		body  = 1.00
		under = 0.80
	)
	return []color.NRGBA{
		shade(faceMid, lit), shade(faceLight, lit), shade(faceDark, lit),
		shade(faceMid, body), shade(faceLight, body), shade(faceDark, body),
		shade(faceMid, under), shade(faceLight, under), shade(faceDark, under),
	}
}

// render draws the mark at size x size pixels. It rasterises at
// ss times the resolution and box-filters down, which is where the
// antialiasing comes from; there is no edge smoothing anywhere else.
func render(size int) *image.NRGBA {
	const ss = 8
	big := size * ss
	hi := image.NewNRGBA(image.Rect(0, 0, big, big))

	tones := faceTones()
	for i, face := range crystal() {
		fillPolygon(hi, face, tones[i], big)
	}

	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a int
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					c := hi.NRGBAAt(x*ss+dx, y*ss+dy)
					// Weight colour by coverage so transparent pixels
					// do not drag the edges towards black.
					r += int(c.R) * int(c.A)
					g += int(c.G) * int(c.A)
					b += int(c.B) * int(c.A)
					a += int(c.A)
				}
			}
			if a == 0 {
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{
				uint8(r / a), uint8(g / a), uint8(b / a),
				uint8(a / (ss * ss)),
			})
		}
	}
	return out
}

// fillPolygon scanline-fills a convex polygon given in 0..1 space.
func fillPolygon(dst *image.NRGBA, poly []pt, c color.NRGBA, size int) {
	minY, maxY := 1.0, 0.0
	for _, p := range poly {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	y0, y1 := int(minY*float64(size)), int(maxY*float64(size))+1
	if y0 < 0 {
		y0 = 0
	}
	if y1 > size {
		y1 = size
	}

	for y := y0; y < y1; y++ {
		sy := (float64(y) + 0.5) / float64(size)
		var xs []float64
		for i := range poly {
			a, b := poly[i], poly[(i+1)%len(poly)]
			if (a.Y <= sy) == (b.Y <= sy) {
				continue // edge does not cross this scanline
			}
			t := (sy - a.Y) / (b.Y - a.Y)
			xs = append(xs, a.X+t*(b.X-a.X))
		}
		if len(xs) < 2 {
			continue
		}
		lo, hi := xs[0], xs[0]
		for _, x := range xs {
			if x < lo {
				lo = x
			}
			if x > hi {
				hi = x
			}
		}
		for x := int(lo * float64(size)); x <= int(hi*float64(size)); x++ {
			if x < 0 || x >= size {
				continue
			}
			cx := (float64(x) + 0.5) / float64(size)
			if cx >= lo && cx <= hi {
				dst.SetNRGBA(x, y, c)
			}
		}
	}
}

// iconSizes are the sizes Windows asks for. 256 is the one Explorer
// shows in Large Icons view; 16 is the one in the title bar.
var iconSizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}

type iconImage struct {
	size int
	png  []byte
}

func renderAll() ([]iconImage, error) {
	var out []iconImage
	for _, s := range iconSizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, render(s)); err != nil {
			return nil, err
		}
		out = append(out, iconImage{s, buf.Bytes()})
	}
	return out, nil
}

// writeICO packs the images into a .ico. Every entry is a PNG, which
// Windows has accepted inside icons since Vista and keeps the file
// about a tenth the size of the old BMP encoding.
func writeICO(path string, imgs []iconImage) error {
	var buf bytes.Buffer
	w16 := func(v uint16) { binary.Write(&buf, binary.LittleEndian, v) }
	w32 := func(v uint32) { binary.Write(&buf, binary.LittleEndian, v) }

	w16(0) // reserved
	w16(1) // type: icon
	w16(uint16(len(imgs)))

	offset := 6 + 16*len(imgs)
	for _, im := range imgs {
		buf.WriteByte(byteSize(im.size))
		buf.WriteByte(byteSize(im.size))
		buf.WriteByte(0) // palette size: not paletted
		buf.WriteByte(0) // reserved
		w16(1)           // colour planes
		w16(32)          // bits per pixel
		w32(uint32(len(im.png)))
		w32(uint32(offset))
		offset += len(im.png)
	}
	for _, im := range imgs {
		buf.Write(im.png)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// byteSize encodes a dimension the way an icon directory wants it:
// one byte, with 256 written as zero.
func byteSize(n int) byte {
	if n >= 256 {
		return 0
	}
	return byte(n)
}

func main() {
	repo, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	iconDir := filepath.Join(repo, "icons")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		fail(err)
	}

	imgs, err := renderAll()
	if err != nil {
		fail(err)
	}

	icoPath := filepath.Join(iconDir, "quartz.ico")
	if err := writeICO(icoPath, imgs); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s (%d sizes)\n", icoPath, len(imgs))

	// A standalone PNG, for a README or a website later.
	var big bytes.Buffer
	if err := png.Encode(&big, render(512)); err != nil {
		fail(err)
	}
	pngPath := filepath.Join(iconDir, "quartz-512.png")
	if err := os.WriteFile(pngPath, big.Bytes(), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s\n", pngPath)

	// The resource object the Go linker picks up automatically.
	syso := filepath.Join(repo, "rsrc_windows_amd64.syso")
	if err := writeSyso(syso, imgs); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s\n", syso)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "mkicon: %v\n", err)
	os.Exit(1)
}
