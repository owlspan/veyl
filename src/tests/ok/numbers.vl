// Untyped integer literals adapt to a float operand, exactly as Go's
// untyped constants do.
let radius = 2.5
let doubled = radius * 2
let shifted = radius + 1
print("doubled {doubled} shifted {shifted}")

// Two variables of different types still need an explicit conversion.
let n = 3
let asFloat = float(n) * radius
print("asFloat {asFloat}")

// An annotation accepts an integer literal for a float binding.
let ratio: float = 1
ratio += 1
print("ratio {ratio}")

// Integer division truncates; divf gives the fractional result.
print("7/2 = {7 / 2}, divf = {divf(7, 2)}")

// Numeric builtins take int or float.
print("{sqrt(2)} {sqrt(2.0)} {max(3, 9)} {min(1.5, 0.5)}")

// Modulo is int-only; mod() covers floats.
print("{7 % 2} {mod(7.5, 2.0)}")

// Rounding returns int.
print("{floor(3.7)} {ceil(3.2)} {round(2.6)} {trunc(-1.8)}")
