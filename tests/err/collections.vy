// an empty literal cannot be inferred
let a = []
let b = {}

// elements must agree
let c = [1, "two", 3]
let d = {"x": 1, "y": "two"}

// map keys are limited to str and int
let e = {true: 1}

// indexing rules
let nums = [1, 2, 3]
print(nums["first"])
let m = {"a": 1}
print(m[0])
print("hello"[0])

// annotation must match
let f: []str = [1, 2]

// wrong element type going in
push(nums, "four")

// push needs somewhere to put it
push([1, 2], 3)

// these want a list
print(sum(["a", "b"]))
print(sort([[1], [2]]))
print(first("hello"))
print(keys(nums))
print(has(nums, 1))

// iterating the wrong things
for x in 5 {
    print(x)
}
for x in m {
    print(x)
}
for x in "word" {
    print(x)
}
