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
	__ntdll    = syscall.NewLazyDLL("ntdll.dll")
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

	// winBuild reports the OS build number. RtlGetVersion is used rather
	// than GetVersionEx because the latter lies on unmanifested binaries:
	// it reports 6.2 on Windows 10 and 11 alike, which would make every
	// version check below useless.
	"winBuild": {
		code: `type __osVersionInfoW struct {
	dwOSVersionInfoSize uint32
	dwMajorVersion      uint32
	dwMinorVersion      uint32
	dwBuildNumber       uint32
	dwPlatformId        uint32
	szCSDVersion        [128]uint16
}

var __winBuildCache = -1

func __winBuild() int {
	if __winBuildCache >= 0 {
		return __winBuildCache
	}
	__winBuildCache = 0
	var info __osVersionInfoW
	info.dwOSVersionInfoSize = uint32(unsafe.Sizeof(info))
	r, _, _ := __ntdll.NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 { // STATUS_SUCCESS
		__winBuildCache = int(info.dwBuildNumber)
	}
	return __winBuildCache
}`,
		imports: []string{"unsafe"},
		deps:    []string{"win_dlls"},
	},

	// roundCorners asks the Desktop Window Manager to round a window's
	// corners. DWMWA_WINDOW_CORNER_PREFERENCE needs Windows 11 (build
	// 22000+); on Windows 10 the attribute does not exist. It returns
	// whether the corners were actually rounded, so no caller can claim
	// success it did not verify.
	"roundCorners": {
		code: `func __roundCorners(hwnd uintptr, on bool) bool {
	const DWMWA_WINDOW_CORNER_PREFERENCE = 33
	if !on {
		return false
	}
	if __winBuild() < 22000 {
		return false
	}
	pref := int32(2) // DWMWCP_ROUND
	r, _, _ := __dwmapi.NewProc("DwmSetWindowAttribute").Call(
		hwnd,
		DWMWA_WINDOW_CORNER_PREFERENCE,
		uintptr(unsafe.Pointer(&pref)),
		unsafe.Sizeof(pref),
	)
	return r == 0 // S_OK
}`,
		deps: []string{"win_dlls", "winBuild"},
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

// __openWindow returns whether the window's corners were actually
// rounded — never whether they were merely requested.
func __openWindow(title string, width int, height int, rounded bool) bool {
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
		return false
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
		return false
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
		return false
	}

	didRound := __roundCorners(hwnd, rounded)

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
	return didRound
}`,
		imports: []string{"fmt", "runtime", "syscall", "unsafe"},
		deps:    []string{"win_dlls", "roundCorners"},
	},
}

var winBuiltins = map[string]builtin{
	"messageBox": {
		emit:    func(a []string) string { return "__messageBox(" + a[0] + ", " + a[1] + ")" },
		helpers: []string{"messageBox"}, minArgs: 2, maxArgs: 2, osOnly: "windows",
		params: []*Type{Str, Str}, ret: Void,
	},
	"setTitle": {
		emit:    func(a []string) string { return "__setTitle(" + a[0] + ")" },
		helpers: []string{"setTitle"}, minArgs: 1, maxArgs: 1, osOnly: "windows",
		params: []*Type{Str}, ret: Void,
	},
	"beep": {
		emit:    func(a []string) string { return "__beep(" + a[0] + ", " + a[1] + ")" },
		helpers: []string{"beep"}, minArgs: 2, maxArgs: 2, osOnly: "windows",
		params: []*Type{Int, Int}, ret: Void,
	},
	"hideConsole": {
		emit:    func(a []string) string { return "__hideConsole()" },
		helpers: []string{"hideConsole"}, minArgs: 0, maxArgs: 0, osOnly: "windows",
		ret: Void,
	},
	"winBuild": {
		emit:    func(a []string) string { return "__winBuild()" },
		helpers: []string{"winBuild"}, minArgs: 0, maxArgs: 0, osOnly: "windows",
		ret: Int,
	},
	"isWin11": {
		emit:    func(a []string) string { return "(__winBuild() >= 22000)" },
		helpers: []string{"winBuild"}, minArgs: 0, maxArgs: 0, osOnly: "windows",
		ret: Bool,
	},
	// Returns whether the corners were actually rounded, which is false on
	// Windows 10 however the call was written.
	"openWindow": {
		emit: func(a []string) string {
			rounded := "true"
			if len(a) == 4 {
				rounded = a[3]
			}
			return "__openWindow(" + a[0] + ", " + a[1] + ", " + a[2] + ", " + rounded + ")"
		},
		helpers: []string{"win_window"}, minArgs: 3, maxArgs: 4, osOnly: "windows",
		params: []*Type{Str, Int, Int, Bool}, ret: Bool,
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
