fn mayFail(ok: bool) -> int! {
    if ok {
        return 1
    }
    return fail("nope")
}

// a result is not a value until it is unwrapped
let r = mayFail(true)
print(r + 1)
print(r > 0)

// nor can it stand in for the plain type
fn needsInt(n: int) -> int {
    return n
}
print(needsInt(r))

// '?' needs a function that can itself fail
fn cannotFail() -> int {
    return mayFail(true)?
}

// '?' needs something that can fail
fn wrongTry() -> int! {
    let plain = 5
    return plain?
}

// the inspection builtins need a result
print(isOk(5))
print(errorOf("text"))
print(must([1, 2]))
print(valueOr(7, 8))

// fail() only makes sense where a result is wanted
fn notAResult() -> int {
    return fail("no")
}

// the fallback has to match what the result carries
print(valueOr(r, "text"))

// returning the wrong thing from a failing function
fn wrongInner() -> str! {
    return 5
}
