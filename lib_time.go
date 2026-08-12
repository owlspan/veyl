package main

// The time and mem libraries.
//
// time uses friendly format tokens - YYYY-MM-DD HH:mm:ss - rather than
// Go's reference-date layout. Go's "2006-01-02 15:04:05" is a genuinely
// good design that nobody can remember, and a language aimed at
// beginners should not make them look it up.

var timeHelperDefs = map[string]helperDef{
	"timeLayout": {
		// Longest tokens first, so YYYY is not eaten by YY.
		code: `var __layoutPairs = []string{
	"YYYY", "2006",
	"YY", "06",
	"MMMM", "January",
	"MMM", "Jan",
	"MM", "01",
	"DDDD", "Monday",
	"DDD", "Mon",
	"DD", "02",
	"HH", "15",
	"hh", "03",
	"mm", "04",
	"ss", "05",
	"AM", "PM",
	"ZZ", "-07:00",
}

func __toGoLayout(f string) string {
	return strings.NewReplacer(__layoutPairs...).Replace(f)
}

func __formatTime(unixSeconds int, format string) string {
	return time.Unix(int64(unixSeconds), 0).Format(__toGoLayout(format))
}

func __parseTime(text string, format string) int {
	t, err := time.ParseInLocation(__toGoLayout(format), text, time.Local)
	if err != nil {
		return -1
	}
	return int(t.Unix())
}`,
		imports: []string{"strings", "time"},
	},

	"memStats": {
		code: `func __memStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}`,
		imports: []string{"runtime"},
	},
}

var timeBuiltins map[string]builtin

func buildTimeBuiltins() {
	timeBuiltins = map[string]builtin{

		// ---- time ----

		"time.now": {
			emit:    func(a []string) string { return "int(time.Now().Unix())" },
			ret:     Int,
			imports: []string{"time"},
		},
		"time.millis": {
			emit:    func(a []string) string { return "int(time.Now().UnixMilli())" },
			ret:     Int,
			imports: []string{"time"},
		},
		"time.nanos": {
			emit:    func(a []string) string { return "int(time.Now().UnixNano())" },
			ret:     Int,
			imports: []string{"time"},
		},
		"time.format": {
			emit:   func(a []string) string { return "__formatTime(" + a[0] + ", " + a[1] + ")" },
			params: []*Type{Int, Str}, ret: Str, minArgs: 2, maxArgs: 2,
			helpers: []string{"timeLayout"},
		},
		"time.parse": {
			emit:   func(a []string) string { return "__parseTime(" + a[0] + ", " + a[1] + ")" },
			params: []*Type{Str, Str}, ret: Int, minArgs: 2, maxArgs: 2,
			helpers: []string{"timeLayout"},
		},
		"time.date": {
			emit:    func(a []string) string { return `time.Now().Format("2006-01-02")` },
			ret:     Str,
			imports: []string{"time"},
		},
		"time.clock": {
			emit:    func(a []string) string { return `time.Now().Format("15:04:05")` },
			ret:     Str,
			imports: []string{"time"},
		},
		"time.stamp": {
			emit:    func(a []string) string { return `time.Now().Format("2006-01-02 15:04:05")` },
			ret:     Str,
			imports: []string{"time"},
		},
		"time.since": {
			emit: func(a []string) string {
				return "(int(time.Now().Unix()) - " + a[0] + ")"
			},
			params: []*Type{Int}, ret: Int, minArgs: 1, maxArgs: 1,
			imports: []string{"time"},
		},
		"time.sleep": {
			emit: func(a []string) string {
				return "time.Sleep(time.Duration(" + a[0] + ") * time.Millisecond)"
			},
			params: []*Type{Int}, ret: Void, minArgs: 1, maxArgs: 1,
			imports: []string{"time"},
		},
		"time.year": {
			emit:    func(a []string) string { return "time.Now().Year()" },
			ret:     Int,
			imports: []string{"time"},
		},
		"time.month": {
			emit:    func(a []string) string { return "int(time.Now().Month())" },
			ret:     Int,
			imports: []string{"time"},
		},
		"time.day": {
			emit:    func(a []string) string { return "time.Now().Day()" },
			ret:     Int,
			imports: []string{"time"},
		},
		"time.weekday": {
			emit:    func(a []string) string { return "time.Now().Weekday().String()" },
			ret:     Str,
			imports: []string{"time"},
		},

		// ---- mem ----
		//
		// Quartz is garbage collected, so these report and nudge rather
		// than allocate and free. Manual memory needs the C backend.

		"mem.used": {
			emit:    func(a []string) string { return "int(__memStats().Alloc)" },
			ret:     Int,
			helpers: []string{"memStats"},
		},
		"mem.total": {
			emit:    func(a []string) string { return "int(__memStats().TotalAlloc)" },
			ret:     Int,
			helpers: []string{"memStats"},
		},
		"mem.system": {
			emit:    func(a []string) string { return "int(__memStats().Sys)" },
			ret:     Int,
			helpers: []string{"memStats"},
		},
		"mem.objects": {
			emit:    func(a []string) string { return "int(__memStats().Mallocs - __memStats().Frees)" },
			ret:     Int,
			helpers: []string{"memStats"},
		},
		"mem.collections": {
			emit:    func(a []string) string { return "int(__memStats().NumGC)" },
			ret:     Int,
			helpers: []string{"memStats"},
		},
		"mem.collect": {
			emit:    func(a []string) string { return "runtime.GC()" },
			ret:     Void,
			imports: []string{"runtime"},
		},
		"mem.goroutines": {
			emit:    func(a []string) string { return "runtime.NumGoroutine()" },
			ret:     Int,
			imports: []string{"runtime"},
		},
	}
}

func registerTime() {
	buildTimeBuiltins()
	registerNamespace("time", "mem")
	for k, v := range timeHelperDefs {
		helperDefs[k] = v
	}
	for k, v := range timeBuiltins {
		builtins[k] = v
	}
}
