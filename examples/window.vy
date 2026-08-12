// window.vy -- native Windows GUI from Veyl

setTitle("Veyl Demo")

print("Windows build {winBuild()}, Windows 11: {isWin11()}")
print("Opening a native window. Close it to continue.")
beep(880, 150)

// openWindow reports whether the corners were actually rounded, which
// is false on Windows 10 no matter what was requested.
let rounded = openWindow("Hello from Veyl", 800, 500)

print("Window closed. Corners were rounded: {rounded}")
messageBox("Veyl", "That window was created by your own language.")
pause()
