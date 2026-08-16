// Declaration-level mistakes, caught by the resolver.

struct Bad {
    a: int
    a: str
}

struct Point {
    x: float
    y: float
}

impl Point {
    fn length(self) -> float {
        return 1.0
    }
    fn length(self) -> float {
        return 0.0
    }
    fn x(self) -> float {
        return 0.0
    }
    fn noSelf() -> int {
        return 1
    }
}

impl Nothing {
    fn hello(self) {
        print("hi")
    }
}

let u = Ghost{name: "boo"}
