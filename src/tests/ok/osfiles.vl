// The example from the brief, in the shorthand spelling.
// Reading can fail, so it hands back a str! rather than dying.
let coolVariable = "f297fh3"
os.write.file("vy_scratch.txt", coolVariable)
let thing1 = os.read.file("vy_scratch.txt")
print("read back: {must(thing1)}")

// The same calls in the documented noun-first spelling.
os.file.write("vy_scratch.txt", "line one\n")
os.file.append("vy_scratch.txt", "line two\n")
print("size {must(os.file.size("vy_scratch.txt"))}")
print("lines {must(os.file.lines("vy_scratch.txt"))}")

// A failure carries a reason instead of stopping the program.
let missing = os.file.read("definitely_not_here.txt")
print("missing ok {isOk(missing)}")
print("reason mentions the path {contains(errorOf(missing), "definitely_not_here.txt")}")
print("valueOr {valueOr(missing, "fallback")}")

print("exists {os.file.exists("vy_scratch.txt")} missing {os.file.exists("nope.txt")}")
print("readOr {os.file.readOr("nope.txt", "default")}")

// A whole pipeline that stops at the first failure.
fn wordCount(path: str) -> int! {
    let text = os.file.read(path)?
    return len(split(trim(text), " "))
}
os.file.write("vy_words.txt", "one two three")
print("words {must(wordCount("vy_words.txt"))}")
print("bad path {isOk(wordCount("nope.txt"))}")
os.file.delete("vy_words.txt")

// directories
os.dir.make("vy_scratch_dir")
os.file.write("vy_scratch_dir/a.txt", "a")
os.file.write("vy_scratch_dir/b.txt", "b")
print("listed {must(os.dir.list("vy_scratch_dir"))}")
print("listing a missing dir fails {isOk(os.dir.list("no_such_dir"))}")
print("is dir {os.dir.is("vy_scratch_dir")} is file {os.dir.is("vy_scratch.txt")}")

// Paths never touch the disk. Separators are normalised here so the
// expected output is the same on Windows and everywhere else.
let joined = replace(os.path.join("a", "b", "c.txt"), "\\", "/")
let parent = replace(os.path.dir("/x/y/z.txt"), "\\", "/")
print("join {joined}")
print("base {os.path.base("/x/y/z.txt")} dir {parent} ext {os.path.ext("/x/y/z.txt")}")

// environment
os.env.set("VEYL_TEST_VAR", "hello")
print("env {os.env.get("VEYL_TEST_VAR")} has {os.env.has("VEYL_TEST_VAR")} missing {os.env.has("VEYL_NOT_SET_VAR")}")

// machine facts that are stable enough to assert on
print("cpus positive {os.cpus() > 0}")
print("pid positive {os.pid() > 0}")
print("hostname nonempty {len(os.hostname()) > 0}")

// running a program that does not exist fails rather than returning ""
print("bogus command ok {isOk(os.run("definitely_not_a_program_xyz", []))}")

// tidy up, and prove deletion worked
os.file.delete("vy_scratch.txt")
os.dir.delete("vy_scratch_dir")
print("cleaned {os.file.exists("vy_scratch.txt")} {os.dir.is("vy_scratch_dir")}")
