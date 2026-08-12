package main

import "strings"

// The bytes library.
//
// Before this, binary data was carried in a `str`. The conversion
// itself is lossless — a Go string is a byte sequence and is not
// required to be valid UTF-8 — so the danger is subtler than it looks,
// and worth writing down because the obvious explanation is wrong.
//
// What breaks is every operation that assumes text. Measured on four
// bytes that are not valid UTF-8:
//
//	len, trim         unchanged
//	upper             corrupted
//	json.encode       every invalid byte becomes �
//
// So the data survives right up until something touches it, and then
// it is quietly wrong. A separate type is what makes that impossible
// rather than merely unlikely, and it is what files, sockets, hashing
// and any future FFI all actually want.
//
// There is no separate `byte` type. Indexing gives an `int` from 0 to
// 255: a language that already has int has somewhere to put a small
// number, and a second numeric type would infect every arithmetic rule
// in the checker to buy nothing.

var bytesHelperDefs = map[string]helperDef{
	"bytes": {
		code: `func __bytesFromHex(s string) __Res[[]byte] {
	s = strings.TrimSpace(s)
	b, err := hex.DecodeString(s)
	if err != nil {
		return __fail[[]byte]("not hexadecimal: " + s)
	}
	return __ok(b)
}

func __bytesFromBase64(s string) __Res[[]byte] {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return __fail[[]byte]("not valid base64")
	}
	return __ok(b)
}

// Out-of-range slicing is clamped rather than fatal. Reading past the
// end of a buffer is ordinary when parsing, and a truncated result is
// more useful than a crash.
func __bytesSlice(b []byte, start, end int) []byte {
	if start < 0 {
		start = 0
	}
	if end > len(b) {
		end = len(b)
	}
	if start >= end {
		return []byte{}
	}
	return append([]byte(nil), b[start:end]...)
}

// A copy, not a view. Quartz assignment copies, and returning a slice
// that aliases its argument would let a later write reach backwards
// into something the caller thought was theirs.
func __bytesConcat(parts ...[]byte) []byte {
	out := []byte{}
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func __bytesOfList(ns []int) __Res[[]byte] {
	out := make([]byte, len(ns))
	for i, n := range ns {
		if n < 0 || n > 255 {
			return __fail[[]byte](fmt.Sprintf("%d does not fit in a byte", n))
		}
		out[i] = byte(n)
	}
	return __ok(out)
}

func __bytesToList(b []byte) []int {
	out := make([]int, len(b))
	for i, v := range b {
		out[i] = int(v)
	}
	return out
}

func __bytesFill(n, value int) __Res[[]byte] {
	if n < 0 {
		return __fail[[]byte]("cannot make a negative number of bytes")
	}
	if value < 0 || value > 255 {
		return __fail[[]byte](fmt.Sprintf("%d does not fit in a byte", value))
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(value)
	}
	return __ok(out)
}

func __bytesFind(haystack, needle []byte) int {
	return bytes.Index(haystack, needle)
}

// Integers are read and written little-endian by default, which is
// what x86 and ARM both use. The BE variants exist because network
// protocols went the other way.
func __bytesGetInt(b []byte, offset, size int, big bool) __Res[int] {
	if size != 1 && size != 2 && size != 4 && size != 8 {
		return __fail[int](fmt.Sprintf("a size of %d is not 1, 2, 4 or 8", size))
	}
	if offset < 0 || offset+size > len(b) {
		return __fail[int](fmt.Sprintf(
			"reading %d bytes at %d runs past the end of %d bytes", size, offset, len(b)))
	}
	var n uint64
	for i := 0; i < size; i++ {
		if big {
			n = n<<8 | uint64(b[offset+i])
		} else {
			n |= uint64(b[offset+i]) << (8 * i)
		}
	}
	return __ok(int(n))
}

func __bytesPutInt(n, size int, big bool) __Res[[]byte] {
	if size != 1 && size != 2 && size != 4 && size != 8 {
		return __fail[[]byte](fmt.Sprintf("a size of %d is not 1, 2, 4 or 8", size))
	}
	out := make([]byte, size)
	u := uint64(n)
	for i := 0; i < size; i++ {
		shift := 8 * i
		if big {
			shift = 8 * (size - 1 - i)
		}
		out[i] = byte(u >> shift)
	}
	return __ok(out)
}`,
		imports: []string{"bytes", "encoding/base64", "encoding/hex", "fmt", "strings"},
		deps:    []string{"result"},
	},

	"bytesFile": {
		code: `func __readBytes(path string) __Res[[]byte] {
	b, err := os.ReadFile(path)
	if err != nil {
		return __fail[[]byte](err.Error())
	}
	return __ok(b)
}

func __writeBytes(path string, b []byte) __Res[__Unit] {
	return __try(os.WriteFile(path, b, 0o644))
}`,
		imports: []string{"os"},
		deps:    []string{"result", "try"},
	},

	"bytesHash": {
		code: `func __hashBytes(b []byte, algorithm string) __Res[string] {
	switch algorithm {
	case "md5":
		return __ok(hex.EncodeToString(__md5sum(b)))
	case "sha1":
		return __ok(hex.EncodeToString(__sha1sum(b)))
	case "sha256":
		sum := sha256.Sum256(b)
		return __ok(hex.EncodeToString(sum[:]))
	case "sha512":
		sum := sha512.Sum512(b)
		return __ok(hex.EncodeToString(sum[:]))
	}
	return __fail[string](
		"no such algorithm: " + algorithm + " (md5, sha1, sha256, sha512)")
}

func __md5sum(b []byte) []byte  { s := md5.Sum(b); return s[:] }
func __sha1sum(b []byte) []byte { s := sha1.Sum(b); return s[:] }`,
		imports: []string{
			"crypto/md5", "crypto/sha1", "crypto/sha256", "crypto/sha512",
			"encoding/hex",
		},
		deps: []string{"result"},
	},
}

var bytesBuiltins map[string]builtin

func buildBytesBuiltins() {
	call := func(goFn string, params []*Type, ret *Type, helper string) builtin {
		return builtin{
			emit: func(a []string) string {
				return goFn + "(" + strings.Join(a, ", ") + ")"
			},
			params: params, ret: ret,
			minArgs: len(params), maxArgs: len(params),
			helpers: []string{helper},
		}
	}
	// getInt/putInt differ only in endianness, so the two spellings
	// share one helper rather than duplicating the shifting.
	getInt := func(big string) builtin {
		return builtin{
			emit: func(a []string) string {
				return "__bytesGetInt(" + a[0] + ", " + a[1] + ", " + a[2] + ", " + big + ")"
			},
			params: []*Type{Bytes, Int, Int}, ret: ResultOf(Int), minArgs: 3, maxArgs: 3,
			helpers: []string{"bytes"},
		}
	}
	putInt := func(big string) builtin {
		return builtin{
			emit: func(a []string) string {
				return "__bytesPutInt(" + a[0] + ", " + a[1] + ", " + big + ")"
			},
			params: []*Type{Int, Int}, ret: ResultOf(Bytes), minArgs: 2, maxArgs: 2,
			helpers: []string{"bytes"},
		}
	}

	bytesBuiltins = map[string]builtin{
		// ---- making and unmaking ----

		"bytes.of": {
			emit:   func(a []string) string { return "[]byte(" + a[0] + ")" },
			params: []*Type{Str}, ret: Bytes, minArgs: 1, maxArgs: 1,
		},
		"bytes.str": {
			// Not every byte sequence is text. Go substitutes the
			// replacement character rather than failing, which is the
			// same thing every other language does here.
			emit:   func(a []string) string { return "string(" + a[0] + ")" },
			params: []*Type{Bytes}, ret: Str, minArgs: 1, maxArgs: 1,
		},
		"bytes.list":       call("__bytesToList", []*Type{Bytes}, ListOf(Int), "bytes"),
		"bytes.ofList":     call("__bytesOfList", []*Type{ListOf(Int)}, ResultOf(Bytes), "bytes"),
		"bytes.fill":       call("__bytesFill", []*Type{Int, Int}, ResultOf(Bytes), "bytes"),
		"bytes.hex":        {emit: func(a []string) string { return "hex.EncodeToString(" + a[0] + ")" }, params: []*Type{Bytes}, ret: Str, minArgs: 1, maxArgs: 1, imports: []string{"encoding/hex"}},
		"bytes.fromHex":    call("__bytesFromHex", []*Type{Str}, ResultOf(Bytes), "bytes"),
		"bytes.base64":     {emit: func(a []string) string { return "base64.StdEncoding.EncodeToString(" + a[0] + ")" }, params: []*Type{Bytes}, ret: Str, minArgs: 1, maxArgs: 1, imports: []string{"encoding/base64"}},
		"bytes.fromBase64": call("__bytesFromBase64", []*Type{Str}, ResultOf(Bytes), "bytes"),

		// ---- working with them ----

		"bytes.slice": call("__bytesSlice", []*Type{Bytes, Int, Int}, Bytes, "bytes"),
		"bytes.find":  call("__bytesFind", []*Type{Bytes, Bytes}, Int, "bytes"),
		"bytes.concat": {
			emit: func(a []string) string {
				return "__bytesConcat(" + strings.Join(a, ", ") + ")"
			},
			params: []*Type{Bytes}, rest: Bytes, ret: Bytes, minArgs: 1, maxArgs: -1,
			helpers: []string{"bytes"},
		},

		// ---- numbers ----

		"bytes.getInt":   getInt("false"),
		"bytes.getIntBE": getInt("true"),
		"bytes.putInt":   putInt("false"),
		"bytes.putIntBE": putInt("true"),

		// ---- files and hashing ----

		"bytes.read":  call("__readBytes", []*Type{Str}, ResultOf(Bytes), "bytesFile"),
		"bytes.write": call("__writeBytes", []*Type{Str, Bytes}, ResultOf(Void), "bytesFile"),
		"bytes.hash":  call("__hashBytes", []*Type{Bytes, Str}, ResultOf(Str), "bytesHash"),
	}
}

func registerBytes() {
	buildBytesBuiltins()
	registerNamespace("bytes")
	for k, v := range bytesHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range bytesBuiltins {
		builtins[k] = v
	}
}
