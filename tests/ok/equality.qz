// Lists, maps and structs compare by contents, matching what == means
// for everything else and what the printed form suggests.

print("lists {[1, 2, 3] == [1, 2, 3]} {[1, 2] == [2, 1]} {[1] != [1, 1]}")
let empty1: []int = []
let empty2: []int = []
print("empty {empty1 == empty2}")

let a = ["x", "y"]
let b = ["x", "y"]
print("separate but equal {a == b}")
push(a, "z")
print("changed {a == b} {a}")

// A map literal cannot start an interpolation, because {{ is the
// escape for a literal brace. Bind it first.
let m1 = {"k": 1}
let m2 = {"k": 1}
let m3 = {"k": 2}
print("maps {m1 == m2} {m1 == m3}")

let ordered = {"a": 1, "b": 2}
let reversed = {"b": 2, "a": 1}
print("key order does not matter {ordered == reversed}")

struct Point {
    x: int
    y: int
}
print("structs {Point{x: 1, y: 2} == Point{x: 1, y: 2}} {Point{x: 1} == Point{x: 2}}")

// Nested, which is where Go itself would refuse.
struct Bag {
    label: str
    items: []int
    meta: {str: str}
}
let one = Bag{label: "a", items: [1, 2], meta: {"k": "v"}}
let two = Bag{label: "a", items: [1, 2], meta: {"k": "v"}}
let three = Bag{label: "a", items: [1, 3], meta: {"k": "v"}}
print("nested {one == two} {one == three}")

print("lists of structs {[Point{x: 1}] == [Point{x: 1}]}")
let nested1 = {"n": [1, 2]}
let nested2 = {"n": [1, 2]}
print("map of lists {nested1 == nested2}")

// Nullables compare through the wrapper.
let some: ?int = 5
let same: ?int = 5
let none: ?int = nil
print("nullable {some == same} {some == none} {none == nil}")

// Scalars are unchanged.
print("scalars {1 == 1} {"a" == "a"} {true != false} {1.5 == 1.5}")
