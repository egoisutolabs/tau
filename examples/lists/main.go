// Chainable lists, and the ordered map and set.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/egoisutolabs/tau"
)

type Order struct {
	Customer string
	Item     string
	Cents    int
	Paid     bool
}

func main() {
	orders := tau.List[Order]{
		{"ada", "keyboard", 12000, true},
		{"bob", "cable", 900, true},
		{"ada", "mouse", 4500, false},
		{"cy", "monitor", 31000, true},
		{"bob", "stand", 5200, true},
		{"ada", "cable", 900, true},
	}

	// A chain may change the element type along the way.
	paid := orders.Filter(func(o Order) bool { return o.Paid })
	total := paid.Reduce(0, func(sum int, o Order) int { return sum + o.Cents })
	items := paid.
		Sort(func(a, b Order) bool { return a.Cents > b.Cents }).
		Take(3).
		Map(func(o Order) string { return o.Item })
	check(total == 50000, "total %d", total)
	check(strings.Join(items, ",") == "monitor,keyboard,stand", "top items %v", items)

	// GroupBy returns a map of lists, so each group chains too.
	spend := tau.NewMap[string, int]() // an ordered map keeps the report stable
	for customer, theirs := range paid.GroupBy(func(o Order) string { return o.Customer }) {
		spend.Set(customer, theirs.Reduce(0, func(sum int, o Order) int { return sum + o.Cents }))
	}
	ada, _ := spend.Get("ada")
	check(spend.Len() == 3 && ada == 12900, "spend: %d customers, ada %d", spend.Len(), ada)

	unpaid, _ := orders.Partition(func(o Order) bool { return !o.Paid })
	check(len(unpaid) == 1 && unpaid[0].Item == "mouse", "unpaid %v", unpaid)
	check(len(orders.Chunk(4)) == 2, "chunks")

	// Set keeps insertion order, unlike a map used as a set.
	sold := tau.NewSet(paid.Map(func(o Order) string { return o.Item })...)
	stocked := tau.NewSet("cable", "keyboard", "webcam")
	check(strings.Join(sold.Values(), ",") == "keyboard,cable,monitor,stand", "sold %v", sold.Values())
	check(strings.Join(stocked.Difference(sold).Values(), ",") == "webcam", "never sold")
	check(strings.Join(tau.Uniq([]int{3, 1, 3, 2, 1}).Map(func(n int) string { return fmt.Sprint(n) }), "") == "312", "Uniq")

	fmt.Printf("paid total: $%d.%02d\n", total/100, total%100)
	fmt.Println("top items:", strings.Join(items, ", "))
	fmt.Println("never sold:", strings.Join(stocked.Difference(sold).Values(), ", "))
}

func check(ok bool, format string, args ...any) {
	if !ok {
		fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
		os.Exit(1)
	}
}
