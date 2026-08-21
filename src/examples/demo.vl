// Veyl v0.2 -- functions

fn square(n: int) -> int {
    return n * n
}

fn fib(n: int) -> int {
    if n < 2 {
        return n
    }
    return fib(n - 1) + fib(n - 2)
}

fn greet(name: str, times: int) {
    let i = 0
    while i < times {
        print("hello, {name}!")
        i += 1
    }
}

fn classify(n: int) -> str {
    if n % 15 == 0 {
        return "fizzbuzz"
    } else if n % 3 == 0 {
        return "fizz"
    } else if n % 5 == 0 {
        return "buzz"
    } else {
        return str(n)
    }
}

greet("world", 2)
print("square(7) = {square(7)}")
print("fib(20) = {fib(20)}")

let i = 1
while i <= 15 {
    write("{classify(i)} ")
    i += 1
}
print("")
print("min/max: {min(3, 9, 2)} {max(3, 9, 2)}")
