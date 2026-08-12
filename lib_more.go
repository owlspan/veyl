package main

import "strings"

// The zip, url, args and bits libraries.
//
// These are the gaps that show up when someone writes a real program:
// unpacking a download, taking a URL apart, reading a command line, and
// the bit fiddling that a systems language is expected to have.

var moreHelperDefs = map[string]helperDef{

	"zip": {
		code: `func __zipMake(dest string, paths []string) __Res[bool] {
	f, err := os.Create(dest)
	if err != nil {
		return __fail[bool](err.Error())
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			w.Close()
			return __fail[bool](err.Error())
		}
		if info.IsDir() {
			// Directories go in whole, with paths relative to the
			// directory itself so unpacking does not recreate the
			// machine's whole tree.
			base := filepath.Dir(p)
			err = filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil || fi.IsDir() {
					return err
				}
				rel, err := filepath.Rel(base, path)
				if err != nil {
					return err
				}
				return __zipAdd(w, path, filepath.ToSlash(rel))
			})
		} else {
			err = __zipAdd(w, p, filepath.Base(p))
		}
		if err != nil {
			w.Close()
			return __fail[bool](err.Error())
		}
	}
	if err := w.Close(); err != nil {
		return __fail[bool](err.Error())
	}
	return __ok(true)
}

func __zipAdd(w *zip.Writer, path, name string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := w.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}

func __zipList(src string) __Res[[]string] {
	r, err := zip.OpenReader(src)
	if err != nil {
		return __fail[[]string](err.Error())
	}
	defer r.Close()
	out := []string{}
	for _, f := range r.File {
		out = append(out, f.Name)
	}
	return __ok(out)
}

func __zipExtract(src, dest string) __Res[int] {
	r, err := zip.OpenReader(src)
	if err != nil {
		return __fail[int](err.Error())
	}
	defer r.Close()

	count := 0
	for _, f := range r.File {
		// An archive can name any path it likes, including one that
		// climbs out of the destination. Same rule as the package
		// manager's tarballs, and for the same reason.
		clean := filepath.Clean(filepath.FromSlash(f.Name))
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return __fail[int]("archive contains an unsafe path: " + f.Name)
		}
		target := filepath.Join(dest, clean)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return __fail[int](err.Error())
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return __fail[int](err.Error())
		}
		rc, err := f.Open()
		if err != nil {
			return __fail[int](err.Error())
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return __fail[int](err.Error())
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return __fail[int](err.Error())
		}
		count++
	}
	return __ok(count)
}`,
		imports: []string{"archive/zip", "io", "os", "path/filepath", "strings"},
		deps:    []string{"result"},
	},

	"url": {
		code: `func __urlPart(raw, part string) __Res[string] {
	u, err := url.Parse(raw)
	if err != nil {
		return __fail[string](err.Error())
	}
	switch part {
	case "scheme":
		return __ok(u.Scheme)
	case "host":
		return __ok(u.Hostname())
	case "port":
		return __ok(u.Port())
	case "path":
		return __ok(u.Path)
	case "query":
		return __ok(u.RawQuery)
	case "fragment":
		return __ok(u.Fragment)
	}
	return __fail[string]("no such part of a URL: " + part)
}

func __urlQuery(raw string) __Res[map[string]string] {
	u, err := url.Parse(raw)
	if err != nil {
		return __fail[map[string]string](err.Error())
	}
	out := map[string]string{}
	// Repeated keys keep the first value. A map cannot hold both, and
	// silently keeping the last would be the more surprising choice.
	for k, vs := range u.Query() {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return __ok(out)
}

func __urlBuild(base string, params map[string]string) string {
	if len(params) == 0 {
		return base
	}
	// Sorted, so the same map always builds the same URL - which
	// matters for caching and for tests.
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	q := url.Values{}
	for _, k := range keys {
		q.Set(k, params[k])
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + q.Encode()
}

func __urlJoin(base, ref string) __Res[string] {
	b, err := url.Parse(base)
	if err != nil {
		return __fail[string](err.Error())
	}
	r, err := b.Parse(ref)
	if err != nil {
		return __fail[string](err.Error())
	}
	return __ok(r.String())
}`,
		imports: []string{"net/url", "sort", "strings"},
		deps:    []string{"result"},
	},

	// Command-line flags. Deliberately tiny: no declaration step, no
	// usage generation, no types. A Quartz program asks whether a flag
	// is there and what it was set to, and the answers come straight
	// from os.Args.
	"args": {
		code: `func __argsFlag(name string) bool {
	for _, a := range os.Args[1:] {
		if a == "-"+name || a == "--"+name {
			return true
		}
		if strings.HasPrefix(a, "--"+name+"=") || strings.HasPrefix(a, "-"+name+"=") {
			return true
		}
	}
	return false
}

// Both --name=value and --name value are accepted, because people write
// both and there is no reason to have an opinion about it.
func __argsValue(name, fallback string) string {
	rest := os.Args[1:]
	for i, a := range rest {
		for _, prefix := range []string{"--" + name + "=", "-" + name + "="} {
			if strings.HasPrefix(a, prefix) {
				return strings.TrimPrefix(a, prefix)
			}
		}
		if a == "-"+name || a == "--"+name {
			if i+1 < len(rest) && !strings.HasPrefix(rest[i+1], "-") {
				return rest[i+1]
			}
			return fallback
		}
	}
	return fallback
}

// Everything that is not a flag or a flag's value: the file names.
func __argsRest() []string {
	out := []string{}
	rest := os.Args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
			continue
		}
		if !strings.Contains(a, "=") && i+1 < len(rest) && !strings.HasPrefix(rest[i+1], "-") {
			i++ // skip the value belonging to this flag
		}
	}
	return out
}`,
		imports: []string{"os", "strings"},
	},

	"bits": {
		code: `func __rotl(x, n int) int {
	return int(bits.RotateLeft32(uint32(x), n))
}

func __toBase(n, base int) string {
	if base < 2 || base > 36 {
		return ""
	}
	return strconv.FormatInt(int64(n), base)
}

func __fromBase(s string, base int) __Res[int] {
	if base < 2 || base > 36 {
		return __fail[int]("base must be between 2 and 36")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), base, 64)
	if err != nil {
		return __fail[int](s + " is not a base-" + strconv.Itoa(base) + " number")
	}
	return __ok(int(n))
}`,
		imports: []string{"math/bits", "strconv", "strings"},
		deps:    []string{"result"},
	},
}

var moreBuiltins map[string]builtin

func buildMoreBuiltins() {
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
	part := func(name string) builtin {
		return builtin{
			emit: func(a []string) string {
				return `__urlPart(` + a[0] + `, "` + name + `")`
			},
			params: []*Type{Str}, ret: ResultOf(Str), minArgs: 1, maxArgs: 1,
			helpers: []string{"url"},
		}
	}
	// The plain bit operators are already in the language; these are the
	// ones that need a function.
	bit := func(goExpr func(a []string) string) builtin {
		return builtin{
			emit:   goExpr,
			params: []*Type{Int}, ret: Int, minArgs: 1, maxArgs: 1,
			imports: []string{"math/bits"},
		}
	}

	moreBuiltins = map[string]builtin{

		// ---- zip ----

		"zip.make":    call("__zipMake", []*Type{Str, ListOf(Str)}, ResultOf(Bool), "zip"),
		"zip.list":    call("__zipList", []*Type{Str}, ResultOf(ListOf(Str)), "zip"),
		"zip.extract": call("__zipExtract", []*Type{Str, Str}, ResultOf(Int), "zip"),

		// ---- url ----

		"url.scheme":   part("scheme"),
		"url.host":     part("host"),
		"url.port":     part("port"),
		"url.path":     part("path"),
		"url.fragment": part("fragment"),
		"url.query":    call("__urlQuery", []*Type{Str}, ResultOf(MapOf(Str, Str)), "url"),
		"url.build":    call("__urlBuild", []*Type{Str, MapOf(Str, Str)}, Str, "url"),
		"url.join":     call("__urlJoin", []*Type{Str, Str}, ResultOf(Str), "url"),

		// ---- args ----

		"args.flag": call("__argsFlag", []*Type{Str}, Bool, "args"),
		"args.value": {
			emit: func(a []string) string {
				if len(a) == 1 {
					return "__argsValue(" + a[0] + `, "")`
				}
				return "__argsValue(" + a[0] + ", " + a[1] + ")"
			},
			params: []*Type{Str, Str}, ret: Str, minArgs: 1, maxArgs: 2,
			helpers: []string{"args"},
		},
		"args.rest": {
			emit:    func(a []string) string { return "__argsRest()" },
			ret:     ListOf(Str),
			helpers: []string{"args"},
		},

		// ---- bits ----

		"bits.count": bit(func(a []string) string {
			return "bits.OnesCount64(uint64(" + a[0] + "))"
		}),
		"bits.leading": bit(func(a []string) string {
			return "bits.LeadingZeros64(uint64(" + a[0] + "))"
		}),
		"bits.trailing": bit(func(a []string) string {
			return "bits.TrailingZeros64(uint64(" + a[0] + "))"
		}),
		"bits.length": bit(func(a []string) string {
			return "bits.Len64(uint64(" + a[0] + "))"
		}),
		"bits.reverse": bit(func(a []string) string {
			return "int(bits.Reverse32(uint32(" + a[0] + ")))"
		}),
		"bits.rotate":   call("__rotl", []*Type{Int, Int}, Int, "bits"),
		"bits.toBase":   call("__toBase", []*Type{Int, Int}, Str, "bits"),
		"bits.fromBase": call("__fromBase", []*Type{Str, Int}, ResultOf(Int), "bits"),
	}
}

func registerMore() {
	buildMoreBuiltins()
	registerNamespace("zip", "url", "args", "bits")
	for k, v := range moreHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range moreBuiltins {
		builtins[k] = v
	}
}
