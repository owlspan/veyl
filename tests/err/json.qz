struct Point {
    x: float
    y: float
}

// decode has nothing to tell it what to build
let a = json.decode("{{}}")
print(a)

// the annotation and the use disagree
let b: Point = json.decode("{{}}")
print(b.z)

// wrong argument types
print(json.get(1, 2))
print(json.decodeOr(3, 4))

// a path reader used on a number
let n = 5
print(json.get(n, "x"))
