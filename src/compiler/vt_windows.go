//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// enableVirtualTerminal switches the console into the mode where ANSI
// escapes mean something.
//
// The generated program already does this, through the term library's
// Windows-only helper. The compiler itself did not, which is why the
// console banner arrived as literal <-[36m: veyl.exe was writing
// escape sequences at a console that had never been told to interpret
// them. The two are separate programs and each has to ask.
//
// Unlike the generated program, the compiler is an ordinary Go package
// and can use build tags, so this needs no runtime branch.
func enableVirtualTerminal() {
	const enableVTProcessing = 0x0004

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	handle, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}
	var mode uint32
	if r, _, _ := getConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode))); r == 0 {
		// Not a console: output is redirected to a file or a pipe.
		// colourWanted has already reached the same conclusion.
		return
	}
	setConsoleMode.Call(uintptr(handle), uintptr(mode|enableVTProcessing))
}
