package main

// The PE writer: a laid-out image to an executable Windows will run.
//
// This is what removes MinGW. link.go has already turned the assembly
// into sections and knows where every label sits relative to the start
// of its section; here those sections are given addresses in one image,
// every reference is filled in, and the whole thing is written in the
// format the loader reads.
//
// Two decisions keep it short.
//
// The image is fixed at one base address and says so, by setting
// RELOCS_STRIPPED and not asking for a dynamic base. Every reference
// this compiler emits is either a direct call or rip-relative, so
// nothing in the file holds an absolute address and there is no base
// relocation table to write. The cost is that the executable is not
// ASLR'd, which is a security property worth having and worth adding
// later; the gain is that a whole section and a whole pass do not exist.
//
// Imports come from msvcrt.dll and kernel32.dll, both of which ship with
// Windows on every machine that can run the result. That is the point:
// a self-written import table against DLLs that are already there needs
// no toolchain and no redistributable, so the dependency goes away
// rather than moving somewhere else.

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
)

const (
	// A high base, the conventional one for a 64-bit executable, chosen
	// so that a null pointer dereference cannot land anywhere mapped.
	imageBase = 0x140000000

	sectionAlignment = 0x1000
	fileAlignment    = 0x200

	// The name Windows starts the program at. x64.go writes it: it sets
	// the stack up, calls main, and hands main's result to exit.
	entrySymbol = "__start"
)

func alignUp(n, to int) int {
	if n%to == 0 {
		return n
	}
	return n + to - n%to
}

// importOverride is for a foreign symbol the rule below gets wrong.
// There are none yet.
var importOverride = map[string]string{}

// importDLL says which library a foreign symbol comes from.
//
// The rule is the one the two libraries themselves follow: a Win32 API
// is named in PascalCase and a C runtime function is not, so an initial
// capital means kernel32 and anything else means msvcrt. It holds for
// every symbol this compiler emits and for the wider API either side of
// them, which is why it is a rule here rather than a list that would
// have to be edited every time a library function is added.
func importDLL(sym string) string {
	if dll, ok := importOverride[sym]; ok {
		return dll
	}
	if sym != "" && sym[0] >= 'A' && sym[0] <= 'Z' {
		return "kernel32.dll"
	}
	return "msvcrt.dll"
}

// A section as it will appear in the file.
type peSection struct {
	name  string
	rva   int
	vsize int
	data  []byte // nil means uninitialised, as .bss is
	chars uint32
}

const (
	scnCode    = 0x00000020
	scnData    = 0x00000040
	scnBSS     = 0x00000080
	scnExecute = 0x20000000
	scnRead    = 0x40000000
	scnWrite   = 0x80000000
)

// writePE lays the object out and writes an executable.
func writePE(obj *object, out string) error {
	text := append([]byte(nil), obj.text...)

	textRVA := sectionAlignment
	rdataRVA := alignUp(textRVA+len(text), sectionAlignment)
	idataRVA := alignUp(rdataRVA+len(obj.rdata), sectionAlignment)

	idata, slots, iatRVA, iatSize := buildImports(obj.externs, idataRVA)

	bssRVA := alignUp(idataRVA+len(idata), sectionAlignment)

	// Where each section begins, so a symbol's offset becomes an RVA.
	base := map[secID]int{
		secText:  textRVA,
		secRdata: rdataRVA,
		secBss:   bssRVA,
	}

	resolve := func(name string) (int, bool) {
		if rva, ok := slots[name]; ok {
			return rva, true
		}
		s, ok := obj.sym[name]
		if !ok {
			return 0, false
		}
		return base[s.sec] + s.off, true
	}

	for _, r := range obj.relocs {
		target, ok := resolve(r.sym)
		if !ok {
			return fmt.Errorf("nothing defines %s", r.sym)
		}
		// Every reference left in the text is rel32, whether it is a
		// call or a rip-relative load, and both count from the end of
		// the instruction. Whatever is already in the field is the
		// addend the encoder put there.
		addend := int32(binary.LittleEndian.Uint32(text[r.at : r.at+4]))
		delta := target - (textRVA + r.next) + int(addend)
		if delta < -(1<<31) || delta >= 1<<31 {
			return fmt.Errorf("%s is too far away to reach", r.sym)
		}
		binary.LittleEndian.PutUint32(text[r.at:r.at+4], uint32(int32(delta)))
	}

	entry, ok := resolve(entrySymbol)
	if !ok {
		return fmt.Errorf("the program has no %s", entrySymbol)
	}

	sections := []peSection{
		{".text", textRVA, len(text), text, scnCode | scnExecute | scnRead},
		{".rdata", rdataRVA, len(obj.rdata), obj.rdata, scnData | scnRead},
		{".idata", idataRVA, len(idata), idata, scnData | scnRead | scnWrite},
	}
	if obj.bssLen > 0 {
		sections = append(sections,
			peSection{".bss", bssRVA, obj.bssLen, nil, scnBSS | scnRead | scnWrite})
	}

	return writeImage(out, sections, entry, idataRVA, len(idata), iatRVA, iatSize)
}

// buildImports produces the import section: one descriptor per library,
// a lookup table and an address table per library, and the name of every
// symbol.
//
// The two tables start out identical. The loader reads the lookup table
// to find out what to import and overwrites the address table with the
// addresses it found, which is why the call thunks in link.go point at
// the second one.
func buildImports(externs []string, rva int) (blob []byte, slots map[string]int, iatRVA, iatSize int) {
	byDLL := map[string][]string{}
	for _, sym := range externs {
		dll := importDLL(sym)
		byDLL[dll] = append(byDLL[dll], sym)
	}
	dlls := make([]string, 0, len(byDLL))
	for name := range byDLL {
		dlls = append(dlls, name)
	}
	sort.Strings(dlls)

	// Sizes first, so every part knows its own address before any of it
	// is written.
	descSize := (len(dlls) + 1) * 20
	tableSize := 0
	for _, dll := range dlls {
		tableSize += (len(byDLL[dll]) + 1) * 8
	}

	iltAt := descSize
	iatAt := iltAt + tableSize
	namesAt := iatAt + tableSize

	// The hint/name entries, and where each one ends up.
	nameOf := map[string]int{}
	var names []byte
	for _, dll := range dlls {
		for _, sym := range byDLL[dll] {
			nameOf[sym] = namesAt + len(names)
			names = append(names, 0, 0) // the hint, which may be zero
			names = append(names, []byte(sym)...)
			names = append(names, 0)
			if len(names)%2 != 0 {
				names = append(names, 0)
			}
		}
	}
	dllNameAt := map[string]int{}
	for _, dll := range dlls {
		dllNameAt[dll] = namesAt + len(names)
		names = append(names, []byte(dll)...)
		names = append(names, 0)
		if len(names)%2 != 0 {
			names = append(names, 0)
		}
	}

	blob = make([]byte, namesAt+len(names))
	copy(blob[namesAt:], names)

	slots = map[string]int{}
	ilt, iat := iltAt, iatAt
	for i, dll := range dlls {
		d := i * 20
		binary.LittleEndian.PutUint32(blob[d+0:], uint32(rva+ilt))
		binary.LittleEndian.PutUint32(blob[d+12:], uint32(rva+dllNameAt[dll]))
		binary.LittleEndian.PutUint32(blob[d+16:], uint32(rva+iat))

		for _, sym := range byDLL[dll] {
			binary.LittleEndian.PutUint64(blob[ilt:], uint64(rva+nameOf[sym]))
			binary.LittleEndian.PutUint64(blob[iat:], uint64(rva+nameOf[sym]))
			slots[iatSym(sym)] = rva + iat
			ilt += 8
			iat += 8
		}
		// The terminating zero of each table, which is already zero.
		ilt += 8
		iat += 8
	}

	return blob, slots, rva + iatAt, tableSize
}

// writeImage assembles the headers and the section bodies into a file.
func writeImage(out string, sections []peSection, entry, importRVA, importSize, iatRVA, iatSize int) error {
	headerSize := alignUp(0x40+4+20+240+len(sections)*40, fileAlignment)

	// File offsets. A section with no initialised data occupies none.
	at := headerSize
	offsets := make([]int, len(sections))
	for i, s := range sections {
		if s.data == nil {
			continue
		}
		offsets[i] = at
		at += alignUp(len(s.data), fileAlignment)
	}
	fileSize := at

	last := sections[len(sections)-1]
	imageSize := alignUp(last.rva+last.vsize, sectionAlignment)

	var codeSize, initSize, uninitSize int
	for _, s := range sections {
		switch {
		case s.chars&scnCode != 0:
			codeSize += alignUp(len(s.data), fileAlignment)
		case s.chars&scnBSS != 0:
			uninitSize += alignUp(s.vsize, fileAlignment)
		default:
			initSize += alignUp(len(s.data), fileAlignment)
		}
	}

	buf := make([]byte, fileSize)

	// The DOS header. Only the magic and the offset at 0x3c matter to
	// Windows; the rest of the sixty-four bytes would be the stub's
	// business, and there is no stub, so running this under DOS prints
	// nothing rather than a message.
	copy(buf, "MZ")
	binary.LittleEndian.PutUint16(buf[0x02:], 0x90)
	binary.LittleEndian.PutUint16(buf[0x04:], 3)
	binary.LittleEndian.PutUint16(buf[0x08:], 4)
	binary.LittleEndian.PutUint16(buf[0x0c:], 0xffff)
	binary.LittleEndian.PutUint16(buf[0x18:], 0x40)
	binary.LittleEndian.PutUint32(buf[0x3c:], 0x40)

	p := 0x40
	copy(buf[p:], []byte{'P', 'E', 0, 0})
	p += 4

	// The COFF header.
	binary.LittleEndian.PutUint16(buf[p+0:], 0x8664) // x86-64
	binary.LittleEndian.PutUint16(buf[p+2:], uint16(len(sections)))
	binary.LittleEndian.PutUint16(buf[p+16:], 240) // optional header size
	// EXECUTABLE_IMAGE | LARGE_ADDRESS_AWARE | RELOCS_STRIPPED.
	binary.LittleEndian.PutUint16(buf[p+18:], 0x0002|0x0020|0x0001)
	p += 20

	// The optional header, which is not optional.
	o := p
	binary.LittleEndian.PutUint16(buf[o+0:], 0x20b) // PE32+
	buf[o+2] = 14                                   // a linker version, for tools that print one
	binary.LittleEndian.PutUint32(buf[o+4:], uint32(codeSize))
	binary.LittleEndian.PutUint32(buf[o+8:], uint32(initSize))
	binary.LittleEndian.PutUint32(buf[o+12:], uint32(uninitSize))
	binary.LittleEndian.PutUint32(buf[o+16:], uint32(entry))
	binary.LittleEndian.PutUint32(buf[o+20:], uint32(sections[0].rva))
	binary.LittleEndian.PutUint64(buf[o+24:], imageBase)
	binary.LittleEndian.PutUint32(buf[o+32:], sectionAlignment)
	binary.LittleEndian.PutUint32(buf[o+36:], fileAlignment)
	binary.LittleEndian.PutUint16(buf[o+40:], 6) // major OS version
	binary.LittleEndian.PutUint16(buf[o+48:], 6) // major subsystem version
	binary.LittleEndian.PutUint32(buf[o+56:], uint32(imageSize))
	binary.LittleEndian.PutUint32(buf[o+60:], uint32(headerSize))
	binary.LittleEndian.PutUint16(buf[o+68:], 3) // console subsystem
	// NX_COMPAT | TERMINAL_SERVER_AWARE. No dynamic base: the image has
	// no relocations to apply, so it has to load where it says it does.
	binary.LittleEndian.PutUint16(buf[o+70:], 0x0100|0x8000)
	binary.LittleEndian.PutUint64(buf[o+72:], 0x100000) // stack reserve
	binary.LittleEndian.PutUint64(buf[o+80:], 0x1000)   // stack commit
	binary.LittleEndian.PutUint64(buf[o+88:], 0x100000) // heap reserve
	binary.LittleEndian.PutUint64(buf[o+96:], 0x1000)   // heap commit
	binary.LittleEndian.PutUint32(buf[o+108:], 16)      // data directories

	dirs := o + 112
	binary.LittleEndian.PutUint32(buf[dirs+1*8+0:], uint32(importRVA))
	binary.LittleEndian.PutUint32(buf[dirs+1*8+4:], uint32(importSize))
	binary.LittleEndian.PutUint32(buf[dirs+12*8+0:], uint32(iatRVA))
	binary.LittleEndian.PutUint32(buf[dirs+12*8+4:], uint32(iatSize))

	p = o + 240
	for i, s := range sections {
		h := p + i*40
		copy(buf[h:h+8], s.name)
		binary.LittleEndian.PutUint32(buf[h+8:], uint32(s.vsize))
		binary.LittleEndian.PutUint32(buf[h+12:], uint32(s.rva))
		if s.data != nil {
			binary.LittleEndian.PutUint32(buf[h+16:], uint32(alignUp(len(s.data), fileAlignment)))
			binary.LittleEndian.PutUint32(buf[h+20:], uint32(offsets[i]))
			copy(buf[offsets[i]:], s.data)
		}
		binary.LittleEndian.PutUint32(buf[h+36:], s.chars)
	}

	return os.WriteFile(out, buf, 0o755)
}
