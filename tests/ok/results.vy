// A function that can fail returns T! - a value, or a reason.
fn parsePort(text: str) -> int! {
    if !isInt(text) {
        return fail("{text} is not a number")
    }
    let n = toInt(text)
    if n < 1 || n > 65535 {
        return fail("port {n} is out of range")
    }
    return n
}

// Inspecting a result without unwrapping it.
let good = parsePort("8080")
let bad = parsePort("http")
print("good ok {isOk(good)} bad ok {isOk(bad)}")
print("bad failed {failed(bad)} reason: {errorOf(bad)}")
print("good has no error {errorOf(good) == ""}")

// Taking the value, with a fallback for the failure case.
print("value {valueOr(good, 0)} fallback {valueOr(bad, 80)}")

// must() takes the value or stops the program. Explicit, unlike a
// library call that dies on its own.
print("must {must(good)}")

// `?` unwraps, or returns the failure from the enclosing function.
fn addressFor(host: str, port: str) -> str! {
    let n = parsePort(port)?
    return "{host}:{n}"
}
print("{valueOr(addressFor("localhost", "443"), "-")}")
print("{errorOf(addressFor("localhost", "nope"))}")

// `?` chains: the first failure wins and the rest never runs.
fn sumPorts(a: str, b: str) -> int! {
    return parsePort(a)? + parsePort(b)?
}
print("sum {valueOr(sumPorts("80", "443"), -1)}")
print("first bad {errorOf(sumPorts("x", "443"))}")
print("second bad {errorOf(sumPorts("80", "y"))}")

// `?` works mid-expression, and in a condition.
fn isPrivileged(port: str) -> bool! {
    if parsePort(port)? < 1024 {
        return true
    }
    return false
}
print("80 privileged {valueOr(isPrivileged("80"), false)}")
print("8080 privileged {valueOr(isPrivileged("8080"), false)}")
print("bad {errorOf(isPrivileged("nope"))}")

// Results carrying other shapes.
struct Server {
    host: str
    port: int
}

fn parseServer(text: str) -> Server! {
    let parts = split(text, ":")
    if len(parts) != 2 {
        return fail("expected host:port, got {text}")
    }
    let port = parsePort(parts[1])?
    return Server{host: parts[0], port: port}
}

let s = parseServer("example.com:443")
if isOk(s) {
    let server = must(s)
    print("parsed {server.host} on {server.port}")
}
print("bad server {errorOf(parseServer("example.com"))}")
print("bad port {errorOf(parseServer("example.com:99999"))}")

// A result holding a list.
fn firstWords(line: str) -> []str! {
    if trim(line) == "" {
        return fail("empty line")
    }
    return split(line, " ")
}
print("words {valueOr(firstWords("a b c"), [])}")
print("empty {errorOf(firstWords("  "))}")

// Results and nullables compose.
fn maybePort(text: str) -> ?int! {
    if text == "" {
        return nil
    }
    let n = parsePort(text)?
    return n
}
let absent = maybePort("")
print("absent is ok {isOk(absent)} and nil {must(absent) == nil}")
let present = maybePort("22")
let p = must(present)
if p != nil {
    print("present {p}")
}
print("invalid {errorOf(maybePort("zz"))}")
