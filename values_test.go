package tau

import "testing"

func TestPtrDerefIf(t *testing.T) {
	p := Ptr(3)
	if *p != 3 || Deref(p, 9) != 3 || Deref[int](nil, 9) != 9 {
		t.Fatal("Ptr/Deref wrong")
	}
	if If(true, "a", "b") != "a" || If(false, "a", "b") != "b" {
		t.Fatal("If wrong")
	}
}
