// literals and inference
let nums = [3, 1, 2]
let words = ["beta", "alpha"]
let mixed = [1, 2.5, 3]
print("{nums} {words} {mixed}")

// an empty list needs an annotation
let empty: []int = []
print("empty {empty} len {len(empty)}")

// growing and shrinking
push(empty, 10)
push(empty, 20, 30)
print("after push {empty}")
let top = pop(empty)
print("popped {top}, now {empty}")

// indexing, reading and writing
print("nums[0] = {nums[0]}")
nums[0] = 99
print("after write {nums}")
nums[0] += 1
print("after += {nums}")

// reading operations leave the original alone
print("sorted {sort(nums)} original {nums}")
print("reversed {reverse(words)}")
print("slice {slice(nums, 1, 3)}")
print("first {first(nums)} last {last(nums)}")
print("sum {sum([1, 2, 3, 4])}")
print("join {join(words, " | ")}")

// searching, sharing a name with the string versions
print("contains {contains(words, "alpha")} {contains("hello", "ell")}")
print("indexOf {indexOf(words, "alpha")} {indexOf("hello", "llo")}")

// insert and remove
let letters = ["a", "c"]
insert(letters, 1, "b")
print("inserted {letters}")
let gone = removeAt(letters, 0)
print("removed {gone}, left {letters}")
clear(letters)
print("cleared {letters} len {len(letters)}")

// iteration, one name and two
for w in words {
    write("{w} ")
}
print("")
for i, w in words {
    write("{i}={w} ")
}
print("")

// nested lists
let grid: [][]int = [[1, 2], [3, 4]]
print("grid {grid} grid[1][0] {grid[1][0]}")
grid[0][1] = 9
print("after nested write {grid}")

// strings to lists and back
print("split {split("a,b,c", ",")}")
print("chars {chars("hey")}")
let twoLines = "one\ntwo"
print("lines {len(lines(twoLines))}")

// lists through functions
fn total(xs: []int) -> int {
    let t = 0
    for x in xs {
        t += x
    }
    return t
}
print("total {total([5, 6, 7])}")

fn evens(limit: int) -> []int {
    let out: []int = []
    for i in 0..limit {
        if i % 2 == 0 {
            push(out, i)
        }
    }
    return out
}
print("evens {evens(10)}")
