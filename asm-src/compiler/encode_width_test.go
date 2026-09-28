package main

import (
	"strings"
	"testing"
)

// TestEncoderWidths holds the instructions behind mem.read and mem.write
// and extern struct fields to GNU as, one line at a time. No example
// program is guaranteed to use every width, so they are listed here.
func TestEncoderWidths(t *testing.T) {
	as, _, binDir := findToolchainForTest(t)
	lines := []string{
		"movzx eax, byte ptr [rax]",
		"movzx eax, word ptr [rax]",
		"movzx eax, word ptr [r9]",
		"movsx rax, byte ptr [rax]",
		"movsx rax, word ptr [rax]",
		"movsx rax, word ptr [rcx]",
		"movsx rax, byte ptr [r8]",
		"movsxd rax, dword ptr [rax]",
		"mov eax, dword ptr [rax]",
		"mov byte ptr [rax], cl",
		"mov word ptr [rax], cx",
		"mov word ptr [r8], r9w",
		"mov dword ptr [rax], ecx",
		"mov dword ptr [rax], r10d",
		"mov qword ptr [rax], rcx",
		"cvtss2sd xmm0, dword ptr [rax]",
		"cvtss2sd xmm1, dword ptr [r11]",
		"cvtsd2ss xmm0, xmm0",
		"cvtsd2ss xmm1, xmm0",
		"movss dword ptr [rax], xmm0",
		"movss xmm0, dword ptr [rax]",
		"movss dword ptr [r10], xmm1",
		// callback stubs
		"movsxd rcx, ecx",
		"movsxd r9, r9d",
		"test edx, edx",
		"setne dl",
		"movzx edx, dl",
		"movzx r8d, r8b",
		"movsxd rax, dword ptr [rsp+40]",
		"mov qword ptr [rsp+48], rax",
		"mov eax, dword ptr [rsp+56]",
		// atomics, and the register swap the argument scheduler uses
		"lock xadd qword ptr [rcx], rax",
		"lock xadd qword ptr [r10], r11",
		"lock cmpxchg qword ptr [rcx], rdx",
		"lock cmpxchg qword ptr [r8+16], r9",
		"xchg qword ptr [rcx], rax",
		"xchg r8, r9",
		"sete al",
		"movzx eax, al",
	}
	for _, line := range lines {
		text := ".intel_syntax noprefix\n.text\n" + line + "\n"
		want, _ := assembleText(t, as, binDir, text)
		got := encodeText(t, text)
		if len(got) > len(want) || string(want[:len(got)]) != string(got) {
			t.Errorf("%s: encoded % x, the assembler says % x", line, got, want)
			continue
		}
		if rest := want[len(got):]; strings.Trim(string(rest), "\x00\x90") != "" {
			t.Errorf("%s: encoded % x, the assembler says % x", line, got, want)
		}
	}
}
