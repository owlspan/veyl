// A file that is not there.
import "no_such_module.vy"

// Not a .vy file.
import "notes.txt"

// A file whose top level does more than declare things.
import "mod/runs_things.vy"

// A cycle: this one imports back to something that imports it.
import "mod/loop_a.vy"

print("unreachable")
