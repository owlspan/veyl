// ---- bitwise operators ----

let a = 12 // 1100
let b = 10 // 1010

print("and {a & b} or {a | b} xor {a ^ b}")
print("shl {a << 2} shr {a >> 2}")
print("not {~a}")

// C's precedence ladder: & binds looser than ==, so parens matter
const MASK = 4
print("masked {(a & MASK) == MASK}")

// compound forms
let flags = 0
flags |= 1
flags |= 4
print("set {flags}")
flags &= ~1
print("cleared {flags}")
flags ^= 12
print("toggled {flags}")
flags <<= 3
print("shifted {flags}")
flags >>= 2
print("back {flags}")
let m = 17
m %= 5
print("mod-assign {m}")

// a small bit-field helper, the sort of thing this is for
fn hasBit(value: int, bit: int) -> bool {
    return (value & (1 << bit)) != 0
}
fn setBit(value: int, bit: int) -> int {
    return value | (1 << bit)
}

let perms = 0
perms = setBit(perms, 0)
perms = setBit(perms, 3)
print("perms {perms} bit0 {hasBit(perms, 0)} bit1 {hasBit(perms, 1)} bit3 {hasBit(perms, 3)}")

// counting bits, exercising a loop over shifts
fn popcount(n: int) -> int {
    let count = 0
    let v = n
    while v != 0 {
        count += v & 1
        v >>= 1
    }
    return count
}
print("popcount 255 {popcount(255)} 1024 {popcount(1024)} 0 {popcount(0)}")

// hex-ish formatting built from shifts
fn toBinary(n: int, width: int) -> str {
    let out = ""
    for i in width - 1..=0 step -1 {
        if hasBit(n, i) {
            out += "1"
        } else {
            out += "0"
        }
    }
    return out
}
print("12 in binary {toBinary(12, 8)}")
print("170 in binary {toBinary(170, 8)}")

// ---- match ----

fn describe(code: int) -> str {
    match code {
        200 => return "ok"
        301, 302 => return "redirect"
        404, 410 => return "gone"
        else => return "unknown"
    }
}
print("{describe(200)} {describe(302)} {describe(404)} {describe(500)}")

// arms take a block, and do not fall through
for n in 1..=5 {
    match n % 3 {
        0 => {
            write("fizz ")
        }
        1 => write("one ")
        else => write("{n} ")
    }
}
print("")

// matching on strings and bools
let command = "stop"
match command {
    "go" => print("going")
    "stop" => print("stopping")
    else => print("no idea")
}

let ready = false
match ready {
    true => print("ready")
    false => print("not ready")
}

// a match with no else, where nothing matches, simply does nothing
match 99 {
    1 => print("never")
}
print("fell through cleanly")
