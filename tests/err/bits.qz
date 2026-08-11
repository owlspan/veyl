// bitwise operators are int-only
let f = 1.5
print(f & 1)
print(1 | f)
print("text" ^ 2)
print(f << 2)
print(~f)
print(~"text")

// the C precedence wart, which should be explained rather than just rejected
let flags = 12
print(flags & 1 == 1)

// compound forms follow the same rule
let g = 2.5
g &= 1
g <<= 2
let n = 3
n |= 1.5

// match needs a comparable subject
let xs = [1, 2]
match xs {
    else => print("no")
}

// arms must agree with the subject's type
match 5 {
    "five" => print("no")
    else => print("no")
}

// duplicate arms are dead code
match 1 {
    1 => print("one")
    1 => print("also one")
    else => print("other")
}
