package main

// The Go backend's builtin table, presented to the shared type checker.
//
// The checker lives in ../../frontend and knows nothing about which
// backend it is serving. It asks a Library what a builtin's signature
// is; this is that Library for the 302 builtins backed by Go's standard
// library.
//
// Only the signature crosses the line. How a builtin is emitted, which
// Go imports it drags in and which runtime helpers it needs stay here,
// because none of it is the checker's business and none of it means
// anything to another backend.

import front "veylfront"

type goLibrary struct{}

func (goLibrary) Signature(name string) (front.Signature, bool) {
	b, ok := builtins[name]
	if !ok {
		return front.Signature{}, false
	}
	return front.Signature{
		Params:      b.params,
		Rest:        b.rest,
		Ret:         b.ret,
		RetOf:       b.retOf,
		Check:       b.check,
		HintFor:     b.hintFor,
		WantsTarget: b.wantsTarget,
	}, true
}

func (goLibrary) ConstType(name string) (*Type, bool) {
	bc, ok := builtinConsts[name]
	if !ok {
		return nil, false
	}
	return bc.typ, true
}
