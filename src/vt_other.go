//go:build !windows

package main

// Every other console understands ANSI escapes without being asked.
func enableVirtualTerminal() {}
