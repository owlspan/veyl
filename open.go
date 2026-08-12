package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runOpen(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("open needs a .vy file")
	}

	path := args[0]
	action := ""
	for _, a := range args[1:] {
		switch a {
		case "--run", "-run":
			action = "run"
		case "--build", "-build":
			action = "build"
		case "--fmt", "-fmt":
			action = "fmt"
		case "--emit", "-emit":
			action = "emit"
		}
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		fmt.Printf("Cannot find %s\n", path)
		waitForEnter()
		return nil
	}
	if !strings.HasSuffix(strings.ToLower(abs), ".vy") {
		fmt.Printf("%s is not a Veyl file\n", filepath.Base(abs))
		waitForEnter()
		return nil
	}

	enableVirtualTerminal()

	if action == "" {
		action = chooseAction(abs)
	}

	switch action {
	case "run":
		doRun(abs)
	case "build":
		doBuild(abs)
	case "fmt":
		doSimple(abs, "fmt", "formatting", "formatted")
	case "emit":
		doSimple(abs, "emit", "generating Go", "done")
	case "folder":
		openFolder(filepath.Dir(abs))
		return nil
	default:
		return nil
	}

	waitForEnter()
	return nil
}

func chooseAction(abs string) string {
	name := filepath.Base(abs)
	exe := strings.TrimSuffix(name, filepath.Ext(name)) + ".exe"

	fmt.Printf("\n  %s\n", paint("Veyl "+Version, "1"))
	fmt.Printf("  %s\n\n", paint(name, "36"))
	fmt.Printf("    %s  Run it now\n", paint("[R]", "1"))
	fmt.Printf("    %s  Build %s, so it can run without Veyl\n", paint("[B]", "1"), exe)
	fmt.Printf("    %s  Open the folder\n", paint("[F]", "1"))
	fmt.Printf("    %s  Nothing, close this\n\n", paint("[Q]", "1"))

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("  Choose [R/B/F/Q]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return ""
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "r", "run", "":
			return "run"
		case "b", "build":
			return "build"
		case "f", "folder":
			return "folder"
		case "q", "quit", "exit":
			return ""
		}
		fmt.Println("  Not one of the options.")
	}
}

func doRun(abs string) {
	os.Chdir(filepath.Dir(abs))
	fmt.Printf("\n%s\n\n", paint("-- running -----------------------------", "90"))
	if err := run("run", abs, nil); err != nil {
		fmt.Fprintf(os.Stderr, "\nveyl: %v\n", err)
	}
	fmt.Printf("\n%s\n", paint("-- finished ----------------------------", "90"))
}

func doBuild(abs string) {
	os.Chdir(filepath.Dir(abs))
	fmt.Printf("\n%s\n\n", paint("-- building ----------------------------", "90"))
	if err := run("build", abs, nil); err != nil {
		fmt.Fprintf(os.Stderr, "\nveyl: %v\n", err)
		fmt.Printf("\n%s\n", paint("-- failed ------------------------------", "90"))
		return
	}
	fmt.Printf("\n%s\n", paint("-- done --------------------------------", "90"))
	fmt.Println("\n  That .exe is standalone. It needs neither Veyl nor Go\n  on the machine you copy it to.")
}

func doSimple(abs, cmd, doing, done string) {
	fmt.Printf("\n%s\n\n", paint("-- "+doing, "90"))
	if err := run(cmd, abs, nil); err != nil {
		fmt.Fprintf(os.Stderr, "\nveyl: %v\n", err)
		fmt.Printf("\n%s\n", paint("-- failed", "90"))
		return
	}
	fmt.Printf("\n%s\n", paint("-- "+done, "90"))
}

func openFolder(dir string) {
	_ = exec.Command("explorer", dir).Start()
}

func waitForEnter() {
	fmt.Print("\n  Press Enter to close this window...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
