package tau

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

// FieldError is one rule that one field failed.
type FieldError struct {
	Field string // path using JSON names, e.g. "address.city" or "tags[2]"
	Rule  string // the rule that failed, e.g. "required", "gte"
	Param string // the rule's argument, e.g. "18"; empty if it has none
	Value any    // the offending value
}

func (f FieldError) String() string {
	rule := f.Rule
	if f.Param != "" {
		rule += "=" + f.Param
	}
	return fmt.Sprintf("%s must satisfy %s (got %#v)", f.Field, rule, f.Value)
}

// ValidationError is thrown by Validate and ParseJSON when a value breaks the
// rules in its `validate` struct tags. Fields lists every failure.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.String()
	}
	return strings.Join(parts, "; ")
}

var validate = func() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	// Report fields by their JSON names, which is what the sender of the data sees.
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			return f.Name
		}
		return name
	})
	return v
}()

// The validator has to be told about every Field[T] type it will meet, and
// registering one is not safe while a validation is running. validateMu lets
// validations run together and registrations run alone; knownTypes records
// the types already searched.
var (
	validateMu sync.RWMutex
	knownTypes sync.Map // reflect.Type -> struct{}
)

type fieldLike interface{ validationPointer() any }

var fieldLikeType = reflect.TypeFor[fieldLike]()

// registerFields finds the Field types reachable from t and teaches the
// validator to look inside them. It is a no-op for a type seen before.
func registerFields(t reflect.Type) {
	if _, seen := knownTypes.Load(t); seen {
		return
	}
	var found []reflect.Type
	visited := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		if visited[t] {
			return
		}
		visited[t] = true
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			walk(t.Elem())
		case reflect.Struct:
			if t.Implements(fieldLikeType) {
				found = append(found, t)
				return
			}
			for i := range t.NumField() {
				if f := t.Field(i); f.IsExported() {
					walk(f.Type)
				}
			}
		}
	}
	walk(t)

	validateMu.Lock()
	defer validateMu.Unlock()
	for _, ft := range found {
		if _, seen := knownTypes.LoadOrStore(ft, struct{}{}); !seen {
			validate.RegisterCustomTypeFunc(func(field reflect.Value) any {
				return field.Interface().(fieldLike).validationPointer()
			}, reflect.Zero(ft).Interface())
		}
	}
	knownTypes.Store(t, struct{}{})
}

// Validate checks v against the rules in its `validate` struct tags, as
// defined by go-playground/validator, and throws a *ValidationError listing
// every failure:
//
//	type Signup struct {
//		Email string `json:"email" validate:"required,email"`
//		Age   int    `json:"age" validate:"gte=18"`
//	}
//
// It then calls v's Validate() error method, if it has one, for rules that
// tags cannot express, and throws that error. The method may be declared on
// T or *T. Structs, pointers to structs (at any depth), and slices, arrays
// and maps of them are checked; other values, and nil pointers, pass.
func Validate(v any) {
	// Follow pointers down to the value. On the way, pick up a Validate()
	// method from the outermost level that has one, so it is found whether it
	// is declared on T or *T and however many pointers v arrives behind.
	var custom interface{ Validate() error }
	rv := reflect.ValueOf(v)
	for rv.IsValid() {
		isNilPointer := rv.Kind() == reflect.Pointer && rv.IsNil()
		if c, ok := rv.Interface().(interface{ Validate() error }); ok && custom == nil && !isNilPointer {
			custom = c
		}
		if rv.Kind() != reflect.Pointer || isNilPointer {
			break
		}
		rv = rv.Elem()
	}

	var err error
	topLevelStruct := false
	if rv.IsValid() {
		registerFields(rv.Type())
		// Held only around the validator itself: a Validate() method called
		// below may validate a type not seen before, which needs the write lock.
		validateMu.RLock()
		switch rv.Kind() {
		case reflect.Struct:
			topLevelStruct = true
			err = validate.Struct(rv.Interface())
		case reflect.Slice, reflect.Array, reflect.Map:
			err = validate.Var(rv.Interface(), "dive")
		}
		validateMu.RUnlock()
	}
	if failures, ok := errors.AsType[validator.ValidationErrors](err); ok {
		out := &ValidationError{}
		for _, f := range failures {
			// A struct's paths start with its type name, which the sender of
			// the data never sees: "signup.address.city" -> "address.city".
			// Paths into a slice or map start with the index and stay whole.
			field := f.Namespace()
			if _, rest, found := strings.Cut(field, "."); topLevelStruct && found {
				field = rest
			}
			out.Fields = append(out.Fields, FieldError{field, f.Tag(), f.Param(), f.Value()})
		}
		Throw(out)
	}
	Check(err)
	if custom != nil {
		Check(custom.Validate())
	}
}
