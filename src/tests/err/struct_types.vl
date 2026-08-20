// Type-level mistakes, caught by the checker.

struct Point {
    x: float
    y: float
}

struct Holder {
    thing: Missing
}

// a struct cannot contain itself by value
struct Loop {
    next: Loop
}

struct Counter {
    n: int
}

impl Point {
    fn length(self) -> float {
        return sqrt(self.x * self.x + self.y * self.y)
    }
}

impl Counter {
    fn add(self, by: int) {
        self.n += by
    }
}

let p = Point{x: 1.0, y: 2.0}

// fields that are not fields
print(p.z)
let q = Point{x: 1.0, z: 3.0}

// a field given twice
let r = Point{x: 1.0, x: 2.0}

// wrong field type
let s = Point{x: "left", y: 2.0}

// method and field confusion, in both directions
print(p.length)
print(p.x())

// a method that does not exist
p.rotate(90)

// wrong argument count and type
let c = Counter{n: 0}
c.add()
c.add(1, 2)
c.add("three")

// fields on something that has none
let n = 5
print(n.field)

// assigning the wrong type to a field
p.x = "over there"
