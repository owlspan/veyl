// Warnings do not stop a program running. They go to stderr, after the
// program is known to compile, so they never bury a real error.

fn earlyOut(n: int) -> int {
    return n
    print("unreachable")
}

fn loopExit() {
    for i in 0..3 {
        break
        print("unreachable too")
    }
}

// Declared, written, never read.
let leftover = 42

// A const is exempt: a global that nothing uses yet is normal.
const SPARE = 99

let used = earlyOut(7)
print("earlyOut {used}")
loopExit()
print("still running")
