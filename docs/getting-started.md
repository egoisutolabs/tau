# Getting started

## Install

```bash
go get github.com/egoisutolabs/tau
```

Go 1.27 or newer is required: tau uses generic methods and
`encoding/json/v2`, both new in that release. There is nothing else to
install. tau brings in `samber/lo` and `go-playground/validator` itself.

## A first function

Here is a function that reads a config file, in ordinary Go:

```go
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("load config %s: port out of range", path)
	}
	return cfg, nil
}
```

And with tau:

```go
type Config struct {
	Port int `json:"port" validate:"gte=1,lte=65535"`
}

func LoadConfig(path string) (cfg Config, err error) {
	defer tau.Catch(&err)
	defer tau.Wrapf("load config %s", path)
	return tau.ParseJSON[Config](tau.Must(os.ReadFile(path))), nil
}
```

Three things are happening:

- `tau.Must` takes the `(value, error)` pair `os.ReadFile` returns. If the
  error is not nil it *throws* it; otherwise it gives back the value.
- `tau.ParseJSON` decodes strictly, checks the `validate` tags, and throws
  if either fails.
- `defer tau.Wrapf` adds the path to whatever was thrown, and
  `defer tau.Catch(&err)` turns the thrown error into the function's
  ordinary returned error.

Callers of `LoadConfig` see a normal Go function.

## The boundary rule

That last point is the one rule to keep: **throw inside, return at the
boundary.** Unexported helpers can throw freely. Exported functions return
`(T, error)` and catch with `defer tau.Catch(&err)`, so code that calls
your package never needs to know tau is involved.

A thrown error that nothing catches crashes the program, like any panic.
In an HTTP server, catch at the handler:

```go
func handle(w http.ResponseWriter, r *http.Request) {
	resp, err := tau.TryValue(func() Response { return serve(r) })
	if err != nil {
		log.Printf("request failed: %+v", err) // %+v prints the stack
		http.Error(w, "internal error", 500)
		return
	}
	writeJSON(w, resp)
}
```

## Next

- [Errors](errors.md) covers the rest of the error functions.
- [Concurrency](concurrency.md) is where tau saves the most code.
- [`examples/order_summary`](../examples/order_summary) is a whole
  endpoint, checked against eleven scenarios.
