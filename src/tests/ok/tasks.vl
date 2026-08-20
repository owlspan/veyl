// task.map is map(), run concurrently. Same arguments, same ordered
// results - switching between them is a one-word edit.

let nums = [1, 2, 3, 4, 5, 6, 7, 8]

fn square(n: int) -> int {
    return n * n
}

let serial = map(nums, square)
let concurrent = task.map(nums, square)
print("same answer {serial == concurrent}")
print("ordered {concurrent}")

// Order holds even when the work finishes out of order: the slowest
// item is first, so a naive implementation would put it last.
let staggered = task.map([40, 20, 5, 1], fn(ms: int) -> int {
    sleep(ms)
    return ms
})
print("still in order {staggered}")

// A cap on how many run at once, for work that is rate-limited rather
// than CPU-bound.
let limited = task.mapLimit(nums, 2, square)
print("limited {limited}")

// It is faster than doing it one at a time, which is the whole point.
// Eight 60ms sleeps: serial is about 480ms, concurrent about 60.
fn nap(ms: int) -> int {
    sleep(ms)
    return ms
}
let waits = [60, 60, 60, 60, 60, 60, 60, 60]

let t0 = time.millis()
let a = map(waits, nap)
let serialMs = time.millis() - t0

let t1 = time.millis()
let b = task.map(waits, nap)
let parallelMs = time.millis() - t1

print("both did {len(a)} and {len(b)} items")
print("concurrent was faster {parallelMs < serialMs}")
print("and by a lot {parallelMs * 3 < serialMs}")

// task.each when the results are not wanted.
let seen: []int = []
task.each([1, 2, 3], fn(n: int) {
    sleep(1)
})
print("each finished {len(seen) == 0}")

// task.all runs a set of unrelated jobs and waits for the last one.
let jobs: []fn() = [
    fn() { sleep(20) },
    fn() { sleep(10) },
    fn() { sleep(30) }
]
let t2 = time.millis()
task.all(jobs)
let allMs = time.millis() - t2
print("waited for the slowest {allMs >= 25} not the sum {allMs < 55}")

// Composing with the rest of the library.
let lengths = task.map(["alpha", "be", "gamma!"], fn(s: str) -> int {
    return len(s)
})
print("lengths {lengths} total {sum(lengths)}")
