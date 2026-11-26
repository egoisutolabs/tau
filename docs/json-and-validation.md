# JSON and validation

## ParseJSON

```go
type Signup struct {
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=18,lte=120"`
	Plan  string `json:"plan" validate:"oneof=free pro"`
}

s := tau.ParseJSON[Signup](body)
```

`ParseJSON` throws a `*tau.InputError` unless all of these hold:

1. **The JSON decodes strictly.** Unknown fields, duplicate keys, invalid
   UTF-8 and trailing data are errors. A misspelt field name is reported,
   not silently ignored.
2. **The `validate` tags pass.** The rules are
   [go-playground/validator](https://pkg.go.dev/github.com/go-playground/validator/v10)'s:
   `required`, `email`, `gte`, `oneof`, `min`, `max`, `dive` for elements,
   and many more.
3. **The type's `Validate() error` method passes**, if it has one. Use it
   for rules that tags cannot express, such as one field depending on
   another.

`tau.Validate(v)` runs steps 2 and 3 on a value you already have. It
accepts structs, pointers to them, and slices, arrays and maps of them.

## From a reader, with a size limit

`ReadJSON` reads at most a given number of bytes and then does everything
`ParseJSON` does. It is meant for request bodies:

```go
req := tau.ReadJSON[Request](r.Body, 1<<20) // at most 1 MiB
```

If the input is longer it throws an `InputError` that matches
`tau.ErrTooLarge`, without reading the rest. The limit must be positive.
Mapping errors to HTTP statuses stays with your handler; see the next
section.

## Recognising bad input

Everything `ParseJSON` and `ReadJSON` throw because of the input is a
`*tau.InputError`, so a handler needs one check to answer 400:

```go
req, err := tau.TryValue(func() Request { return tau.ReadJSON[Request](r.Body, 1<<20) })
if bad, ok := errors.AsType[*tau.InputError](err); ok {
	http.Error(w, bad.Error(), http.StatusBadRequest)
	return
}
```

`Kind` says what went wrong, and `Path` where, as a JSON Pointer, when the
decoder knows:

| `Kind` | Meaning | `Path` |
| --- | --- | --- |
| `tau.InputMalformed` | not valid JSON: syntax, duplicate key, invalid UTF-8, trailing data | sometimes |
| `tau.InputUnknownField` | a key the type does not have | `/colour` |
| `tau.InputWrongType` | wrong JSON type for the field | `/age` |
| `tau.InputInvalid` | decoded, but failed validation | empty; see below |
| `tau.InputTooLarge` | over `ReadJSON`'s byte limit | empty |

An `InputError` wraps its cause, which `errors.Is` and `errors.As` still
reach: `tau.ErrTooLarge`, the decoder's own errors, a
`*tau.ValidationError`, or whatever a `Validate()` method returned. A
failure to read from `ReadJSON`'s reader is the reader's fault and is not
an `InputError`. Neither is an error from calling `tau.Validate` directly.

## Reading the failed fields

A tag failure carries a `*tau.ValidationError`. Every broken rule is
listed, not just the first:

```go
if invalid, ok := errors.AsType[*tau.ValidationError](err); ok {
	for _, f := range invalid.Fields {
		// f.Field  "address.city", "tags[2]"   path in JSON names
		// f.Rule   "required", "gte"
		// f.Param  "18"                        empty if the rule has none
		// f.Value  the offending value
	}
}
```

The message reads `age must satisfy gte=18 (got 12); plan must satisfy
oneof=free pro (got "gold")`. Fields are named as the sender of the JSON
sees them, by their `json` tag.

The validator only looks inside a slice or map of structs when the field
says so with `validate:"dive"`. Without it the elements are not checked.

## Fields that may be left out: PATCH requests

A partial update needs three states for each field: not sent (leave it
alone), sent as `null` (clear it), and sent with a value (replace it). A
pointer only gives two. `tau.Field[T]` gives all three:

```go
type UpdateProfile struct {
	Bio  tau.Field[string] `json:"bio,omitzero" validate:"omitnil,max=200"`
	Name tau.Field[string] `json:"name,omitzero" validate:"omitnil,min=1"`
}

req := tau.ParseJSON[UpdateProfile](body)
profile.Bio = req.Bio.OrElse(profile.Bio)
profile.Name = req.Name.OrElse(profile.Name)
```

| | |
| --- | --- |
| `f.IsOmitted()` | the key was not in the JSON |
| `f.IsNull()` | the key was there with `null` |
| `f.Get()` | the value, and whether there is one |
| `f.OrElse(current)` | the value; the zero value if null; `current` if omitted |
| `tau.FieldOf(v)`, `tau.NullField[T]()` | build one, for tests and for encoding |

Two things to write in the tags:

- **`omitzero` in the `json` tag**, so an omitted field is also left out
  when the struct is encoded. Without it an omitted field encodes as
  `null`.
- **`omitnil` or `required` first in the `validate` tag.** For validation
  a `Field` is a pointer to its value, nil when omitted or null.
  `omitnil,max=200` checks the value only when there is one;
  `required,max=200` insists on one. A bare `max=200` fails for an omitted
  field.

A value that happens to be empty, such as `""` or `[]`, is still a value:
it is neither null nor omitted, and its rules are checked. A rule that
null is not allowed, while omitting is, cannot be written as a tag; put it
in the type's `Validate()` method with `f.IsNull()`.

## Untyped JSON

For data decoded into `any`, `Dig` replaces a chain of type assertions,
the way `?.` does in JavaScript:

```go
city, ok := tau.Dig[string](data, "user", "addresses", 0, "city")
plan := tau.DigOr(data, "free", "account", "plan") // with a fallback, like ??
```

A string step looks up an object key; an int step indexes an array, from
the end if negative. `Dig` reports false for a missing key, an index out
of range, a `null`, or a value of the wrong type. JSON numbers decode as
`float64`; `Dig` converts a whole number to whichever integer type you ask
for, and refuses one that does not fit.

## Encoding

There is no tau function for encoding. Use the standard library:

```go
body := tau.Must(json.Marshal(v))
```
