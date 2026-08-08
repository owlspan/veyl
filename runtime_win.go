package main

// This file holds the Windows-only half of the standard library.
//
// Everything here is emitted into the generated program on demand and
// calls into Win32 through Go's syscall package, which loads DLLs at
// runtime. That means no cgo, no C compiler, and no external
// dependencies — a Quartz program that opens a window is still a single
// self-contained .exe.

var winHelperDefs = map[string]helperDef{

	"win_dlls": {
		code: `var (
	__user32   = syscall.NewLazyDLL("user32.dll")
	__kernel32 = syscall.NewLazyDLL("kernel32.dll")
	__dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
)

func __utf16(s string) uintptr {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return 0
	}
	return uintptr(unsafe.Pointer(p))
}`,
		imports: []string{"syscall", "unsafe"},
	},

	"messageBox": {
		code: `func __messageBox(title string, text string) {
	__user32.NewProc("MessageBoxW").Call(0, __utf16(text), __utf16(title), 0)
}`,
		deps: []string{"win_dlls"},
	},

	"setTitle": {
		code: `func __setTitle(s string) {
	__kernel32.NewProc("SetConsoleTitleW").Call(__utf16(s))
}`,
		deps: []string{"win_dlls"},
	},

	"beep": {
		code: `func __beep(freq int, ms int) {
	__kernel32.NewProc("Beep").Call(uintptr(freq), uintptr(ms))
}`,
		deps: []string{"win_dlls"},
	},

	"hideConsole": {
		code: `func __hideConsole() {
	hwnd, _, _ := __kernel32.NewProc("GetConsoleWindow").Call()
	if hwnd != 0 {
		__user32.NewProc("ShowWindow").Call(hwnd, 0) // SW_HIDE
	}
}`,
		deps: []string{"win_dlls"},
	},

	// roundCorners asks the Desktop Window Manager to round a window's
	// corners. DWMWA_WINDOW_CORNER_PREFERENCE is Windows 11 only; on
	// Windows 10 the call fails harmlessly and the window stays square.
	"roundCorners": {
		code: `func __roundCorners(hwnd uintptr, on bool) {
	const DWMWA_WINDOW_CORNER_PREFERENCE = 33
	pref := int32(1) // DWMWCP_DONOTROUND
	if on {
		pref = 2 // DWMWCP_ROUND
	}
	__dwmapi.NewProc("DwmSetWindowAttribute").Call(
		hwnd,
		DWMWA_WINDOW_CORNER_PREFERENCE,
		uintptr(unsafe.Pointer(&pref)),
		unsafe.Sizeof(pref),
	)
}`,
		deps: []string{"win_dlls"},
	},

	"win_window": {
		code: `type __wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type __point struct{ x, y int32 }

type __msgStruct struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      __point
}

func __wndProc(hwnd uintptr, msg uint32, wp uintptr, lp uintptr) uintptr {
	const (
		WM_DESTROY = 0x0002
		WM_CLOSE   = 0x0010
	)
	switch msg {
	case WM_CLOSE:
		__user32.NewProc("DestroyWindow").Call(hwnd)
		return 0
	case WM_DESTROY:
		__user32.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	r, _, _ := __user32.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wp, lp)
	return r
}

// The callback is created once. syscall.NewCallback draws from a small
// fixed pool, so allocating one per window would eventually panic.
var __wndProcPtr = syscall.NewCallback(__wndProc)

var __classCount int

func __openWindow(title string, width int, height int, rounded bool) {
	// Win32 requires that a window and its message loop live on the same
	// OS thread. Without this, Go's scheduler may move the goroutine and
	// the loop stops receiving messages.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const (
		CS_HREDRAW          = 0x0002
		CS_VREDRAW          = 0x0001
		WS_OVERLAPPEDWINDOW = 0x00CF0000
		CW_USEDEFAULT       = 0x80000000
		SW_SHOW             = 5
		IDC_ARROW           = 32512
		COLOR_WINDOW        = 5
	)

	hInstance, _, _ := __kernel32.NewProc("GetModuleHandleW").Call(0)
	cursor, _, _ := __user32.NewProc("LoadCursorW").Call(0, IDC_ARROW)

	__classCount++
	className, err := syscall.UTF16PtrFromString(fmt.Sprintf("QuartzWindow%d", __classCount))
	if err != nil {
		return
	}

	wc := __wndClassExW{
		style:         CS_HREDRAW | CS_VREDRAW,
		lpfnWndProc:   __wndProcPtr,
		hInstance:     hInstance,
		hCursor:       cursor,
		hbrBackground: COLOR_WINDOW + 1,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))

	atom, _, _ := __user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return
	}

	hwnd, _, _ := __user32.NewProc("CreateWindowExW").Call(
		0,
		uintptr(unsafe.Pointer(className)),
		__utf16(title),
		WS_OVERLAPPEDWINDOW,
		CW_USEDEFAULT, CW_USEDEFAULT,
		uintptr(width), uintptr(height),
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		return
	}

	if rounded {
		__roundCorners(hwnd, true)
	}

	__user32.NewProc("ShowWindow").Call(hwnd, SW_SHOW)
	__user32.NewProc("UpdateWindow").Call(hwnd)

	getMessage := __user32.NewProc("GetMessageW")
	translate := __user32.NewProc("TranslateMessage")
	dispatch := __user32.NewProc("DispatchMessageW")

	var m __msgStruct
	for {
		r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // 0 = WM_QUIT, -1 = error
			break
		}
		translate.Call(uintptr(unsafe.Pointer(&m)))
		dispatch.Call(uintptr(unsafe.Pointer(&m)))
	}
}`,
		imports: []string{"fmt", "runtime", "syscall", "unsafe"},
		deps:    []string{"win_dlls", "roundCorners"},
	},
}

var winBuiltins = map[string]builtin{
	"messageBox": {
		emit:    func(a []string) string { return "__messageBox(" + a[0] + ", " + a[1] + ")" },
		helpers: []string{"messageBox"}, minArgs: 2, maxArgs: 2, osOnly: "windows",
	},
	"setTitle": {
		emit:    func(a []string) string { return "__setTitle(" + a[0] + ")" },
		helpers: []string{"setTitle"}, minArgs: 1, maxArgs: 1, osOnly: "windows",
	},
	"beep": {
		emit:    func(a []string) string { return "__beep(" + a[0] + ", " + a[1] + ")" },
		helpers: []string{"beep"}, minArgs: 2, maxArgs: 2, osOnly: "windows",
	},
	"hideConsole": {
		emit:    func(a []string) string { return "__hideConsole()" },
		helpers: []string{"hideConsole"}, minArgs: 0, maxArgs: 0, osOnly: "windows",
	},
	"openWindow": {
		emit: func(a []string) string {
			rounded := "true"
			if len(a) == 4 {
				rounded = a[3]
			}
			return "__openWindow(" + a[0] + ", " + a[1] + ", " + a[2] + ", " + rounded + ")"
		},
		helpers: []string{"win_window"}, minArgs: 3, maxArgs: 4, osOnly: "windows",
	},
}

// registerWindowsRuntime folds the Windows library into the core tables.
// Called explicitly from codegen's init() so ordering is deterministic
// rather than depending on Go's cross-file init sequence.
func registerWindowsRuntime() {
	for k, v := range winHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range winBuiltins {
		builtins[k] = v
	}
}
