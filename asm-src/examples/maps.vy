// Maps.
//
// The entries are kept sorted by key, so printing and iteration come
// out in the Go backend's order without a sort step. Everything here is
// compared against that backend byte for byte.

// Literal keys deliberately out of order - printing must sort them.
let m: {str: int} = {"pear": 3, "apple": 1, "fig": 2}
print(m)
print(len(m))
print(m["apple"])
print(m["pear"])

// A missing key reads the zero value rather than failing.
print(m["durian"])

// Insert. The new key has to land in sorted position, not at the end.
m["banana"] = 9
print(m)
print(len(m))

// Overwrite an existing key: the value changes, the length does not.
m["apple"] = 100
print(m)
print(len(m))

// Enough inserts to force the blocks to grow more than once.
let big: {int: int} = {}
for i in 1..20 {
    big[i] = i * i
}
print(len(big))
print(big[7])
print(big[20])
print(big[21])

// Integer keys sort as numbers, and inserting backwards still sorts.
let back: {int: str} = {}
for i in 5..1 step -1 {
    back[i] = "n{i}"
}
print(back)

// An empty map prints as {} and has length zero.
let none: {str: int} = {}
print(none)
print(len(none))

// Values of other kinds.
let flags: {str: bool} = {"on": true, "off": false}
print(flags)

let sizes: {str: float} = {"half": 0.5, "quarter": 0.25}
print(sizes)

let names: {int: str} = {2: "two", 1: "one"}
print(names)
print(names[1])
print(names[3])

// Maps work through a function, and length survives the call.
fn total(scores: {str: int}, k: str) -> int {
    return scores[k] + len(scores)
}
print(total(m, "apple"))
