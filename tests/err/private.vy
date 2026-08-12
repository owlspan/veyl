import "mod/geometry.vy"

// geometry.vy declares these without pub, so they are private to it.
print(secret())
print(fudge)

// A private struct cannot be named either.
import "mod/hidden.vy"
let h = Hidden{n: 1}
print(usesHidden())
