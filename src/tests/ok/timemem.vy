// A fixed instant, so the formatting is deterministic.
// 1700000000 = 2023-11-14 22:13:20 UTC
const moment = 1700000000

print("iso    {time.format(moment, "YYYY-MM-DD")}")
print("long   {time.format(moment, "DDDD DD MMMM YYYY")}")
print("short  {time.format(moment, "DD/MM/YY")}")
print("clock  {len(time.format(moment, "HH:mm:ss")) == 8}")

// A round trip through parse and format returns the same text.
let text = time.format(moment, "YYYY-MM-DD HH:mm:ss")
let back = time.parse(text, "YYYY-MM-DD HH:mm:ss")
print("roundtrip {time.format(back, "YYYY-MM-DD HH:mm:ss") == text}")

// A malformed date parses as -1 rather than crashing.
print("bad parse {time.parse("not a date", "YYYY-MM-DD")}")

// The current time only needs to be sane, not exact.
print("now is recent {time.now() > 1700000000}")
print("millis bigger {time.millis() > time.now()}")
print("date length {len(time.date()) == 10}")
print("weekday nonempty {len(time.weekday()) > 0}")
print("month in range {time.month() >= 1 && time.month() <= 12}")

// Memory: allocate a lot, then check the counters moved.
let before = mem.total()
let big: []int = []
for i in 0..50000 {
    push(big, i)
}
print("allocated {mem.total() > before}")
print("list length {len(big)}")

mem.collect()
print("collections happened {mem.collections() > 0}")
print("goroutines at least one {mem.goroutines() >= 1}")
print("system memory positive {mem.system() > 0}")
