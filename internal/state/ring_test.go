package state

import (
	"reflect"
	"testing"
)

func TestRing_PushBelowCap(t *testing.T) {
	r := NewRing[int](3)
	r.Push(1)
	r.Push(2)
	if got, want := r.Len(), 2; got != want {
		t.Fatalf("Len=%d want %d", got, want)
	}
	if !reflect.DeepEqual(r.Slice(), []int{1, 2}) {
		t.Fatalf("Slice=%v", r.Slice())
	}
}

func TestRing_OverwriteOldest(t *testing.T) {
	r := NewRing[int](3)
	for _, v := range []int{1, 2, 3, 4, 5} {
		r.Push(v)
	}
	if got, want := r.Len(), 3; got != want {
		t.Fatalf("Len=%d want %d", got, want)
	}
	if !reflect.DeepEqual(r.Slice(), []int{3, 4, 5}) {
		t.Fatalf("Slice=%v", r.Slice())
	}
}

func TestRing_AtAndForEach(t *testing.T) {
	r := NewRing[string](2)
	r.Push("a")
	r.Push("b")
	r.Push("c") // overwrites "a"
	if got := r.At(0); got != "b" {
		t.Errorf("At(0)=%q", got)
	}
	if got := r.At(1); got != "c" {
		t.Errorf("At(1)=%q", got)
	}
	if got := r.At(2); got != "" { // out of range -> zero
		t.Errorf("At(2)=%q", got)
	}
	var seen []string
	r.ForEach(func(v string) bool { seen = append(seen, v); return true })
	if !reflect.DeepEqual(seen, []string{"b", "c"}) {
		t.Errorf("ForEach=%v", seen)
	}
}

func TestRing_ForEachStops(t *testing.T) {
	r := NewRing[int](5)
	for i := 1; i <= 5; i++ {
		r.Push(i)
	}
	var seen []int
	r.ForEach(func(v int) bool { seen = append(seen, v); return v < 3 })
	if !reflect.DeepEqual(seen, []int{1, 2, 3}) {
		t.Errorf("seen=%v", seen)
	}
}

func TestRing_Reset(t *testing.T) {
	r := NewRing[int](3)
	r.Push(1)
	r.Push(2)
	r.Reset()
	if r.Len() != 0 {
		t.Errorf("Len after Reset=%d", r.Len())
	}
	if len(r.Slice()) != 0 {
		t.Errorf("Slice after Reset has len %d", len(r.Slice()))
	}
	r.Push(7) // still usable
	if r.At(0) != 7 {
		t.Errorf("At(0) after reuse=%d", r.At(0))
	}
}

func TestNewRing_PanicsOnZero(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on zero capacity")
		}
	}()
	_ = NewRing[int](0)
}
