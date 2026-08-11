package main

// The hash and csv libraries.
//
// hash covers digests and the two encodings that always come up with
// them. Every digest returns lower-case hex, because that is what every
// other tool prints and a checksum you cannot compare by eye is not
// much use.
//
// Decoding is the fallible half — the input comes from outside — so
// those return `T!` while the encoders do not.

var dataHelperDefs = map[string]helperDef{
	"hashDigests": {
		code: `func __md5(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func __sha1(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func __sha256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func __sha512(s string) string {
	sum := sha512.Sum512([]byte(s))
	return hex.EncodeToString(sum[:])
}`,
		imports: []string{"crypto/md5", "crypto/sha1", "crypto/sha256", "crypto/sha512", "encoding/hex"},
	},

	"hashFile": {
		// Streamed rather than read whole, so hashing something large
		// does not need it all in memory at once.
		code: `func __sha256File(path string) __Res[string] {
	f, err := os.Open(path)
	if err != nil {
		return __fail[string](__why("read", path, err))
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return __fail[string](__why("read", path, err))
	}
	return __ok(hex.EncodeToString(h.Sum(nil)))
}`,
		imports: []string{"crypto/sha256", "encoding/hex", "io", "os"},
		deps:    []string{"qzWhy", "result"},
	},

	"hashEncodings": {
		code: `func __fromBase64(s string) __Res[string] {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return __fail[string]("not valid base64: " + err.Error())
	}
	return __ok(string(b))
}

func __fromHex(s string) __Res[string] {
	b, err := hex.DecodeString(s)
	if err != nil {
		return __fail[string]("not valid hex: " + err.Error())
	}
	return __ok(string(b))
}`,
		imports: []string{"encoding/base64", "encoding/hex"},
		deps:    []string{"result"},
	},

	"csvHelpers": {
		code: `func __csvParse(text string) __Res[[][]string] {
	r := csv.NewReader(strings.NewReader(text))
	// Rows are allowed to have different lengths. Real files are ragged,
	// and refusing to read one is less useful than handing it over.
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return __fail[[][]string]("cannot read csv: " + err.Error())
	}
	if rows == nil {
		rows = [][]string{}
	}
	return __ok(rows)
}

func __csvWrite(rows [][]string) string {
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	w.WriteAll(rows)
	w.Flush()
	return sb.String()
}`,
		imports: []string{"encoding/csv", "strings"},
		deps:    []string{"result"},
	},
}

var dataBuiltins map[string]builtin

func buildDataBuiltins() {
	one := func(goFn string, ret *Type, helper string) builtin {
		return builtin{
			emit:   func(a []string) string { return goFn + "(" + a[0] + ")" },
			params: []*Type{Str}, ret: ret, minArgs: 1, maxArgs: 1,
			helpers: []string{helper},
		}
	}

	dataBuiltins = map[string]builtin{
		// ---- digests ----

		"hash.md5":    one("__md5", Str, "hashDigests"),
		"hash.sha1":   one("__sha1", Str, "hashDigests"),
		"hash.sha256": one("__sha256", Str, "hashDigests"),
		"hash.sha512": one("__sha512", Str, "hashDigests"),

		"hash.crc32": {
			emit:   func(a []string) string { return "int(crc32.ChecksumIEEE([]byte(" + a[0] + ")))" },
			params: []*Type{Str}, ret: Int, minArgs: 1, maxArgs: 1,
			imports: []string{"hash/crc32"},
		},

		"hash.file": one("__sha256File", ResultOf(Str), "hashFile"),

		// ---- encodings ----

		"hash.base64": {
			emit: func(a []string) string {
				return "base64.StdEncoding.EncodeToString([]byte(" + a[0] + "))"
			},
			params: []*Type{Str}, ret: Str, minArgs: 1, maxArgs: 1,
			imports: []string{"encoding/base64"},
		},
		"hash.fromBase64": one("__fromBase64", ResultOf(Str), "hashEncodings"),

		"hash.hex": {
			emit:   func(a []string) string { return "hex.EncodeToString([]byte(" + a[0] + "))" },
			params: []*Type{Str}, ret: Str, minArgs: 1, maxArgs: 1,
			imports: []string{"encoding/hex"},
		},
		"hash.fromHex": one("__fromHex", ResultOf(Str), "hashEncodings"),

		// ---- csv ----

		"csv.parse": one("__csvParse", ResultOf(ListOf(ListOf(Str))), "csvHelpers"),

		"csv.write": {
			emit:   func(a []string) string { return "__csvWrite(" + a[0] + ")" },
			params: []*Type{ListOf(ListOf(Str))}, ret: Str, minArgs: 1, maxArgs: 1,
			helpers: []string{"csvHelpers"},
		},

		"csv.read": {
			emit: func(a []string) string {
				return "func() __Res[[][]string] { r := __readFile(" + a[0] + "); " +
					"if r.e != \"\" { return __fail[[][]string](r.e) }; return __csvParse(r.v) }()"
			},
			params: []*Type{Str}, ret: ResultOf(ListOf(ListOf(Str))), minArgs: 1, maxArgs: 1,
			helpers: []string{"csvHelpers", "readFile"},
		},

		"csv.save": {
			emit: func(a []string) string {
				return "__writeFile(" + a[0] + ", __csvWrite(" + a[1] + "))"
			},
			params: []*Type{Str, ListOf(ListOf(Str))}, ret: Bool, minArgs: 2, maxArgs: 2,
			helpers: []string{"csvHelpers", "writeFile"},
		},
	}
}

func registerData() {
	buildDataBuiltins()
	registerNamespace("hash", "csv")
	for k, v := range dataHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range dataBuiltins {
		builtins[k] = v
	}
}
