package unsafe

import (
	/*
		#include <stdlib.h>
	*/
	"C"
	"errors"
	"unsafe"

	"github.com/bytedance/sonic"
)

type Anyint interface {
	~int | ~uint | ~uintptr |
		~int8 | ~int16 | ~int32 | ~int64 |
		~uint8 | ~uint16 | ~uint32 | ~uint64
}

func String(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
func String2Uptr[I Anyint](b unsafe.Pointer, length I) string {
	return unsafe.String((*byte)(b), length)
}
func String2AnyPtr[P any, I Anyint](b *P, length I) string {
	return unsafe.String((*byte)(unsafe.Pointer(b)), length)
}
func Cfree[P any](ptr *P) {
	C.free(unsafe.Pointer(ptr))
}

func Bytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

type SizedSpan[T any, I Anyint] struct {
	Items  *T
	Length I
}

type Span[T any] = SizedSpan[T, int]

func (s SizedSpan[T, I]) MarshalJSON() ([]byte, error) {
	return SonicConfig.Marshal(s.Slice())
}

var SonicConfig = sonic.ConfigStd

func (s *SizedSpan[T, I]) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("unsafe.SizedSpan.UnmarshalJSON on nil pointer")
	}
	values := s.Slice()
	if err := SonicConfig.UnmarshalFromString(String(data), &values); err != nil {
		return err
	}
	if int(I(len(values))) != len(values) {
		return errors.New("unsafe.SizedSpan.UnmarshalJSON array length overflows index type")
	}
	s.FromSlice(values)
	return nil
}

func (s *SizedSpan[T, I]) ShiftRef() *T {
	var val = s.RefAt(0)
	s.Length--
	if s.Length == 0 {
		s.Items = nil
	} else {
		s.Items = s.RefAt(1)
	}
	return val
}
func (s *SizedSpan[T, I]) Shift() T {
	var val = s.At(0)
	s.Length--
	if s.Length == 0 {
		s.Items = nil
	} else {
		s.Items = s.RefAt(1)
	}
	return val
}
func (s *SizedSpan[T, I]) FromSlice(datas []T) SizedSpan[T, I] {
	var ss = SizedSpan[T, I]{unsafe.SliceData(datas), I(len(datas))}
	*s = ss
	return ss
}
func NewSpanDefault[T any](length int) Span[T] {
	return Span[T]{
		Length: length,
		Items:  unsafe.SliceData(make([]T, length)),
	}
}
func NewSpan[T any, I Anyint](length I) SizedSpan[T, I] {
	return SizedSpan[T, I]{
		Length: length,
		Items:  unsafe.SliceData(make([]T, length)),
	}
}
func SpanFromSlice[I Anyint, T any, Slice ~[]T](datas Slice) SizedSpan[T, I] {
	var ss = SizedSpan[T, I]{unsafe.SliceData(datas), I(len(datas))}
	return ss
}
func SpanContains[T comparable, I Anyint](haystack SizedSpan[T, I], needle T) bool {
	for i := range uint64(haystack.Length) {
		if haystack.At(I(i)) == needle {
			return true
		}
	}
	return false
}
func (s SizedSpan[T, I]) Slice() []T {
	return unsafe.Slice(s.Items, s.Length)
}
func (s SizedSpan[T, I]) At(i I) T {
	return *(*T)(unsafe.Add(unsafe.Pointer(s.Items), unsafe.Sizeof(*s.Items)*uintptr(i)))
}
func (s SizedSpan[T, I]) RefAt(i I) *T {
	return (*T)(unsafe.Add(unsafe.Pointer(s.Items), unsafe.Sizeof(*s.Items)*uintptr(i)))
}
