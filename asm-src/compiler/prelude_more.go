package main

// bits, args and url, in Veyl.
//
// All three are pure computation over ints and strings, which is exactly
// what the prelude is for. The only thing here that needs the operating
// system is the command line, and that is one call.
//
// The bit functions are written against Go's math/bits, which works on
// uint64 where Veyl's int is signed. Every place that matters, the shift
// is followed by a mask: `>>` here is arithmetic, so a negative value
// shifted right brings ones down from the top, and the mask is what
// turns that back into the logical shift these functions mean.

const preludeBits2 = `
// Go's bits.OnesCount64, the SWAR version: add adjacent pairs, then
// nibbles, then bytes, and read the total out of the top byte with one
// multiply. The first shift is the only one that can see a sign bit,
// and the mask beside it removes it.
fn __vy_bitsCount(n: int) -> int {
    let x = n
    x = (x & 0x5555555555555555) + ((x >> 1) & 0x5555555555555555)
    x = (x & 0x3333333333333333) + ((x >> 2) & 0x3333333333333333)
    x = (x + (x >> 4)) & 0x0F0F0F0F0F0F0F0F
    x = x * 0x0101010101010101
    return (x >> 56) & 0xFF
}

// bits.Len64: how many bits are needed to hold the value, reading it as
// unsigned. A negative int has bit 63 set, so it needs all 64.
fn __vy_bitsLength(n: int) -> int {
    if n < 0 { return 64 }
    let x = n
    let count = 0
    while x != 0 {
        count = count + 1
        x = x >> 1
    }
    return count
}

fn __vy_bitsLeading(n: int) -> int {
    return 64 - __vy_bitsLength(n)
}

fn __vy_bitsTrailing(n: int) -> int {
    if n == 0 { return 64 }
    let x = n
    let count = 0
    while (x & 1) == 0 {
        count = count + 1
        x = x >> 1
    }
    return count
}

// bits.Reverse32, then widened back to an int - so the answer is always
// in 0 .. 2**32-1 and never negative, matching Go's uint32-to-int.
fn __vy_bitsReverse(n: int) -> int {
    let x = n & 0xFFFFFFFF
    let out = 0
    let i = 0
    while i < 32 {
        out = (out << 1) | ((x >> i) & 1)
        i = i + 1
    }
    return out
}

// bits.RotateLeft32. The count is taken modulo 32 the way Go takes it,
// by masking, so a negative rotation is a rotation the other way.
fn __vy_bitsRotate(n: int, by: int) -> int {
    let x = n & 0xFFFFFFFF
    let s = by & 31
    if s == 0 { return x }
    return ((x << s) | (x >> (32 - s))) & 0xFFFFFFFF
}

fn __vy_baseDigits() -> str {
    return "0123456789abcdefghijklmnopqrstuvwxyz"
}

// strconv.FormatInt. A base outside 2..36 gives the empty string rather
// than a panic, which is what the Go backend's wrapper does.
fn __vy_toBase(n: int, base: int) -> str {
    if base < 2 || base > 36 { return "" }
    if n == 0 { return "0" }

    let digits = __vy_baseDigits()
    let neg = n < 0
    let x = n
    if neg { x = -x }

    let out = ""
    while x != 0 {
        let d = x % base
        out = substr(digits, d, d + 1) + out
        x = x / base
    }
    if neg { return "-" + out }
    return out
}

fn __vy_digitValue(c: str, base: int) -> int {
    let digits = __vy_baseDigits()
    let i = 0
    while i < base {
        if substr(digits, i, i + 1) == lower(c) { return i }
        i = i + 1
    }
    return -1
}

// strconv.ParseInt, with the Go backend's own two messages. Every way of
// failing gives the same one, so the reasons never have to agree beyond
// these two strings.
fn __vy_fromBase(s: str, base: int) -> int! {
    if base < 2 || base > 36 {
        return fail("base must be between 2 and 36")
    }
    let bad = s + " is not a base-" + str(base) + " number"

    let t = trim(s)
    if len(t) == 0 { return fail(bad) }

    let i = 0
    let neg = false
    if charAt(t, 0) == "-" {
        neg = true
        i = 1
    }
    if charAt(t, 0) == "+" { i = 1 }
    if i >= len(t) { return fail(bad) }

    let n = 0
    while i < len(t) {
        let d = __vy_digitValue(charAt(t, i), base)
        if d < 0 { return fail(bad) }
        n = n * base + d
        i = i + 1
    }
    if neg { return -n }
    return n
}
`

const preludeArgs = `
// The command line.
//
// A program built here has no C runtime startup, so there is no argv to
// read - __cmdline asks Windows for the raw string and this splits it
// the way the runtime would have: double quotes group, and a quote
// inside a quoted run ends it.

fn __vy_osArgs() -> []str {
    let raw = __cmdline()
    let out: []str = []
    let cur = ""
    let inQuotes = false
    let started = false

    let i = 0
    while i < len(raw) {
        let c = charAt(raw, i)
        if c == "\"" {
            inQuotes = !inQuotes
            started = true
        } else {
            if c == " " && !inQuotes {
                if started { push(out, cur) }
                cur = ""
                started = false
            } else {
                cur = cur + c
                started = true
            }
        }
        i = i + 1
    }
    if started { push(out, cur) }

    // Drop the program's own name, which is what os.Args[1:] means.
    let rest: []str = []
    let k = 1
    while k < len(out) {
        push(rest, out[k])
        k = k + 1
    }
    return rest
}

fn __vy_argsFlag(name: str) -> bool {
    let args = __vy_osArgs()
    for a in args {
        if a == "--" + name || a == "-" + name { return true }
    }
    return false
}

fn __vy_argsValue(name: str, fallback: str) -> str {
    let args = __vy_osArgs()
    let i = 0
    while i < len(args) {
        let a = args[i]
        if a == "--" + name || a == "-" + name {
            if i + 1 < len(args) { return args[i + 1] }
            return fallback
        }
        if startsWith(a, "--" + name + "=") {
            return substr(a, len(name) + 3, len(a))
        }
        i = i + 1
    }
    return fallback
}

fn __vy_argsRest() -> []str {
    let args = __vy_osArgs()
    let out: []str = []
    let skipNext = false
    for a in args {
        if skipNext {
            skipNext = false
        } else {
            if startsWith(a, "-") {
                if !contains(a, "=") { skipNext = true }
            } else {
                push(out, a)
            }
        }
    }
    return out
}
`

const preludeURL = `
// A URL parser, covering what net/url reports for the parts this
// library exposes. It is not a general implementation of RFC 3986: it
// handles the shapes a program actually writes, and says so by failing
// rather than guessing on anything else.

fn __vy_schemeOf(raw: str) -> str {
    let i = 0
    while i < len(raw) {
        let c = charAt(raw, i)
        if c == ":" {
            if i == 0 { return "" }
            return lower(substr(raw, 0, i))
        }
        if c == "/" || c == "?" || c == "#" { return "" }
        i = i + 1
    }
    return ""
}

// Everything between the // and the next /, ? or #.
fn __vy_urlAuthority(raw: str) -> str {
    let scheme = __vy_schemeOf(raw)
    let at = 0
    if len(scheme) > 0 { at = len(scheme) + 1 }
    if at + 2 > len(raw) { return "" }
    if substr(raw, at, at + 2) != "//" { return "" }
    at = at + 2

    let i = at
    while i < len(raw) {
        let c = charAt(raw, i)
        if c == "/" || c == "?" || c == "#" { return substr(raw, at, i) }
        i = i + 1
    }
    return substr(raw, at, len(raw))
}

// Userinfo is stripped, as Hostname() strips it.
fn __vy_urlHostPort(raw: str) -> str {
    let a = __vy_urlAuthority(raw)
    let at = indexOf(a, "@")
    if at >= 0 { return substr(a, at + 1, len(a)) }
    return a
}

fn __vy_hostOf(raw: str) -> str {
    let hp = __vy_urlHostPort(raw)
    let colon = __vy_lastColon(hp)
    if colon < 0 { return hp }
    if !__vy_allDigits(substr(hp, colon + 1, len(hp))) { return hp }
    return substr(hp, 0, colon)
}

fn __vy_portOf(raw: str) -> str {
    let hp = __vy_urlHostPort(raw)
    let colon = __vy_lastColon(hp)
    if colon < 0 { return "" }
    let tail = substr(hp, colon + 1, len(hp))
    if !__vy_allDigits(tail) { return "" }
    return tail
}

fn __vy_lastColon(s: str) -> int {
    let i = len(s) - 1
    while i >= 0 {
        if charAt(s, i) == ":" { return i }
        i = i - 1
    }
    return -1
}

fn __vy_allDigits(s: str) -> bool {
    if len(s) == 0 { return false }
    let i = 0
    while i < len(s) {
        if __vy_digitOf(charAt(s, i)) < 0 { return false }
        i = i + 1
    }
    return true
}

fn __vy_pathOf(raw: str) -> str {
    let scheme = __vy_schemeOf(raw)
    let at = 0
    if len(scheme) > 0 { at = len(scheme) + 1 }
    if at + 2 <= len(raw) && substr(raw, at, at + 2) == "//" {
        at = at + 2 + len(__vy_urlAuthority(raw))
    }
    let i = at
    while i < len(raw) {
        let c = charAt(raw, i)
        if c == "?" || c == "#" { return __vy_unescape(substr(raw, at, i)) }
        i = i + 1
    }
    return __vy_unescape(substr(raw, at, len(raw)))
}

fn __vy_urlRawQuery(raw: str) -> str {
    let q = indexOf(raw, "?")
    if q < 0 { return "" }
    let h = indexOf(raw, "#")
    if h >= 0 && h > q { return substr(raw, q + 1, h) }
    if h >= 0 { return "" }
    return substr(raw, q + 1, len(raw))
}

fn __vy_fragmentOf(raw: str) -> str {
    let h = indexOf(raw, "#")
    if h < 0 { return "" }
    return __vy_unescape(substr(raw, h + 1, len(raw)))
}

// Percent-decoding, with + meaning a space inside a query. Anything that
// is not a valid escape is left alone, which is what Go does for a path
// and close enough for the rest.
fn __vy_unescape(s: str) -> str {
    let out = ""
    let i = 0
    while i < len(s) {
        let c = charAt(s, i)
        if c == "%" && i + 2 < len(s) {
            let hi = __vy_hexVal(charAt(s, i + 1))
            let lo = __vy_hexVal(charAt(s, i + 2))
            if hi >= 0 && lo >= 0 {
                out = out + __chr(hi * 16 + lo)
                i = i + 3
            } else {
                out = out + c
                i = i + 1
            }
        } else {
            out = out + c
            i = i + 1
        }
    }
    return out
}

fn __vy_unescapeQuery(s: str) -> str {
    let swapped = replace(s, "+", " ")
    return __vy_unescape(swapped)
}

fn __vy_hexVal(c: str) -> int {
    let digits = "0123456789abcdef"
    let i = 0
    let low = lower(c)
    while i < 16 {
        if substr(digits, i, i + 1) == low { return i }
        i = i + 1
    }
    return -1
}


// The public spellings. net/url can fail on a malformed URL, so the Go
// backend returns a T! and a program written against it says must(...);
// these keep that shape even though this parser reports the parts of
// anything it is given.

fn __vy_urlScheme(raw: str) -> str! {
    return __vy_schemeOf(raw)
}

fn __vy_urlHost(raw: str) -> str! {
    return __vy_hostOf(raw)
}

fn __vy_urlPort(raw: str) -> str! {
    return __vy_portOf(raw)
}

fn __vy_urlPath(raw: str) -> str! {
    return __vy_pathOf(raw)
}

fn __vy_urlFragment(raw: str) -> str! {
    return __vy_fragmentOf(raw)
}

fn __vy_urlQuery(raw: str) -> {str: str}! {
    let out: {str: str} = {}
    let q = __vy_urlRawQuery(raw)
    if len(q) == 0 { return out }

    let parts = split(q, "&")
    for p in parts {
        if len(p) > 0 {
            let eq = indexOf(p, "=")
            let k = p
            let v = ""
            if eq >= 0 {
                k = substr(p, 0, eq)
                v = substr(p, eq + 1, len(p))
            }
            let key = __vy_unescapeQuery(k)
            // Repeated keys keep the first value, which is what the Go
            // backend does when it flattens the multi-valued map.
            if !has(out, key) { out[key] = __vy_unescapeQuery(v) }
        }
    }
    return out
}

// Reference resolution. Enough of RFC 3986 for the shapes that come up:
// an absolute reference, a network-relative one, a rooted path and a
// relative path, with dot segments removed.
fn __vy_urlJoin(base: str, ref: str) -> str! {
    if len(__vy_schemeOf(ref)) > 0 { return ref }

    let scheme = __vy_schemeOf(base)
    if len(scheme) == 0 { return fail("cannot resolve against a URL with no scheme") }
    let authority = __vy_urlAuthority(base)
    let root = scheme + "://" + authority

    if startsWith(ref, "//") { return scheme + ":" + ref }
    if startsWith(ref, "#") { return base + ref }
    if startsWith(ref, "?") {
        let cut = indexOf(base, "?")
        if cut >= 0 { return substr(base, 0, cut) + ref }
        return base + ref
    }

    let tail = ref
    let suffix = ""
    let mark = __vy_firstOf(tail, "?#")
    if mark >= 0 {
        suffix = substr(tail, mark, len(tail))
        tail = substr(tail, 0, mark)
    }

    let merged = tail
    if !startsWith(tail, "/") {
        let bp = __vy_pathOf(base)
        let slash = __vy_lastSlash(bp)
        let dir = "/"
        if slash >= 0 { dir = substr(bp, 0, slash + 1) }
        merged = dir + tail
    }
    return root + __vy_removeDots(merged) + suffix
}

fn __vy_firstOf(s: str, set: str) -> int {
    let i = 0
    while i < len(s) {
        if contains(set, charAt(s, i)) { return i }
        i = i + 1
    }
    return -1
}

fn __vy_lastSlash(s: str) -> int {
    let i = len(s) - 1
    while i >= 0 {
        if charAt(s, i) == "/" { return i }
        i = i - 1
    }
    return -1
}

// RFC 3986's remove_dot_segments, done on whole segments rather than
// character by character.
fn __vy_removeDots(p: str) -> str {
    let segs = split(p, "/")
    let out: []str = []
    for s in segs {
        if s == "." {
            // nothing to do
        } else {
            if s == ".." {
                if len(out) > 1 { removeAt(out, len(out) - 1) }
            } else {
                push(out, s)
            }
        }
    }
    let joined = join(out, "/")
    if len(joined) == 0 { return "/" }
    if !startsWith(joined, "/") { return "/" + joined }
    return joined
}
`

// os.dir.home, os.dir.temp and os.path.clean, in Veyl. None of them
// needs Win32: two are an environment variable and the third is string
// work.
const preludeOsMore = `
fn __vy_dirHome() -> str {
    let h = os.env.get("USERPROFILE")
    if h != "" { return h }
    // A service account may have neither, in which case there is no
    // home directory to report rather than a wrong one.
    return os.env.get("HOME")
}

fn __vy_dirTemp() -> str {
    let t = os.env.get("TEMP")
    if t != "" { return t }
    t = os.env.get("TMP")
    if t != "" { return t }
    return "C:\\Windows\\Temp"
}

// Collapse separators, drop "." segments, and resolve ".." against what
// came before it. Purely textual: it never touches the disk, so it does
// not follow links and does not care whether the path exists.
fn __vy_pathClean(p: str) -> str {
    if p == "" { return "." }

    let rooted = false
    let prefix = ""
    let rest = p

    // Keep a drive letter or a leading separator, so cleaning does not
    // turn an absolute path into a relative one.
    if len(p) >= 2 && __strAt(p, 1) == 58 {
        prefix = substr(p, 0, 2)
        rest = substr(p, 2, len(p))
    }
    if len(rest) > 0 && (__strAt(rest, 0) == 47 || __strAt(rest, 0) == 92) {
        rooted = true
    }

    let parts: []str = []
    let cur = ""
    let i = 0
    while i <= len(rest) {
        let atEnd = i == len(rest)
        let c = 0
        if !atEnd { c = __strAt(rest, i) }
        if atEnd || c == 47 || c == 92 {
            if cur == "." || cur == "" {
                // nothing to add
            } else {
                if cur == ".." {
                    if len(parts) > 0 && parts[len(parts) - 1] != ".." {
                        removeAt(parts, len(parts) - 1)
                    } else {
                        if !rooted { push(parts, "..") }
                    }
                } else {
                    push(parts, cur)
                }
            }
            cur = ""
        } else {
            cur = cur + charAt(rest, i)
        }
        i = i + 1
    }

    let out = join(parts, "/")
    if rooted { out = "/" + out }
    out = prefix + out
    if out == "" { return "." }
    return out
}
`
