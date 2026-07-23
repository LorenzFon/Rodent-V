package main

import "unsafe"

func ttPrefetch(addr unsafe.Pointer)

func (t *TTable) prefetch(key uint64) {
	ttPrefetch(unsafe.Pointer(&t.entries[int(key)&t.mask]))
}
