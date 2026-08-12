package main

// This file holds the Windows-only half of the standard library.
//
// Everything here is emitted into the generated program on demand and
// calls into Win32 through Go's syscall package, which loads DLLs at
// runtime. That means no cgo, no C compiler, and no external
// dependencies - a Veyl program that opens a window is still a single
// self-contained .exe.

var winHelperDefs = map[string]helperDef{

	"win_dlls": {
		code: `var (
	__user32   = syscall.NewLazyDLL("user32.dll")
	__kernel32 = syscall.NewLazyDLL("kernel32.dll")
	__dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	__ntdll    = syscall.NewLazyDLL("ntdll.dll")
	__advapi32 = syscall.NewLazyDLL("advapi32.dll")
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
// rounded - never whether they were merely requested.
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
	className, err := syscall.UTF16PtrFromString(fmt.Sprintf("VeylWindow%d", __classCount))
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

// ---- the win namespace ----
//
// Everything added after the original bare names lives under `win.`,
// which groups it and leaves room for the rest of Win32 without
// crowding the global names.

var winExtraHelperDefs = map[string]helperDef{
	"clipboard": {
		// The clipboard is a shared, locked resource: open it, take what
		// you need, close it. Leaving it open blocks every other program
		// on the machine, so every path here closes it.
		//
		// `go vet` reports "possible misuse of unsafe.Pointer" for the two
		// conversions below, and it is right to be suspicious in general:
		// turning a uintptr into a pointer is unsound when the address
		// belongs to the Go heap, because the collector may have moved it.
		// It does not here. GlobalAlloc memory belongs to Windows, is
		// never moved by Go's collector, and stays valid between the
		// GlobalLock and GlobalUnlock that bracket each use. There is no
		// way to express that to vet without a dependency on
		// golang.org/x/sys, which would cost the zero-dependency property
		// for a warning.
		code: `func __clipGet() string {
	const CF_UNICODETEXT = 13

	if r, _, _ := __user32.NewProc("OpenClipboard").Call(0); r == 0 {
		return ""
	}
	defer __user32.NewProc("CloseClipboard").Call()

	h, _, _ := __user32.NewProc("GetClipboardData").Call(CF_UNICODETEXT)
	if h == 0 {
		return ""
	}
	p, _, _ := __kernel32.NewProc("GlobalLock").Call(h)
	if p == 0 {
		return ""
	}
	defer __kernel32.NewProc("GlobalUnlock").Call(h)

	size, _, _ := __kernel32.NewProc("GlobalSize").Call(h)
	if size < 2 {
		return ""
	}
	// One conversion, then ordinary slice indexing. UTF16ToString stops
	// at the first NUL, so a buffer larger than the text is harmless.
	chars := unsafe.Slice((*uint16)(unsafe.Pointer(p)), int(size/2))
	return syscall.UTF16ToString(chars)
}

func __clipSet(text string) bool {
	const (
		CF_UNICODETEXT = 13
		GMEM_MOVEABLE  = 0x0002
	)

	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return false
	}
	size := uintptr(len(utf16) * 2)

	h, _, _ := __kernel32.NewProc("GlobalAlloc").Call(GMEM_MOVEABLE, size)
	if h == 0 {
		return false
	}
	p, _, _ := __kernel32.NewProc("GlobalLock").Call(h)
	if p == 0 {
		__kernel32.NewProc("GlobalFree").Call(h)
		return false
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(utf16)), utf16)
	__kernel32.NewProc("GlobalUnlock").Call(h)

	if r, _, _ := __user32.NewProc("OpenClipboard").Call(0); r == 0 {
		__kernel32.NewProc("GlobalFree").Call(h)
		return false
	}
	defer __user32.NewProc("CloseClipboard").Call()
	__user32.NewProc("EmptyClipboard").Call()

	// Once SetClipboardData succeeds the system owns the memory, so it
	// must not be freed here.
	if r, _, _ := __user32.NewProc("SetClipboardData").Call(CF_UNICODETEXT, h); r == 0 {
		__kernel32.NewProc("GlobalFree").Call(h)
		return false
	}
	return true
}`,
		imports: []string{"syscall", "unsafe"},
		deps:    []string{"win_dlls"},
	},

	"registry": {
		code: `func __regRoot(name string) uintptr {
	switch strings.ToUpper(name) {
	case "HKCU", "HKEY_CURRENT_USER":
		return 0x80000001
	case "HKLM", "HKEY_LOCAL_MACHINE":
		return 0x80000002
	case "HKCR", "HKEY_CLASSES_ROOT":
		return 0x80000000
	case "HKU", "HKEY_USERS":
		return 0x80000003
	}
	return 0
}

func __regRead(root string, path string, name string) __Res[string] {
	const (
		KEY_READ   = 0x20019
		ERROR_NONE = 0
	)
	hRoot := __regRoot(root)
	if hRoot == 0 {
		return __fail[string]("unknown registry root " + root + " (try HKCU or HKLM)")
	}

	var key uintptr
	r, _, _ := __advapi32.NewProc("RegOpenKeyExW").Call(
		hRoot, __utf16(path), 0, KEY_READ, uintptr(unsafe.Pointer(&key)))
	if r != ERROR_NONE {
		return __fail[string](fmt.Sprintf("cannot open %s\\%s: error %d", root, path, r))
	}
	defer __advapi32.NewProc("RegCloseKey").Call(key)

	// Asked twice: once for the size, once for the value.
	var size uint32
	var kind uint32
	r, _, _ = __advapi32.NewProc("RegQueryValueExW").Call(
		key, __utf16(name), 0,
		uintptr(unsafe.Pointer(&kind)), 0, uintptr(unsafe.Pointer(&size)))
	if r != ERROR_NONE {
		return __fail[string](fmt.Sprintf("cannot read %s: error %d", name, r))
	}

	buf := make([]uint16, size/2+1)
	r, _, _ = __advapi32.NewProc("RegQueryValueExW").Call(
		key, __utf16(name), 0,
		uintptr(unsafe.Pointer(&kind)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != ERROR_NONE {
		return __fail[string](fmt.Sprintf("cannot read %s: error %d", name, r))
	}

	const REG_DWORD = 4
	if kind == REG_DWORD {
		return __ok(fmt.Sprintf("%d", *(*uint32)(unsafe.Pointer(&buf[0]))))
	}
	return __ok(syscall.UTF16ToString(buf))
}`,
		imports: []string{"fmt", "strings", "syscall", "unsafe"},
		deps:    []string{"win_dlls", "result"},
	},

	"processList": {
		code: `type __procEntry struct {
	dwSize              uint32
	cntUsage            uint32
	th32ProcessID       uint32
	th32DefaultHeapID   uintptr
	th32ModuleID        uint32
	cntThreads          uint32
	th32ParentProcessID uint32
	pcPriClassBase      int32
	dwFlags             uint32
	szExeFile           [260]uint16
}

func __processes() []string {
	const TH32CS_SNAPPROCESS = 0x00000002

	snap, _, _ := __kernel32.NewProc("CreateToolhelp32Snapshot").Call(TH32CS_SNAPPROCESS, 0)
	if snap == 0 || snap == ^uintptr(0) {
		return []string{}
	}
	defer __kernel32.NewProc("CloseHandle").Call(snap)

	var e __procEntry
	e.dwSize = uint32(unsafe.Sizeof(e))

	out := []string{}
	first := __kernel32.NewProc("Process32FirstW")
	next := __kernel32.NewProc("Process32NextW")

	r, _, _ := first.Call(snap, uintptr(unsafe.Pointer(&e)))
	for r != 0 {
		out = append(out, syscall.UTF16ToString(e.szExeFile[:]))
		r, _, _ = next.Call(snap, uintptr(unsafe.Pointer(&e)))
	}
	sort.Strings(out)
	return out
}`,
		imports: []string{"sort", "syscall", "unsafe"},
		deps:    []string{"win_dlls"},
	},

	"cursor": {
		code: `type __cursorPoint struct{ x, y int32 }

func __cursor() (int, int) {
	var p __cursorPoint
	__user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
	return int(p.x), int(p.y)
}`,
		imports: []string{"unsafe"},
		deps:    []string{"win_dlls"},
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

	// ---- win.clipboard ----

	"win.clipboard.get": {
		emit: func(a []string) string { return "__clipGet()" },
		ret:  Str, osOnly: "windows",
		helpers: []string{"clipboard"},
	},
	"win.clipboard.set": {
		emit:   func(a []string) string { return "__clipSet(" + a[0] + ")" },
		params: []*Type{Str}, ret: Bool, minArgs: 1, maxArgs: 1, osOnly: "windows",
		helpers: []string{"clipboard"},
	},

	// ---- win.registry ----
	//
	// Reading only. Writing to the registry from a scripting language
	// is a good way to break a machine by accident, and nothing here
	// needs it yet.

	"win.registry.read": {
		emit: func(a []string) string {
			return "__regRead(" + a[0] + ", " + a[1] + ", " + a[2] + ")"
		},
		params: []*Type{Str, Str, Str}, ret: ResultOf(Str), minArgs: 3, maxArgs: 3,
		osOnly:  "windows",
		helpers: []string{"registry"},
	},

	// ---- win.screen and win.mouse ----

	"win.screen.width": {
		emit: func(a []string) string {
			return "func() int { w, _, _ := __user32.NewProc(\"GetSystemMetrics\").Call(0); return int(w) }()"
		},
		ret: Int, osOnly: "windows",
		helpers: []string{"win_dlls"},
	},
	"win.screen.height": {
		emit: func(a []string) string {
			return "func() int { h, _, _ := __user32.NewProc(\"GetSystemMetrics\").Call(1); return int(h) }()"
		},
		ret: Int, osOnly: "windows",
		helpers: []string{"win_dlls"},
	},
	"win.mouse.x": {
		emit: func(a []string) string { return "func() int { x, _ := __cursor(); return x }()" },
		ret:  Int, osOnly: "windows",
		helpers: []string{"cursor"},
	},
	"win.mouse.y": {
		emit: func(a []string) string { return "func() int { _, y := __cursor(); return y }()" },
		ret:  Int, osOnly: "windows",
		helpers: []string{"cursor"},
	},

	// ---- win.process ----

	"win.process.list": {
		emit: func(a []string) string { return "__processes()" },
		ret:  ListOf(Str), osOnly: "windows",
		helpers: []string{"processList"},
	},
	"win.process.running": {
		emit: func(a []string) string {
			return "slices.Contains(__processes(), " + a[0] + ")"
		},
		params: []*Type{Str}, ret: Bool, minArgs: 1, maxArgs: 1, osOnly: "windows",
		helpers: []string{"processList"},
		imports: []string{"slices"},
	},

	// ---- win.key ----
	//
	// The high bit of GetAsyncKeyState means "down right now"; the low
	// bit means "was pressed since last asked", which is a different
	// question and not the one being asked here.

	"win.key.down": {
		emit: func(a []string) string {
			return "func() bool { s, _, _ := __user32.NewProc(\"GetAsyncKeyState\").Call(uintptr(" +
				a[0] + ")); return s&0x8000 != 0 }()"
		},
		params: []*Type{Int}, ret: Bool, minArgs: 1, maxArgs: 1, osOnly: "windows",
		helpers: []string{"win_dlls"},
	},
}

// registerWindowsRuntime folds the Windows library into the core tables.
// Called explicitly from codegen's init() so ordering is deterministic
// rather than depending on Go's cross-file init sequence.
func registerWindowsRuntime() {
	registerNamespace("win")
	for k, v := range winHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range winExtraHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range winBuiltins {
		builtins[k] = v
	}
}
