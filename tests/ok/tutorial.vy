// Every claim TUTORIAL.md makes, as a program that runs. If the
// language changes under the tutorial, this fails rather than the
// tutorial quietly going stale.

// ---- values ----
let count = 0
const LIMIT = 10
let name: str = "ada"
count += 1
print("values {count} {LIMIT} {name}")

let n = 42
let ratio = 2.5
let word = "hello"
let ok = true
let xs = [1, 2, 3]
let ages = {"ada": 36}
print("types {n} {ratio} {word} {ok} {xs} {ages}")

print("count: " + str(n))
print("count: {n}")
print("{n} squared is {n * n}")
print("shouting: {upper(word)}")

let radius = 2.5
let area = PI * radius * radius
let wide = radius * 2
let two = 2
let converted = radius * float(two)
print("numbers {area} {wide} {converted}")

const PATTERN = `\d{4}-\d{2}-\d{2}`
print("pattern finds {re.find(PATTERN, "on 2024-01-15 it rained")}")

// ---- deciding and repeating ----
let score = 85
if score >= 90 {
    print("A")
} else if score >= 80 {
    print("B")
} else {
    print("C")
}

let words = ["alpha", "beta"]
for i in 0..3 {
    write("{i} ")
}
for i in 1..=3 {
    write("{i} ")
}
for i in 10..0 step -4 {
    write("{i} ")
}
print("")

for w in words {
    write("{w} ")
}
for i, w in words {
    write("{i}={w} ")
}
for who, age in ages {
    write("{who}:{age} ")
}
print("")

let code = 302
match code {
    200 => print("ok")
    301, 302 => print("redirect")
    else => print("something else")
}

// ---- functions ----
fn add(a: int, b: int) -> int {
    return a + b
}

fn greet(who: str) {
    print("hello, {who}")
}

let double = fn(x: int) -> int { return x * 2 }

fn twice(f: fn(int) -> int, start: int) -> int {
    return f(f(start))
}

print("add {add(2, 3)}")
greet("veyl")
print("twice {twice(double, 3)}")

// ---- lists and maps ----
let nums = [3, 1, 2]
push(nums, 4)
print("list {nums[0]} {len(nums)} {sort(nums)} original {nums}")

let people = {"ada": 36}
people["alan"] = 41
people["ada"] += 1
print("map {has(people, "grace")} {keys(people)}")

let counts: {str: int} = {}
for w in split("a b a", " ") {
    counts[w] += 1
}
print("counted {counts}")

let emptyList: []int = []
let emptyMap: {str: int} = {}
print("empty {emptyList} {emptyMap}")

print("map {map(nums, fn(x: int) -> int { return x * x })}")
print("filter {filter(nums, fn(x: int) -> bool { return x > 1 })}")
print("reduce {reduce(nums, 0, fn(acc: int, x: int) -> int { return acc + x })}")
print("sortBy {sortBy(nums, fn(a: int, b: int) -> bool { return a > b })}")

// ---- structs ----
struct Point {
    x: float
    y: float
}

impl Point {
    fn length(self) -> float {
        return sqrt(self.x * self.x + self.y * self.y)
    }

    fn scale(self, by: float) {
        self.x *= by
        self.y *= by
    }
}

let p = Point{x: 3.0, y: 4.0}
print("length {p.length()}")
p.scale(2.0)
print("scaled {p.length()}")
print("origin {Point{}}")

let a = Point{x: 1.0, y: 1.0}
let b = a
b.x = 99.0
print("copies {a.x} {b.x}")

// ---- when there is nothing ----
let note: ?str = nil
if note != nil {
    print("never runs {upper(note)}")
}
let filled: ?str = "here"
if filled != nil {
    print("narrowed {upper(filled)}")
}

// ---- when it goes wrong ----
os.file.write("vy_tutorial.txt", "one two three")
let text = os.file.read("vy_tutorial.txt")
print("must {len(must(text))}")
print("valueOr {len(valueOr(text, ""))}")
print("isOk {isOk(text)} errorOf {errorOf(text) == ""}")

fn wordCount(path: str) -> int! {
    let body = os.file.read(path)?
    return len(split(trim(body), " "))
}
print("wordCount {must(wordCount("vy_tutorial.txt"))}")
print("wordCount of a missing file {isOk(wordCount("nope.txt"))}")

fn parsePort(t: str) -> int! {
    if !isInt(t) {
        return fail("{t} is not a number")
    }
    return toInt(t)
}
print("parsePort {valueOr(parsePort("8080"), -1)} {valueOr(parsePort("x"), -1)}")

// ---- doing real work ----
print("stamp is {len(time.stamp())} characters")
print("sha256 of abc starts {substr(hash.sha256("abc"), 0, 8)}")
print("findAll {re.findAll(`\d+`, "a1 b22 c333")}")

let doubled = task.map([1, 2, 3], fn(x: int) -> int { return x * 2 })
print("task.map {doubled}")

os.file.delete("vy_tutorial.txt")
print("cleaned up {os.file.exists("vy_tutorial.txt")}")
