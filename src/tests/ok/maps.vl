// literals and inference
let ages = {"ada": 36, "alan": 41}
print("ages {ages}")

// an empty map needs an annotation, like an empty list
let scores: {str: float} = {}
scores["pi"] = 3.14
scores["half"] = 0.5
print("scores {scores}")

// reading, writing, and compound assignment
print("ada is {ages["ada"]}")
ages["ada"] = 37
ages["alan"] += 1
print("after edits {ages}")

// a missing key reads as the zero value, so test with has() first
print("missing {ages["nobody"]} has {has(ages, "nobody")} {has(ages, "ada")}")

// keys, values and length, all in sorted order
print("keys {keys(ages)} values {values(ages)} len {len(ages)}")

// removing
remove(ages, "alan")
print("after remove {ages} len {len(ages)}")

// int keys
let squares: {int: int} = {}
for i in 1..=4 {
    squares[i] = i * i
}
print("squares {squares}")

// iteration is sorted, so the output is the same on every run
for name, age in ages {
    write("{name}={age} ")
}
print("")
for n, sq in squares {
    write("{n}:{sq} ")
}
print("")

// nested: a map of lists
let groups: {str: []str} = {}
groups["fruit"] = ["apple", "pear"]
groups["veg"] = ["leek"]
push(groups["fruit"], "plum")
print("groups {groups}")
print("fruit count {len(groups["fruit"])} second {groups["fruit"][1]}")

// maps through functions
fn tally(words: []str) -> {str: int} {
    let counts: {str: int} = {}
    for w in words {
        counts[w] += 1
    }
    return counts
}
print("tally {tally(["a", "b", "a", "c", "a"])}")

// clear works on both kinds of collection
clear(squares)
print("cleared {squares} len {len(squares)}")
