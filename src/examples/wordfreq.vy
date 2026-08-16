// wordfreq.vy -- count the words in a file and show the commonest.
//
//     veyl run examples\wordfreq.vy
//     veyl run examples\wordfreq.vy SYNTAX.md 20
//
// A whole small program: arguments, files that might not be there,
// regular expressions, maps, structs, sorting with your own comparison.

struct Count {
    word: str
    times: int
}

// Words are letters and apostrophes, so "don't" stays one word.
const WORD = `[A-Za-z']+`

fn tally(text: str) -> {str: int} {
    let counts: {str: int} = {}
    for word in re.findAll(WORD, lower(text)) {
        // A missing key reads as 0, so this needs no special first case.
        counts[word] += 1
    }
    return counts
}

fn ranked(counts: {str: int}) -> []Count {
    let out: []Count = []
    for word, times in counts {
        push(out, Count{word: word, times: times})
    }
    // Commonest first; ties fall back to alphabetical so the output is
    // the same every run.
    return sortBy(out, fn(a: Count, b: Count) -> bool {
        if a.times != b.times {
            return a.times > b.times
        }
        return a.word < b.word
    })
}

fn report(path: str, top: int) -> int! {
    let text = os.file.read(path)?
    let counts = tally(text)
    let table = ranked(counts)

    print("{path}: {len(text)} characters, {len(keys(counts))} distinct words")
    print("")

    let widest = 0
    for row in slice(table, 0, min(top, len(table))) {
        widest = max(widest, len(row.word))
    }

    for row in slice(table, 0, min(top, len(table))) {
        let bar = repeat("#", min(row.times, 40))
        print("  {padRight(row.word, widest)}  {padLeft(str(row.times), 5)}  {bar}")
    }
    return len(table)
}

// Arguments, with sensible defaults when there are none.
let args = os.args()
let path = "README.md"
let top = 12

if len(args) > 0 {
    path = args[0]
}
if len(args) > 1 {
    top = toInt(args[1], 12)
}

let outcome = report(path, top)
if failed(outcome) {
    print("could not read it: {errorOf(outcome)}")
    print("try: veyl run examples\\wordfreq.vy SYNTAX.md 20")
    exit(1)
}
print("")
print("{must(outcome)} distinct words in total")
