//go:build windows

package oodle

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

var (
	libsMu sync.Mutex
	procs  = map[string]*syscall.LazyProc{}
)

func proc(lib string) (*syscall.LazyProc, error) {
	libsMu.Lock()
	defer libsMu.Unlock()
	if p, ok := procs[lib]; ok {
		return p, nil
	}
	p := syscall.NewLazyDLL(lib).NewProc("OodleLZ_Decompress")
	if err := p.Find(); err != nil {
		return nil, fmt.Errorf("loading %s: %w", lib, err)
	}
	procs[lib] = p
	return p, nil
}

// Decompress decodes src into exactly rawSize bytes using the library
// found for a file at near.
func Decompress(near string, src []byte, rawSize int) ([]byte, error) {
	lib := Find(near)
	if lib == "" {
		return nil, ErrMissing
	}
	p, err := proc(lib)
	if err != nil {
		return nil, err
	}
	if len(src) == 0 || rawSize <= 0 {
		return nil, fmt.Errorf("oodle: empty input")
	}
	// Oodle may write a little past the end while decoding.
	dst := make([]byte, rawSize+64)
	const (
		fuzzSafe    = 1
		threadPhase = 3 // OodleLZ_Decode_Unthreaded
	)
	r, _, _ := p.Call(
		uintptr(unsafe.Pointer(&src[0])), uintptr(len(src)),
		uintptr(unsafe.Pointer(&dst[0])), uintptr(rawSize),
		fuzzSafe, 0, 0, // fuzz safe, check CRC, verbosity
		0, 0, // decoder buffer base and size
		0, 0, // callback and its data
		0, 0, // scratch memory and its size
		threadPhase,
	)
	if int(r) != rawSize {
		return nil, fmt.Errorf("oodle: decoded %d of %d bytes", int(r), rawSize)
	}
	return dst[:rawSize], nil
}
