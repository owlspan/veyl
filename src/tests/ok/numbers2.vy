// Hex, binary and digit grouping.

let mask = 0xFF
let lower = 0xff
let bits = 0b1010_1010
let big = 1_000_000
let frac = 1_234.5

print("hex {mask} {lower} bin {bits} grouped {big} {frac}")
print("same {0xFF == 255} {0b1010 == 10}")

// The point of them: masks that look like masks.
const READ = 0b0001
const WRITE = 0b0010
const EXEC = 0b0100

fn can(perms: int, bit: int) -> bool {
    return (perms & bit) != 0
}

let p = READ | EXEC
print("perms {p} read {can(p, READ)} write {can(p, WRITE)} exec {can(p, EXEC)}")
print("cleared {p & ~EXEC} all {READ | WRITE | EXEC}")

// Hex in shifts and comparisons.
print("byte {(0xABCD >> 8) & 0xFF} low {0xABCD & 0xFF}")
print("grouped float {1_0.5 + 0_1.5}")
