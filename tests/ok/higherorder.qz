let nums = [5, 3, 8, 1, 9, 2]
let words = ["pear", "fig", "banana", "kiwi"]

// map changes each element, and may change the type.
print("doubled {map(nums, fn(n: int) -> int { return n * 2 })}")
print("labelled {map(nums, fn(n: int) -> str { return "#{n}" })}")

// filter keeps the ones that answer yes.
print("big {filter(nums, fn(n: int) -> bool { return n > 4 })}")
print("short {filter(words, fn(w: str) -> bool { return len(w) <= 4 })}")

// reduce folds the list into one value, of whatever type you start with.
print("total {reduce(nums, 0, fn(acc: int, n: int) -> int { return acc + n })}")
print("initials {reduce(words, "", fn(acc: str, w: str) -> str { return acc + charAt(w, 0) })}")

// sortBy takes a comparison and leaves the original alone.
print("by size {sortBy(words, fn(a: str, b: str) -> bool { return len(a) < len(b) })}")
print("descending {sortBy(nums, fn(a: int, b: int) -> bool { return a > b })}")
print("original untouched {nums}")

// any and all.
print("any big {any(nums, fn(n: int) -> bool { return n > 8 })}")
print("all positive {all(nums, fn(n: int) -> bool { return n > 0 })}")
print("all big {all(nums, fn(n: int) -> bool { return n > 4 })}")

// each just does the work.
each(words, fn(w: str) {
    write("{upper(w)} ")
})
print("")

// They compose, which is the point.
let result = reduce(
    filter(map(nums, fn(n: int) -> int { return n * n }),
        fn(n: int) -> bool { return n % 2 == 1 }),
    0,
    fn(acc: int, n: int) -> int { return acc + n }
)
print("odd squares summed {result}")

// With named functions rather than literals.
fn isEven(n: int) -> bool {
    return n % 2 == 0
}
fn negate(n: int) -> int {
    return -n
}
print("evens {filter(nums, isEven)} negated {map(nums, negate)}")

// Sorting structs by a field.
struct Person {
    name: str
    age: int
}
let people = [
    Person{name: "grace", age: 45},
    Person{name: "ada", age: 36},
    Person{name: "alan", age: 41}
]
let byAge = sortBy(people, fn(a: Person, b: Person) -> bool { return a.age < b.age })
print("youngest {byAge[0].name} oldest {byAge[2].name}")
print("names {map(people, fn(p: Person) -> str { return p.name })}")

// A closure capturing a value used as a predicate.
let cutoff = 4
print("over cutoff {filter(nums, fn(n: int) -> bool { return n > cutoff })}")
