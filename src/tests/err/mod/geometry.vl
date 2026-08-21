// An imported module. Only `pub` things are visible to importers.

pub const TAU = 6.283185307179586

// Private to this file - importers cannot see it.
const fudge = 0.0

pub struct Vec {
    x: float
    y: float
}

impl Vec {
    fn length(self) -> float {
        return sqrt(self.x * self.x + self.y * self.y)
    }

    fn plus(self, other: Vec) -> Vec {
        return Vec{x: self.x + other.x, y: self.y + other.y}
    }
}

pub fn circleArea(radius: float) -> float {
    return (TAU / 2.0) * radius * radius + fudge
}

// Not pub: an importer that calls this should be refused.
fn secret() -> str {
    return "internal"
}

pub fn describe(v: Vec) -> str {
    return "({v.x}, {v.y}) length {v.length()}"
}
