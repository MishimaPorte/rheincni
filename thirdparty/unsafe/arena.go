package unsafe

import (
	"io"
	"sync"
	"unsafe"
)

// This type represents an ARENA. It is used to store things
// bound by the same lifetime and freeing it at all at once. E.g. a single plugin
// execution or something.
//
// Arena can only hold pointer into the memory owned either by static go memory
// or by the Arena itself - GC assumes that only death lies here.
type Arena struct {
	// now-used chunk (for object and slice alloc)
	data      uintptr
	cap, used uintptr

	chunks []SizedSpan[byte, uintptr]
}

// This should be some other data structure, honestly, since
// we want to group chunks by size, etc to get better results.
// also we probably dont want to trash allocations as aggressively as
// sync.Pool does.
var chunkFreeList sync.Pool

type chunk struct {
	ptr unsafe.Pointer
	cap uintptr
}

const n = 4096 * 4

func (a *Arena) ReadAllFrom(r io.Reader) (Span[byte], error) {
	var b = Allocn[byte](a, 512)
	var bLen = 0
	for {
		var n, err = r.Read(b.Slice()[bLen:b.Length])
		bLen += n
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return b, err
		}

		if bLen == b.Length {
			b = Realloc(a, b, uintptr(b.Length*2))
		}
	}
}

// safety: there should be no alloc/allocn/realloc (for other allocations) calls
// between a call for allocn/realloc and realloc for a given allocation.
// That is, a reallocation may happen only if the allocation happened last for a given arena.
func Realloc[T any](a *Arena, old Span[T], n uintptr) Span[T] {
	if old.Length == 0 {
		return Allocn[T](a, n)
	}
	var oldPlace = uintptr(unsafe.Pointer(old.Items))
	// check if it is the last in the allocation
	var t T
	var elemSize = unsafe.Sizeof(t) &^ (unsafe.Alignof(t) - 1)
	if a.data+(a.used-(uintptr(old.Length)*elemSize)) != oldPlace {
		panic("bad realloc: realloc is called on an arena after something were allocated in between calls to allocn/realloc")
	}
	// we conceptually free the contents of the current chunk and then
	// reallocate them. if we get reallocated into some other chunk, we
	// perform a copy.
	a.used -= uintptr(old.Length * int(elemSize))
	var ret = Allocn[T](a, n)
	if oldPlace != uintptr(unsafe.Pointer(ret.Items)) {
		copy(ret.Slice(), old.Slice())
	}
	return ret
}

// clones the s string into the arena and returns a string that points
// into the arena-allocated string.
func (a *Arena) Strclone(s string) string {
	var bytes = Allocn[byte](a, uintptr(len(s)))
	copy(bytes.Slice(), s)
	return unsafe.String(bytes.Items, bytes.Length)
}

func Clone[T any](a *Arena, src []T) Span[T] {
	var res = Allocn[T](a, uintptr(len(src)))
	copy(res.Slice(), src)
	return res
}

// Maybe make this return a special thing, pointing to a special-cased chunk?
func Allocn[T any](a *Arena, n uintptr) Span[T] {
	var t T
	var elemSize = unsafe.Sizeof(t) &^ (unsafe.Alignof(t) - 1)
	var data = (*T)(unsafe.Pointer(a.allocSize(elemSize * n)))
	return Span[T]{data, int(n)}
}
func Alloc[T any](a *Arena) *T {
	var t T
	return (*T)(unsafe.Pointer(a.allocSize(unsafe.Sizeof(t))))
}
func (a *Arena) Free() {
	for _, ch := range a.chunks {
		chunkFreeList.Put(ch)
	}
}

func (a *Arena) allocSize(size uintptr) uintptr {
	var allocedSize = (size + 7) &^ 7
	if a.cap == 0 || a.used > a.cap || allocedSize >= a.cap-a.used {
		var ch = chunkFreeList.Get()
		if ch != nil {
			var chun = ch.(SizedSpan[byte, uintptr])
			if chun.Length >= size {
				a.data = uintptr(unsafe.Pointer(chun.Items))
				a.used = 0
				a.cap = chun.Length
				a.chunks = append(a.chunks, chun)
				goto Alloc
			}
			chunkFreeList.Put(chun)
		}
		// TODO: maintain a free-list of chunks to facilitate reuse.
		var allocedSize = max(a.cap*2, allocedSize, n)
		var ptr = unsafe.SliceData(
			make([]byte, allocedSize),
		)
		a.chunks = append(a.chunks, SizedSpan[byte, uintptr]{ptr, a.cap})
		a.data = uintptr(unsafe.Pointer(ptr))
		a.used = 0
		a.cap = allocedSize
	}
Alloc:
	var res = a.data + a.used
	a.used += allocedSize // alignment is always 8
	return res
}
