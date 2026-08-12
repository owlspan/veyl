// interactive.vy -- input, validation, and pause

fn describe(age: int) -> str {
    if age < 0 {
        return "not a real age"
    } else if age < 13 {
        return "a kid"
    } else if age < 20 {
        return "a teenager"
    } else {
        return "an adult"
    }
}

// Keep asking until the answer is actually a number.
fn askNumber(prompt: str) -> int {
    let raw = input(prompt)
    while !isInt(raw) {
        print("  that isn't a number, try again")
        raw = input(prompt)
    }
    return toInt(raw)
}

let name = input("What's your name? ")
while len(name) == 0 {
    name = input("Come on, give me a name: ")
}

let age = askNumber("How old are you? ")

print("")
print("Hi {name}, you are {describe(age)}.")
print("Next year you'll be {age + 1}.")
print("")
pause()
