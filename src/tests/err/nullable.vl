let maybe: ?int = nil
let plain = 5

// using a nullable without checking it
print(maybe + 1)
print(maybe * 2)
print(maybe > 3)

// a plain type cannot hold nil
let bad: int = nil
plain = nil

// nor can it usefully be compared to nil
if plain != nil {
    print("always")
}

// a nullable cannot stand in for a plain value
fn needsInt(n: int) -> int {
    return n
}
print(needsInt(maybe))

fn givesInt(m: ?int) -> int {
    return m
}

// the narrowing does not leak past the block that proved it
if maybe != nil {
    print(maybe + 1)
}
print(maybe + 1)

// nor to the wrong branch
if maybe == nil {
    print(maybe + 1)
}

// bare nil with nothing to infer from
let nothing = nil

// wrong inner type
let wrong: ?str = 5

// find returns a nullable, so it needs checking too
let ages = {"ada": 36}
print(find(ages, "ada") + 1)
