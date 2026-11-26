package tau

import (
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type convID [3]byte

func (id convID) MarshalText() ([]byte, error) { return []byte(hex.EncodeToString(id[:])), nil }

func (id *convID) UnmarshalText(text []byte) error {
	raw, err := hex.DecodeString(string(text))
	if err != nil || len(raw) != len(id) {
		return errors.New("not a convID")
	}
	copy(id[:], raw)
	return nil
}

type convTier string

type convBroken struct{}

var errConvBroken = errors.New("no text form today")

func (convBroken) MarshalText() ([]byte, error) { return nil, errConvBroken }

type convMoney struct {
	Cents    int64
	Currency string
}

type convMoneyView struct {
	Cents    int64  `json:"cents"`
	Currency string `json:"currency"`
}

type convAddressRow struct {
	Line1, City, Postcode string
	GeoHash               string
}

type convAddressView struct {
	Line1    string `json:"line1"`
	City     string `json:"city"`
	Postcode string `json:"postcode"`
}

type convCustomerRow struct {
	ID           convID
	Email        string
	FirstName    string
	PasswordHash string
	Phone        *string
	Tier         convTier
}

type convItemRow struct {
	SKU       string
	Quantity  int32
	UnitPrice convMoney
	Tags      []string
}

type convItemView struct {
	SKU       string        `json:"sku"`
	Quantity  int           `json:"quantity"`
	UnitPrice convMoneyView `json:"unit_price"`
	Tags      []string      `json:"tags"`
	LineTotal int64         `json:"line_total" tau:"-"`
}

type convOrderRow struct {
	ID        convID
	Customer  *convCustomerRow
	Items     []convItemRow
	Shipping  convAddressRow
	Billing   *convAddressRow
	Notes     map[string]convMoney
	PlacedAt  time.Time
	ShippedAt *time.Time
	Version   int32
}

type convOrderView struct {
	ID            string                   `json:"id"`
	CustomerID    string                   `json:"customer_id" tau:"Customer.ID"`
	CustomerEmail string                   `json:"customer_email" tau:"Customer.Email"`
	CustomerPhone *string                  `json:"customer_phone" tau:"Customer.Phone"`
	Tier          string                   `json:"tier" tau:"Customer.Tier"`
	Items         []convItemView           `json:"items"`
	Shipping      convAddressView          `json:"shipping"`
	Billing       *convAddressView         `json:"billing"`
	Notes         map[string]convMoneyView `json:"notes"`
	PlacedAt      time.Time                `json:"placed_at"`
	ShippedAt     *string                  `json:"shipped_at"`
	Version       int64                    `json:"version"`
	Total         int64                    `json:"total" tau:"-"`
}

func convSampleOrder() convOrderRow {
	phone := "+44 20 7946 0000"
	shipped := time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC)
	return convOrderRow{
		ID:       convID{1, 2, 3},
		Customer: &convCustomerRow{ID: convID{9, 9, 9}, Email: "ada@example.com", FirstName: "Ada", PasswordHash: "secret", Phone: &phone, Tier: "gold"},
		Items: []convItemRow{
			{SKU: "A1", Quantity: 2, UnitPrice: convMoney{1500, "GBP"}, Tags: []string{"brass"}},
			{SKU: "B2", Quantity: 10, UnitPrice: convMoney{25, "GBP"}},
		},
		Shipping:  convAddressRow{Line1: "12 St James's Sq", City: "London", Postcode: "SW1Y 4JH", GeoHash: "gcpvj"},
		Billing:   &convAddressRow{Line1: "1 Dorset St", City: "London", Postcode: "W1U 4EG"},
		Notes:     map[string]convMoney{"gift": {500, "GBP"}},
		PlacedAt:  time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC),
		ShippedAt: &shipped,
		Version:   4,
	}
}

func TestConvertRowToResponse(t *testing.T) {
	order := convSampleOrder()
	got := Convert[convOrderView](order)
	shipped := "2026-11-20T09:30:00Z"
	want := convOrderView{
		ID: "010203", CustomerID: "090909", CustomerEmail: "ada@example.com", CustomerPhone: order.Customer.Phone, Tier: "gold",
		Items: []convItemView{
			{SKU: "A1", Quantity: 2, UnitPrice: convMoneyView{1500, "GBP"}, Tags: []string{"brass"}},
			{SKU: "B2", Quantity: 10, UnitPrice: convMoneyView{25, "GBP"}},
		},
		Shipping:  convAddressView{"12 St James's Sq", "London", "SW1Y 4JH"},
		Billing:   &convAddressView{"1 Dorset St", "London", "W1U 4EG"},
		Notes:     map[string]convMoneyView{"gift": {500, "GBP"}},
		PlacedAt:  order.PlacedAt,
		ShippedAt: &shipped,
		Version:   4,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%+v\nwant\n%+v", got, want)
	}
	if got.Billing == nil || got.Items[0].LineTotal != 0 || got.Total != 0 {
		t.Error("fields tagged tau:\"-\" must be left for the caller")
	}
}

func TestConvertKeepsNilsAndZeroes(t *testing.T) {
	got := Convert[convOrderView](convOrderRow{})
	if got.Items != nil || got.Notes != nil || got.Billing != nil || got.ShippedAt != nil || got.CustomerPhone != nil {
		t.Errorf("nil slices, maps and pointers must stay nil: %+v", got)
	}
	// Customer is a nil pointer: fields reached through it are left zero.
	if got.CustomerID != "" || got.CustomerEmail != "" || got.Tier != "" {
		t.Errorf("fields behind a nil pointer must be zero: %+v", got)
	}
	empty := Convert[convOrderView](convOrderRow{Items: []convItemRow{}, Notes: map[string]convMoney{}})
	if empty.Items == nil || empty.Notes == nil {
		t.Error("an empty slice or map must stay non-nil")
	}
}

func TestConvertTopLevelShapes(t *testing.T) {
	rows := []convAddressRow{{Line1: "a", City: "London"}, {Line1: "b", City: "Turin"}}
	if list := Convert[[]convAddressView](rows); len(list) != 2 || list[1].City != "Turin" {
		t.Errorf("slice: %+v", list)
	}
	if list := Convert[[]convAddressView]([2]convAddressRow{rows[0], rows[1]}); len(list) != 2 {
		t.Errorf("array to slice: %+v", list)
	}
	if p := Convert[*convAddressView](&rows[0]); p == nil || p.Line1 != "a" {
		t.Errorf("pointer to pointer: %+v", p)
	}
	if p := Convert[*convAddressView](rows[0]); p == nil || p.Line1 != "a" {
		t.Errorf("value to pointer: %+v", p)
	}
	if v := Convert[convAddressView](&rows[1]); v.City != "Turin" {
		t.Errorf("pointer to value: %+v", v)
	}
	if p := Convert[*convAddressView]((*convAddressRow)(nil)); p != nil {
		t.Errorf("a nil pointer must stay nil: %+v", p)
	}
	if v := Convert[convAddressView]((*convAddressRow)(nil)); v != (convAddressView{}) {
		t.Errorf("a nil pointer into a value must give the zero value: %+v", v)
	}
	if m := Convert[map[convTier]convAddressView](map[string]convAddressRow{"home": rows[0]}); m["home"].Line1 != "a" {
		t.Errorf("map with converted keys: %+v", m)
	}
	if same := Convert[convAddressRow](rows[0]); same != rows[0] {
		t.Errorf("same type: %+v", same)
	}
	if s := Convert[string](convID{1, 2, 3}); s != "010203" {
		t.Errorf("text form: %q", s)
	}
	if n := Convert[int64](int32(7)); n != 7 {
		t.Errorf("widening: %d", n)
	}
	if s := Convert[any](rows[0]); s != any(rows[0]) {
		t.Errorf("to an interface: %v", s)
	}
}

func TestConvertMatchesByNameThenJSONName(t *testing.T) {
	type row struct {
		Name  string `json:"full_name"`
		Email string `json:"mail"`
		Mail  string `json:"other"`
	}
	type view struct {
		FullName string `json:"full_name"` // no Go name in common: matched by json name
		Mail     string `json:"mail"`      // Go name wins over json name
	}
	got := Convert[view](row{Name: "Ada", Email: "by-json", Mail: "by-name"})
	if got.FullName != "Ada" || got.Mail != "by-name" {
		t.Errorf("%+v", got)
	}
	type hidden struct {
		Name string `json:"-"`
	}
	type wantsDash struct {
		Other string `json:"-"`
	}
	err := Try(func() { Convert[wantsDash](hidden{}) })
	if _, ok := errors.AsType[*ConvertError](err); !ok {
		t.Errorf(`json:"-" must not count as a shared name: %v`, err)
	}
}

func TestConvertEmbeddedAndRecursiveTypes(t *testing.T) {
	type stamps struct{ CreatedAt time.Time }
	type row struct {
		stamps
		Name string
	}
	type Stamps struct{ CreatedAt time.Time }
	type view struct {
		Stamps
		Name string
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := Convert[view](row{stamps{at}, "Ada"}); !got.CreatedAt.Equal(at) || got.Name != "Ada" {
		t.Errorf("embedded: %+v", got)
	}

	type node struct {
		Name     string
		Children []*node
	}
	type nodeView struct {
		Name     string      `json:"name"`
		Children []*nodeView `json:"children"`
	}
	tree := Convert[nodeView](node{"root", []*node{{"a", nil}, {"b", []*node{{"c", nil}}}}})
	if tree.Children[1].Children[0].Name != "c" || tree.Children[0].Children != nil {
		t.Errorf("recursive: %+v", tree)
	}

	type embedsPointer struct {
		*Stamps
		Name string
	}
	err := Try(func() { Convert[embedsPointer](row{}) })
	if ce, ok := errors.AsType[*ConvertError](err); !ok || !strings.Contains(ce.Error(), "embedded pointers") {
		t.Errorf("embedded pointer in the destination: %v", err)
	}
}

func TestConvertScalars(t *testing.T) {
	type from struct {
		Tier  convTier
		Small int8
		U     uint16
		F     float32
		N     int32
		Flag  bool
	}
	type to struct {
		Tier  string
		Small int
		U     int32
		F     float64
		N     float64
		Flag  bool
	}
	got := Convert[to](from{"gold", -3, 65535, 0.5, 7, true})
	if got != (to{"gold", -3, 65535, 0.5, 7, true}) {
		t.Errorf("%+v", got)
	}
	back := Convert[struct{ Tier convTier }](struct{ Tier string }{"silver"})
	if back.Tier != "silver" {
		t.Errorf("string to a named string: %+v", back)
	}

	refused := map[string]func(){
		"narrowing":             func() { Convert[struct{ N int16 }](struct{ N int32 }{}) },
		"uint to same-size int": func() { Convert[struct{ N int32 }](struct{ N uint32 }{}) },
		"signed to unsigned":    func() { Convert[struct{ N uint64 }](struct{ N int8 }{}) },
		"int64 to float64":      func() { Convert[struct{ N float64 }](struct{ N int64 }{}) },
		"float to int":          func() { Convert[struct{ N int }](struct{ N float64 }{}) },
		"float64 to float32":    func() { Convert[struct{ N float32 }](struct{ N float64 }{}) },
		"int to string":         func() { Convert[struct{ N string }](struct{ N int }{}) },
		"string to int":         func() { Convert[struct{ N int }](struct{ N string }{}) },
		"bool to int":           func() { Convert[struct{ N int }](struct{ N bool }{}) },
		"struct to string":      func() { Convert[struct{ N string }](struct{ N convMoney }{}) },
		"slice to struct":       func() { Convert[convMoneyView]([]convMoney{}) },
		"func to func":          func() { Convert[struct{ N func(int) }](struct{ N func() }{}) },
	}
	for name, fn := range refused {
		if _, ok := errors.AsType[*ConvertError](Try(fn)); !ok {
			t.Errorf("%s was not refused", name)
		}
	}
}

func TestConvertErrorsNameTheField(t *testing.T) {
	type renamed struct {
		Line1 string
		Zip   string // the source calls it Postcode
	}
	cases := []struct {
		name  string
		fn    func()
		field string
		says  string
	}{
		{"no source for a field", func() { Convert[renamed](convAddressRow{}) }, "Zip", "has no field for it"},
		{"nested, inside a slice", func() {
			Convert[struct{ Items []struct{ Price int } }](convOrderRow{})
		}, "Items[].Price", "has no field for it"},
		{"wrong type", func() { Convert[struct{ Postcode int }](convAddressRow{}) }, "Postcode", "no conversion from string to int"},
		{"tag naming a missing field", func() {
			Convert[struct {
				Email string `tau:"Customer.Mail"`
			}](convOrderRow{})
		}, "Email", "has no exported field Mail"},
		{"tag through a non-struct", func() {
			Convert[struct {
				X string `tau:"Version.X"`
			}](convOrderRow{})
		}, "X", "is not a struct"},
		{"map value", func() {
			Convert[map[string]struct{ Pence int }](map[string]convMoney{})
		}, "[].Pence", "has no field for it"},
	}
	for _, c := range cases {
		err := Try(c.fn)
		ce, ok := errors.AsType[*ConvertError](err)
		if !ok {
			t.Errorf("%s: expected a *ConvertError, got %v", c.name, err)
			continue
		}
		if ce.Field != c.field || !strings.Contains(ce.Error(), c.says) || !strings.Contains(ce.Error(), "field "+c.field) {
			t.Errorf("%s: field %q, message %q", c.name, ce.Field, ce.Error())
		}
		if StackOf(err) == "" {
			t.Errorf("%s: no stack", c.name)
		}
	}

	whole, ok := errors.AsType[*ConvertError](Try(func() { Convert[convMoneyView](42) }))
	if !ok || whole.Field != "" || whole.From != reflect.TypeFor[int]() || whole.To != reflect.TypeFor[convMoneyView]() {
		t.Errorf("fault with the whole value: %+v", whole)
	}
	fromNil, ok := errors.AsType[*ConvertError](Try(func() { Convert[convMoneyView](nil) }))
	if !ok || fromNil.From != nil || !strings.Contains(fromNil.Error(), "cannot convert nil") {
		t.Errorf("nil source: %v", fromNil)
	}
}

// A wrong mapping throws for every value, and every time: the failure is
// cached with the plan.
func TestConvertRefusesAWrongMappingEveryTime(t *testing.T) {
	type renamed struct{ Zip string }
	for range 3 {
		err := Try(func() { Convert[renamed](convAddressRow{Postcode: "x"}) })
		if ce, ok := errors.AsType[*ConvertError](err); !ok || ce.Field != "Zip" {
			t.Fatalf("got %v", err)
		}
	}
}

func TestConvertTextForms(t *testing.T) {
	type request struct {
		ID string
		At string
	}
	type command struct {
		ID convID
		At time.Time
	}
	got := Convert[command](request{"0a0b0c", "2026-11-18T14:00:00Z"})
	if got.ID != (convID{10, 11, 12}) || !got.At.Equal(time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("%+v", got)
	}

	err := Try(func() { Convert[command](request{"0a0b0c", "yesterday"}) })
	ce, ok := errors.AsType[*ConvertError](err)
	if !ok || ce.Field != "At" || !strings.Contains(ce.Error(), "yesterday") {
		t.Errorf("a value that cannot be read must name its field: %v", err)
	}
	if _, isParse := errors.AsType[*time.ParseError](err); !isParse {
		t.Errorf("the underlying error must stay reachable: %v", err)
	}
	// The same pair of types still converts good values afterwards.
	if again := Convert[command](request{"010203", "2026-11-18T14:00:00Z"}); again.ID != (convID{1, 2, 3}) {
		t.Errorf("%+v", again)
	}

	err = Try(func() { Convert[struct{ B string }](struct{ B convBroken }{}) })
	if ce, ok := errors.AsType[*ConvertError](err); !ok || ce.Field != "B" || !errors.Is(err, errConvBroken) {
		t.Errorf("a failing MarshalText must name its field: %v", err)
	}
}

func TestConvertSharesSameTypedSlices(t *testing.T) {
	row := convItemRow{Tags: []string{"a"}}
	view := Convert[convItemView](row)
	view.Tags[0] = "changed"
	if row.Tags[0] != "changed" {
		t.Error("documented behaviour changed: a slice of the same type is now copied; update the doc comment")
	}
}

func TestConvertLeavesOrdinaryPanicsAlone(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a panic inside MarshalText was swallowed")
		}
	}()
	Convert[struct{ P string }](struct{ P convPanics }{})
}

type convPanics struct{}

func (convPanics) MarshalText() ([]byte, error) { panic("bug in MarshalText") }

func TestConvertConcurrentFirstUse(t *testing.T) {
	type view struct {
		ID    string
		Items []convItemView
	}
	order := convSampleOrder()
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if got := Convert[view](order); got.ID != "010203" || len(got.Items) != 2 {
				t.Errorf("%+v", got)
			}
		})
	}
	wg.Wait()
}

func BenchmarkConvertOrder(b *testing.B) {
	order := convSampleOrder()
	for b.Loop() {
		_ = Convert[convOrderView](order)
	}
}

func BenchmarkConvertFlatStruct(b *testing.B) {
	row := convSampleOrder().Shipping
	for b.Loop() {
		_ = Convert[convAddressView](row)
	}
}

// Finding: two distinct source keys that convert to the same key must not
// silently overwrite one another.
func TestConvertRefusesMapKeysThatCollapse(t *testing.T) {
	type fullKey struct {
		ID     int
		Region string
	}
	type shortKey struct{ ID int }
	src := map[fullKey]string{{1, "eu"}: "a", {1, "us"}: "b"}
	err := Try(func() { Convert[map[shortKey]string](src) })
	ce, ok := errors.AsType[*ConvertError](err)
	if !ok || !strings.Contains(ce.Error(), "same key") {
		t.Fatalf("collapsing keys were accepted: %v", err)
	}
	// Distinct after conversion: fine.
	got := Convert[map[shortKey]string](map[fullKey]string{{1, "eu"}: "a", {2, "eu"}: "b"})
	if len(got) != 2 || got[shortKey{2}] != "b" {
		t.Errorf("%v", got)
	}
	nested := struct{ M map[fullKey]int }{map[fullKey]int{{1, "eu"}: 1, {1, "us"}: 2}}
	err = Try(func() { Convert[struct{ M map[shortKey]int }](nested) })
	if ce, ok := errors.AsType[*ConvertError](err); !ok || ce.Field != "M" {
		t.Errorf("nested collapse must name the field: %v", err)
	}
}

// Finding: an embedded struct in the destination is a field like any other
// when it carries a tag, and must never be left silently empty.
func TestConvertEmbeddedDestinationStructs(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	type row struct {
		CreatedAt time.Time
		Name      string
	}
	type tagged struct {
		time.Time `tau:"CreatedAt"`
		Name      string
	}
	if got := Convert[tagged](row{at, "Ada"}); !got.Time.Equal(at) || got.Name != "Ada" {
		t.Errorf("tagged embedded struct: %+v", got)
	}

	type untagged struct {
		time.Time
		Name string
	}
	err := Try(func() { Convert[untagged](row{at, "Ada"}) })
	if ce, ok := errors.AsType[*ConvertError](err); !ok || ce.Field != "Time" {
		t.Errorf("an embedded struct nothing can fill must be refused: %v", err)
	}
	if got := Convert[untagged](struct {
		Time time.Time
		Name string
	}{at, "Ada"}); !got.Time.Equal(at) {
		t.Errorf("embedded struct matched by its name: %+v", got)
	}

	type skipped struct {
		time.Time `tau:"-"`
		Name      string
	}
	if got := Convert[skipped](row{at, "Ada"}); !got.Time.IsZero() || got.Name != "Ada" {
		t.Errorf("tau:\"-\" on an embedded struct: %+v", got)
	}

	// A tagged embedded struct is mapped whole; its promoted fields are not
	// then looked up one by one.
	type Stamps struct{ CreatedAt, UpdatedAt time.Time }
	type audit struct{ CreatedAt, UpdatedAt time.Time }
	type auditedRow struct {
		Audit audit
		Name  string
	}
	type view struct {
		Stamps `tau:"Audit"`
		Name   string
	}
	if got := Convert[view](auditedRow{audit{at, at}, "Ada"}); !got.UpdatedAt.Equal(at) {
		t.Errorf("tagged embedded struct converted from another struct: %+v", got)
	}
}

type convPtrText struct{ v string }

func (p *convPtrText) MarshalText() ([]byte, error) { return []byte("text:" + p.v), nil }

// Finding: MarshalText declared on the pointer must be found whether the
// source holds the pointer or the value.
func TestConvertPointerReceiverMarshalText(t *testing.T) {
	type viaPointer struct{ ID *convPtrText }
	type viaValue struct{ ID convPtrText }
	type view struct{ ID string }
	type optionalView struct{ ID *string }
	if got := Convert[view](viaPointer{&convPtrText{"a"}}); got.ID != "text:a" {
		t.Errorf("pointer source: %+v", got)
	}
	if got := Convert[view](viaValue{convPtrText{"b"}}); got.ID != "text:b" {
		t.Errorf("value source: %+v", got)
	}
	if got := Convert[view](viaPointer{}); got.ID != "" {
		t.Errorf("nil pointer must give the zero value: %+v", got)
	}
	if got := Convert[optionalView](viaPointer{&convPtrText{"c"}}); got.ID == nil || *got.ID != "text:c" {
		t.Errorf("pointer to pointer: %+v", got)
	}
	if got := Convert[optionalView](viaPointer{}); got.ID != nil {
		t.Errorf("nil pointer must stay nil: %+v", got)
	}
	if got := Convert[string](&convPtrText{"d"}); got != "text:d" {
		t.Errorf("top level: %q", got)
	}
}

// Finding: a tag path through a pointer to a pointer must work, not panic.
func TestConvertTagPathThroughPointers(t *testing.T) {
	type customer struct{ Email string }
	type row struct{ Customer **customer }
	type view struct {
		Email string `tau:"Customer.Email"`
	}
	c := &customer{"ada@example.com"}
	if got := Convert[view](row{&c}); got.Email != "ada@example.com" {
		t.Errorf("%+v", got)
	}
	var none *customer
	if got := Convert[view](row{&none}); got.Email != "" {
		t.Errorf("nil inner pointer: %+v", got)
	}
	if got := Convert[view](row{}); got.Email != "" {
		t.Errorf("nil outer pointer: %+v", got)
	}

	type inner struct{ Email string }
	type embeds struct{ *inner }
	if got := Convert[struct{ Email string }](embeds{}); got.Email != "" {
		t.Errorf("promoted field behind a nil embedded pointer: %+v", got)
	}
	if got := Convert[struct{ Email string }](embeds{&inner{"x"}}); got.Email != "x" {
		t.Errorf("promoted field behind an embedded pointer: %+v", got)
	}
}

// The shallower source field wins a json name, as in Dig.
func TestConvertShallowestSourceFieldWinsAJSONName(t *testing.T) {
	type deep struct {
		Inner string `json:"name"`
	}
	type row struct {
		deep
		Outer string `json:"name"`
	}
	type view struct {
		FullName string `json:"name"`
	}
	if got := Convert[view](row{deep{"nested"}, "direct"}); got.FullName != "direct" {
		t.Errorf("%+v", got)
	}
}
