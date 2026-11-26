# Converting structs

Go converts one struct type to another only when the two have identical
fields. For everything else, a database row becoming an API response or a
request becoming a command, you write a mapper function that assigns each
field. `Convert` does that mapping by matching fields:

```go
resp := tau.Convert[OrderResponse](order)
list := tau.Convert[[]OrderResponse](orders)
```

## How fields are matched

A field in the destination takes the source field with the same Go name.
If there is none, it takes the source field with the same `json` name.

```go
type CustomerRow struct {
    ID           bson.ObjectID
    Email        string
    PasswordHash string
}

type CustomerResponse struct {
    ID    string `json:"id"`
    Email string `json:"email"`
}

resp := tau.Convert[CustomerResponse](row) // PasswordHash is left behind
```

Source fields with no destination are ignored. Leaving things out is what
a response type is for.

**Every exported destination field must have a source.** If one does not,
`Convert` throws, naming the field:

```
tau: cannot convert main.CustomerRow to main.CustomerResponse: field Mail:
main.CustomerRow has no field for it; add one, or tag it `tau:"-"` and set it yourself
```

This is the point of the function. A mapper written with a library that
skips unmatched fields keeps working after a rename, and returns an empty
value in production. `Convert` stops instead.

## Tags

Two tags on the destination cover what names alone cannot:

```go
type OrderResponse struct {
    ID            string `json:"id"`
    CustomerEmail string `json:"customer_email" tau:"Customer.Email"`
    Total         Money  `json:"total" tau:"-"`
}
```

- `tau:"Customer.Email"` takes the value from a path of Go field names in
  the source. Use it to flatten a nested value or to rename. A nil pointer
  on the path leaves the field at its zero value.
- `tau:"-"` marks a field `Convert` must leave alone, because you compute
  it yourself:

```go
resp := tau.Convert[OrderResponse](order)
resp.Total = sum(order.Items)
```

## What converts

| From | To | Result |
|---|---|---|
| A type | The same type | Assigned as it is. A slice or map is shared with the source, not copied. |
| Struct | Struct | Field by field, by the rules above |
| Slice or array | Slice | Element by element; nil stays nil |
| Map | Map | Keys and values converted; nil stays nil |
| Pointer | Pointer | A new value; nil stays nil |
| Value | Pointer | A pointer to a copy |
| Pointer | Value | The value, or the zero value if the pointer is nil |
| Named type | Its basic kind, and back | `type Tier string` to `string` |
| Number | A wider number | `int32` to `int`, `float32` to `float64`; never narrower |
| A type with a text form | `string` | `time.Time`, most ID types, enums with `MarshalText` |
| `string` | A type that reads text | `"2026-11-18T14:00:00Z"` into a `time.Time` |

Embedded structs are filled through their promoted fields. Types that
contain themselves, such as a tree node, work.

Anything else is refused: `int` to `string`, a float to an integer, a
narrowing number, a struct to a string without a text form. `Convert` does
not guess.

## Errors

`Convert` throws a `*tau.ConvertError`, which carries the two types, the
path of the destination field at fault (`Items[].Price`), and the cause.

Whether two types can be converted is decided from the types alone, the
first time the pair is seen, and remembered. A wrong mapping therefore
throws on every call, for every value, including an empty one. **One test
that runs a conversion proves its whole field mapping.** It is still a
run-time check: the compiler does not catch it, so make sure each
conversion is reached by a test.

The one failure that depends on a value is text: a string that will not
parse into a `time.Time`, or a `MarshalText` that returns an error. Those
throw a `*ConvertError` naming the field, with the underlying error
reachable through `errors.As`.

## Cost

`Convert` works by reflection.

| | Hand-written mapper | `Convert` |
|---|---|---|
| A response of about twenty fields, nested | 0.2 µs | 1.1 µs |
| One flat struct of three strings | under 10 ns | 0.1 µs |

For comparison, encoding that twenty-field response as JSON takes about
2.2 µs. For building a response, the cost disappears next to the encoding
and the network. For a loop over thousands of rows, write the mapper by
hand.

## When not to use it

- **Hot loops**, as above.
- **Types that differ more than they match.** If half the destination
  fields need a tag or a computed value, a hand-written function is
  clearer.
- **When you want the response type to know nothing about the row type.**
  A `tau:"Customer.Email"` tag names a field of the source.
