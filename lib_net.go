package main

// The http and net libraries.
//
// Both are built on Go's standard library, so they add no dependency —
// `net/http` and `net` ship with the toolchain. Everything here obeys
// the same failure convention as the os library: a call that returns a
// value is fatal on failure unless its name ends in Or, and a call that
// merely acts returns a bool.
//
// Every network call carries a timeout. A script that hangs forever on
// an unreachable host is worse than one that fails.

var netHelperDefs = map[string]helperDef{
	"httpClient": {
		// One shared client, so connections are pooled and every call
		// inherits the same timeout.
		code: `var __http = &http.Client{Timeout: 30 * time.Second}

func __httpDo(method string, url string, contentType string, body string, headers map[string]string) (string, int, error) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return "", 0, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := __http.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(data), resp.StatusCode, nil
}`,
		imports: []string{"io", "net/http", "strings", "time"},
	},

	"httpGet": {
		// A 4xx or 5xx is a failure with a reason, not a silent empty
		// body — the status is the most useful thing to say.
		code: `func __httpGet(url string, headers map[string]string) __Res[string] {
	body, status, err := __httpDo("GET", url, "", "", headers)
	if err != nil {
		return __fail[string](__why("fetch", url, err))
	}
	if status >= 400 {
		return __fail[string](fmt.Sprintf("cannot fetch %q: server replied %d", url, status))
	}
	return __ok(body)
}`,
		imports: []string{"fmt"},
		deps:    []string{"httpClient", "qzWhy", "result"},
	},
	"httpGetOr": {
		code: `func __httpGetOr(url string, fallback string) string {
	body, status, err := __httpDo("GET", url, "", "", nil)
	if err != nil || status >= 400 {
		return fallback
	}
	return body
}`,
		deps: []string{"httpClient"},
	},
	"httpPost": {
		code: `func __httpPost(url string, contentType string, body string) __Res[string] {
	out, status, err := __httpDo("POST", url, contentType, body, nil)
	if err != nil {
		return __fail[string](__why("post to", url, err))
	}
	if status >= 400 {
		return __fail[string](fmt.Sprintf("cannot post to %q: server replied %d", url, status))
	}
	return __ok(out)
}`,
		imports: []string{"fmt"},
		deps:    []string{"httpClient", "qzWhy", "result"},
	},
	"httpStatus": {
		// 0 means the request never completed, which is a different thing
		// from a server answering with an error code.
		code: `func __httpStatus(url string) int {
	_, status, err := __httpDo("GET", url, "", "", nil)
	if err != nil {
		return 0
	}
	return status
}`,
		deps: []string{"httpClient"},
	},
	"httpDownload": {
		code: `func __httpDownload(url string, path string) bool {
	resp, err := __http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false
	}
	f, err := os.Create(path)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err == nil
}`,
		imports: []string{"io", "os"},
		deps:    []string{"httpClient"},
	},
	"urlEncode": {
		code: `func __urlEncode(s string) string { return url.QueryEscape(s) }

func __urlDecode(s string) string {
	out, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return out
}`,
		imports: []string{"net/url"},
	},

	// ---- net ----

	"netLookup": {
		code: `func __lookupIPs(host string) __Res[[]string] {
	addrs, err := net.LookupHost(host)
	if err != nil {
		return __fail[[]string](__why("resolve", host, err))
	}
	sort.Strings(addrs)
	return __ok(addrs)
}

func __lookupNames(ip string) []string {
	names, err := net.LookupAddr(ip)
	if err != nil {
		return []string{}
	}
	for i, n := range names {
		names[i] = strings.TrimSuffix(n, ".")
	}
	sort.Strings(names)
	return names
}`,
		imports: []string{"net", "sort", "strings"},
		deps:    []string{"qzWhy", "result"},
	},
	"netConnect": {
		code: `func __canConnect(host string, port int, timeoutMs int) bool {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeoutMs)*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}`,
		imports: []string{"net", "strconv", "time"},
	},
	"netScan": {
		// Ports are probed concurrently, because doing it in sequence
		// makes any useful range take minutes. The worker count is capped
		// so a wide range does not open thousands of sockets at once.
		code: `func __scanPorts(host string, lo int, hi int, timeoutMs int) []int {
	if hi < lo {
		lo, hi = hi, lo
	}
	if lo < 1 {
		lo = 1
	}
	if hi > 65535 {
		hi = 65535
	}

	ports := make(chan int)
	var mu sync.Mutex
	var open []int
	var wg sync.WaitGroup

	workers := 256
	if n := hi - lo + 1; n < workers {
		workers = n
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range ports {
				if __canConnect(host, p, timeoutMs) {
					mu.Lock()
					open = append(open, p)
					mu.Unlock()
				}
			}
		}()
	}
	for p := lo; p <= hi; p++ {
		ports <- p
	}
	close(ports)
	wg.Wait()

	sort.Ints(open)
	return open
}`,
		imports: []string{"sort", "sync"},
		deps:    []string{"netConnect"},
	},
	"netSend": {
		code: `func __tcpSend(host string, port int, data string, timeoutMs int) __Res[string] {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	timeout := time.Duration(timeoutMs) * time.Millisecond
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return __fail[string](__why("connect to", addr, err))
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write([]byte(data)); err != nil {
		return __fail[string](__why("send to", addr, err))
	}
	out, _ := io.ReadAll(conn)
	return __ok(string(out))
}`,
		imports: []string{"io", "net", "strconv", "time"},
		deps:    []string{"qzWhy", "result"},
	},
	"netLocalIP": {
		// No packet is sent: dialling a UDP address only picks the route,
		// which is the cheapest way to learn which interface would be used.
		code: `func __localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}`,
		imports: []string{"net"},
	},
	"netInterfaces": {
		code: `func __interfaceIPs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return []string{}
	}
	var out []string
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			out = append(out, ipnet.IP.String())
		}
	}
	sort.Strings(out)
	if out == nil {
		return []string{}
	}
	return out
}`,
		imports: []string{"net", "sort"},
	},
}

var netBuiltins map[string]builtin

func buildNetBuiltins() {
	netBuiltins = map[string]builtin{

		// ---- http ----

		"http.get": {
			emit:   func(a []string) string { return "__httpGet(" + a[0] + ", nil)" },
			params: []*Type{Str}, ret: ResultOf(Str), minArgs: 1, maxArgs: 1,
			helpers: []string{"httpGet"},
		},
		"http.getWith": {
			emit:   func(a []string) string { return "__httpGet(" + a[0] + ", " + a[1] + ")" },
			params: []*Type{Str, MapOf(Str, Str)}, ret: ResultOf(Str), minArgs: 2, maxArgs: 2,
			helpers: []string{"httpGet"},
		},
		"http.getOr": {
			emit:   func(a []string) string { return "__httpGetOr(" + a[0] + ", " + a[1] + ")" },
			params: []*Type{Str, Str}, ret: Str, minArgs: 2, maxArgs: 2,
			helpers: []string{"httpGetOr"},
		},
		"http.post": {
			emit: func(a []string) string {
				if len(a) == 2 {
					return `__httpPost(` + a[0] + `, "text/plain", ` + a[1] + `)`
				}
				return "__httpPost(" + a[0] + ", " + a[2] + ", " + a[1] + ")"
			},
			params: []*Type{Str, Str, Str}, ret: ResultOf(Str), minArgs: 2, maxArgs: 3,
			helpers: []string{"httpPost"},
		},
		"http.postJson": {
			emit: func(a []string) string {
				return `__httpPost(` + a[0] + `, "application/json", ` + a[1] + `)`
			},
			params: []*Type{Str, Str}, ret: ResultOf(Str), minArgs: 2, maxArgs: 2,
			helpers: []string{"httpPost"},
		},
		"http.status": {
			emit:   func(a []string) string { return "__httpStatus(" + a[0] + ")" },
			params: []*Type{Str}, ret: Int, minArgs: 1, maxArgs: 1,
			helpers: []string{"httpStatus"},
		},
		"http.ok": {
			emit: func(a []string) string {
				return "func() bool { s := __httpStatus(" + a[0] + "); return s >= 200 && s < 400 }()"
			},
			params: []*Type{Str}, ret: Bool, minArgs: 1, maxArgs: 1,
			helpers: []string{"httpStatus"},
		},
		"http.download": {
			emit:   func(a []string) string { return "__httpDownload(" + a[0] + ", " + a[1] + ")" },
			params: []*Type{Str, Str}, ret: Bool, minArgs: 2, maxArgs: 2,
			helpers: []string{"httpDownload"},
		},
		"http.encode": {
			emit:   func(a []string) string { return "__urlEncode(" + a[0] + ")" },
			params: []*Type{Str}, ret: Str, minArgs: 1, maxArgs: 1,
			helpers: []string{"urlEncode"},
		},
		"http.decode": {
			emit:   func(a []string) string { return "__urlDecode(" + a[0] + ")" },
			params: []*Type{Str}, ret: Str, minArgs: 1, maxArgs: 1,
			helpers: []string{"urlEncode"},
		},

		// ---- net ----

		"net.ips": {
			emit:   func(a []string) string { return "__lookupIPs(" + a[0] + ")" },
			params: []*Type{Str}, ret: ResultOf(ListOf(Str)), minArgs: 1, maxArgs: 1,
			helpers: []string{"netLookup"},
		},
		"net.ip": {
			emit: func(a []string) string {
				return "func() __Res[string] { r := __lookupIPs(" + a[0] +
					"); if r.e != \"\" { return __fail[string](r.e) }; " +
					"if len(r.v) == 0 { return __fail[string](\"no addresses for \" + " + a[0] + ") }; " +
					"return __ok(r.v[0]) }()"
			},
			params: []*Type{Str}, ret: ResultOf(Str), minArgs: 1, maxArgs: 1,
			helpers: []string{"netLookup"},
		},
		"net.names": {
			emit:   func(a []string) string { return "__lookupNames(" + a[0] + ")" },
			params: []*Type{Str}, ret: ListOf(Str), minArgs: 1, maxArgs: 1,
			helpers: []string{"netLookup"},
		},
		"net.canConnect": {
			emit: func(a []string) string {
				timeout := "2000"
				if len(a) == 3 {
					timeout = a[2]
				}
				return "__canConnect(" + a[0] + ", " + a[1] + ", " + timeout + ")"
			},
			params: []*Type{Str, Int, Int}, ret: Bool, minArgs: 2, maxArgs: 3,
			helpers: []string{"netConnect"},
		},
		"net.scan": {
			emit: func(a []string) string {
				timeout := "500"
				if len(a) == 4 {
					timeout = a[3]
				}
				return "__scanPorts(" + a[0] + ", " + a[1] + ", " + a[2] + ", " + timeout + ")"
			},
			params: []*Type{Str, Int, Int, Int}, ret: ListOf(Int), minArgs: 3, maxArgs: 4,
			helpers: []string{"netScan"},
		},
		"net.send": {
			emit: func(a []string) string {
				timeout := "5000"
				if len(a) == 4 {
					timeout = a[3]
				}
				return "__tcpSend(" + a[0] + ", " + a[1] + ", " + a[2] + ", " + timeout + ")"
			},
			params: []*Type{Str, Int, Str, Int}, ret: ResultOf(Str), minArgs: 3, maxArgs: 4,
			helpers: []string{"netSend"},
		},
		"net.localIP": {
			emit:    func(a []string) string { return "__localIP()" },
			ret:     Str,
			helpers: []string{"netLocalIP"},
		},
		"net.interfaces": {
			emit:    func(a []string) string { return "__interfaceIPs()" },
			ret:     ListOf(Str),
			helpers: []string{"netInterfaces"},
		},
	}
}

func registerNet() {
	buildNetBuiltins()
	registerNamespace("http", "net")
	for k, v := range netHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range netBuiltins {
		builtins[k] = v
	}
}
