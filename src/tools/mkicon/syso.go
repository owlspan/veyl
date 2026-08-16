package main

import (
	"bytes"
	"encoding/binary"
	"os"
)

// A .syso is a COFF object file that the Go linker picks up from the
// package directory and merges into the executable. Putting an icon
// resource in one is how a Go program gets an Explorer icon without
// cgo, a manifest tool, or a third-party dependency.
//
// The format is nested three deep: a type directory (RT_ICON,
// RT_GROUP_ICON), then a name directory per resource, then a language
// directory whose entries point at the actual bytes. Windows requires
// entries at every level to be sorted by ID, which is why the sizes
// are emitted in the order they are given rather than sorted later.
//
// One subtlety makes this more than a byte-packing exercise: the
// OffsetToData field of a data entry is a virtual address, not a file
// offset, and an object file has no idea where it will be loaded. Each
// one therefore needs a relocation of type IMAGE_REL_AMD64_ADDR32NB
// against the section symbol, and the linker fills in the real value.
// Skip those and the resources are silently present but unreadable.

const (
	rtIcon      = 3
	rtGroupIcon = 14
	langEnglish = 0x0409

	dirSize      = 16 // IMAGE_RESOURCE_DIRECTORY
	dirEntrySize = 8  // IMAGE_RESOURCE_DIRECTORY_ENTRY
	dataEntrySz  = 16 // IMAGE_RESOURCE_DATA_ENTRY

	relocAddr32NB  = 0x0003
	machineAMD64   = 0x8664
	scnInitData    = 0x00000040
	scnMemRead     = 0x40000000
	symClassStatic = 3
)

// groupIcon builds the RT_GROUP_ICON payload: the directory Windows
// reads to decide which RT_ICON to display at a given size. It mirrors
// the .ico header, except each entry ends with a resource ID instead of
// a file offset.
func groupIcon(imgs []iconImage) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&b, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&b, binary.LittleEndian, uint16(len(imgs)))
	for i, im := range imgs {
		b.WriteByte(byteSize(im.size))
		b.WriteByte(byteSize(im.size))
		b.WriteByte(0)                                    // palette size
		b.WriteByte(0)                                    // reserved
		binary.Write(&b, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&b, binary.LittleEndian, uint16(32)) // bpp
		binary.Write(&b, binary.LittleEndian, uint32(len(im.png)))
		binary.Write(&b, binary.LittleEndian, uint16(i+1)) // RT_ICON id
	}
	return b.Bytes()
}

func align(n, to int) int {
	if r := n % to; r != 0 {
		return n + to - r
	}
	return n
}

// writeSyso emits the resource object next to the package sources.
func writeSyso(path string, imgs []iconImage) error {
	n := len(imgs)
	group := groupIcon(imgs)

	// Lay the section out before writing any of it, because directory
	// entries have to point forwards at structures not yet emitted.
	var (
		offRoot      = 0
		offIconType  = offRoot + dirSize + 2*dirEntrySize
		offGroupType = offIconType + dirSize + n*dirEntrySize
		offNameDirs  = offGroupType + dirSize + dirEntrySize
		nameDirSize  = dirSize + dirEntrySize
		offDataEntry = offNameDirs + (n+1)*nameDirSize
		offBlobs     = offDataEntry + (n+1)*dataEntrySz
	)

	// Where each payload lands, and the offsets needing relocation.
	blobOff := make([]int, n+1)
	cur := offBlobs
	for i, im := range imgs {
		blobOff[i] = cur
		cur = align(cur+len(im.png), 8)
	}
	blobOff[n] = cur
	cur = align(cur+len(group), 8)
	sectionSize := cur

	sec := make([]byte, sectionSize)
	put16 := func(off int, v uint16) { binary.LittleEndian.PutUint16(sec[off:], v) }
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(sec[off:], v) }

	// A resource directory header: only the two counts matter here.
	writeDir := func(off, named, ids int) {
		put16(off+12, uint16(named))
		put16(off+14, uint16(ids))
	}
	// An entry pointing at a nested directory has the high bit set.
	writeSubdirEntry := func(off int, id, target int) {
		put32(off, uint32(id))
		put32(off+4, uint32(target)|0x80000000)
	}
	writeLeafEntry := func(off int, id, target int) {
		put32(off, uint32(id))
		put32(off+4, uint32(target))
	}

	// Level 1: the two resource types, ascending by ID.
	writeDir(offRoot, 0, 2)
	writeSubdirEntry(offRoot+dirSize, rtIcon, offIconType)
	writeSubdirEntry(offRoot+dirSize+dirEntrySize, rtGroupIcon, offGroupType)

	// Level 2: one name per icon, plus one for the group.
	writeDir(offIconType, 0, n)
	for i := 0; i < n; i++ {
		writeSubdirEntry(offIconType+dirSize+i*dirEntrySize, i+1, offNameDirs+i*nameDirSize)
	}
	writeDir(offGroupType, 0, 1)
	writeSubdirEntry(offGroupType+dirSize, 1, offNameDirs+n*nameDirSize)

	// Level 3: language, whose entries point at the data descriptors.
	for i := 0; i <= n; i++ {
		off := offNameDirs + i*nameDirSize
		writeDir(off, 0, 1)
		writeLeafEntry(off+dirSize, langEnglish, offDataEntry+i*dataEntrySz)
	}

	// The data descriptors. OffsetToData is written as a plain section
	// offset and turned into an RVA by the relocations below.
	var relocs []int
	for i, im := range imgs {
		off := offDataEntry + i*dataEntrySz
		put32(off, uint32(blobOff[i]))
		put32(off+4, uint32(len(im.png)))
		relocs = append(relocs, off)
		copy(sec[blobOff[i]:], im.png)
	}
	groupEntry := offDataEntry + n*dataEntrySz
	put32(groupEntry, uint32(blobOff[n]))
	put32(groupEntry+4, uint32(len(group)))
	relocs = append(relocs, groupEntry)
	copy(sec[blobOff[n]:], group)

	// Now the object file around it.
	var out bytes.Buffer
	w16 := func(v uint16) { binary.Write(&out, binary.LittleEndian, v) }
	w32 := func(v uint32) { binary.Write(&out, binary.LittleEndian, v) }

	const headerSize = 20 + 40
	ptrRaw := headerSize
	ptrReloc := ptrRaw + sectionSize
	ptrSyms := ptrReloc + 10*len(relocs)

	// COFF header.
	w16(machineAMD64)
	w16(1) // one section
	w32(0) // timestamp: zero keeps the build reproducible
	w32(uint32(ptrSyms))
	w32(1) // one symbol
	w16(0) // no optional header
	w16(0) // characteristics

	// Section header.
	out.WriteString(".rsrc\x00\x00\x00")
	w32(0) // VirtualSize
	w32(0) // VirtualAddress
	w32(uint32(sectionSize))
	w32(uint32(ptrRaw))
	w32(uint32(ptrReloc))
	w32(0) // line numbers
	w16(uint16(len(relocs)))
	w16(0)
	w32(scnInitData | scnMemRead)

	out.Write(sec)

	// Relocations, all against the section symbol at index 0.
	for _, off := range relocs {
		w32(uint32(off))
		w32(0)
		w16(relocAddr32NB)
	}

	// The section symbol itself.
	out.WriteString(".rsrc\x00\x00\x00")
	w32(0) // value
	w16(1) // section number, one-based
	w16(0) // type
	out.WriteByte(symClassStatic)
	out.WriteByte(0) // no auxiliary records

	w32(4) // string table: just its own length, i.e. empty

	return os.WriteFile(path, out.Bytes(), 0o644)
}
