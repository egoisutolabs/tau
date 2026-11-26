package tau

import (
	"strconv"
	"strings"
	"testing"
)

func TestList(t *testing.T) {
	nums := List[int]{1, 2, 3, 4, 5, 6}
	total := nums.
		Filter(func(n int) bool { return n > 2 }).
		Map(func(n int) int { return n * 10 }).
		Reduce(0, func(a, n int) int { return a + n })
	if total != 180 {
		t.Fatalf("got %d", total)
	}
	if !nums.Some(func(n int) bool { return n == 6 }) || nums.Every(func(n int) bool { return n < 6 }) {
		t.Fatal("Some/Every wrong")
	}
	if v, ok := nums.Find(func(n int) bool { return n > 4 }); !ok || v != 5 {
		t.Fatalf("got %d", v)
	}
}

func TestListChangesType(t *testing.T) {
	words := List[int]{1, 2, 3, 4}.
		Filter(func(n int) bool { return n%2 == 0 }).
		Map(strconv.Itoa).
		FlatMap(func(s string) []string { return []string{s, s} })
	if strings.Join(words, "") != "2244" {
		t.Fatalf("got %v", words)
	}
	n := words.Reduce(0, func(acc int, s string) int { return acc + len(s) })
	if n != 4 {
		t.Fatalf("got %d", n)
	}
}

func TestListSortAndReverse(t *testing.T) {
	got := List[int]{5, 3, 1, 4, 2}.
		Sort(func(a, b int) bool { return a < b }).
		Reverse()
	if len(got) != 5 || got[0] != 5 || got[4] != 1 {
		t.Fatalf("got %v", got)
	}
	seen := 0
	if got.ForEach(func(int) { seen++ }).Filter(func(n int) bool { return n > 3 }); seen != 5 {
		t.Fatalf("ForEach visited %d", seen)
	}
}

type order struct {
	customer string
	cents    int
}

func TestListLoBackedMethods(t *testing.T) {
	orders := List[order]{{"ada", 500}, {"bob", 120}, {"ada", 80}, {"cy", 990}, {"bob", 40}}
	who := func(o order) string { return o.customer }
	big := func(o order) bool { return o.cents >= 100 }

	groups := orders.GroupBy(who)
	if len(groups) != 3 || len(groups["ada"]) != 2 || groups["ada"].Map(func(o order) int { return o.cents })[1] != 80 {
		t.Fatalf("GroupBy got %v", groups)
	}
	if u := orders.UniqBy(who); len(u) != 3 || u[1].cents != 120 {
		t.Fatalf("UniqBy got %v", u)
	}
	if k := orders.KeyBy(who); len(k) != 3 || k["bob"].cents != 40 {
		t.Fatalf("KeyBy got %v", k)
	}
	yes, no := orders.Partition(big)
	if len(yes) != 3 || len(no) != 2 || len(orders.Reject(big)) != 2 || orders.Count(big) != 3 {
		t.Fatalf("Partition/Reject/Count got %v / %v", yes, no)
	}
	chunks := orders.Chunk(2)
	if len(chunks) != 3 || len(chunks[2]) != 1 || len(chunks[0].Filter(big)) != 2 {
		t.Fatalf("Chunk got %v", chunks)
	}
	if err := Try(func() { orders.Chunk(0) }); err == nil {
		t.Fatal("Chunk(0) should throw, not panic")
	}
	if len(orders.Take(2)) != 2 || len(orders.Take(99)) != 5 || len(orders.Take(-1)) != 0 {
		t.Fatal("Take wrong")
	}
	if d := orders.Drop(3); len(d) != 2 || d[0].customer != "cy" || len(orders.Drop(99)) != 0 || len(orders.Drop(-1)) != 5 {
		t.Fatal("Drop wrong")
	}
	first, ok1 := orders.First()
	last, ok2 := orders.Last()
	_, ok3 := List[order]{}.First()
	_, ok4 := List[order]{}.Last()
	if !ok1 || !ok2 || ok3 || ok4 || first.cents != 500 || last.cents != 40 {
		t.Fatal("First/Last wrong")
	}
	if len(orders) != 5 || orders[0].cents != 500 {
		t.Fatal("a method changed the original list")
	}
}

func TestUniqKeysValues(t *testing.T) {
	if got := Uniq([]int{3, 1, 3, 2, 1}); len(got) != 3 || got[0] != 3 || got[2] != 2 {
		t.Fatalf("got %v", got)
	}
	m := map[string]int{"a": 1, "b": 2}
	keys := Keys(m).Sort(func(a, b string) bool { return a < b })
	sum := Values(m).Reduce(0, func(acc, v int) int { return acc + v })
	if len(keys) != 2 || keys[0] != "a" || sum != 3 {
		t.Fatalf("got %v, %d", keys, sum)
	}
}
