// Thrown errors: throw inside, return a plain error at the boundary.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/egoisutolabs/tau"
)

type NotFoundError struct{ Key string }

func (e *NotFoundError) Error() string { return "not found: " + e.Key }

var settings = map[string]string{"port": "8080", "retries": "three"}

// lookup is an ordinary Go function: it returns an error.
func lookup(key string) (string, error) {
	if v, ok := settings[key]; ok {
		return v, nil
	}
	return "", &NotFoundError{key}
}

// intSetting throws. Wrapf adds the key to whatever goes wrong inside.
func intSetting(key string) int {
	defer tau.Wrapf("setting %q", key)
	return tau.Must(strconv.Atoi(tau.Must(lookup(key))))
}

// Port is a boundary: callers get a plain (int, error) and never see a throw.
func Port() (port int, err error) {
	defer tau.Catch(&err)
	return intSetting("port"), nil
}

// Retries is a boundary that fails, because "three" is not a number.
func Retries() (n int, err error) {
	defer tau.Catch(&err)
	return intSetting("retries"), nil
}

// settingOr catches one error type and lets every other error keep going.
func settingOr(key string, fallback int) (n int) {
	defer tau.Handle(func(*NotFoundError) { n = fallback })
	return intSetting(key)
}

func main() {
	port, err := Port()
	check(err == nil && port == 8080, "Port() = %d, %v", port, err)

	_, err = Retries()
	check(err != nil && strings.HasPrefix(err.Error(), `setting "retries": `), "Retries() error = %v", err)
	_, isNumErr := errors.AsType[*strconv.NumError](err)
	check(isNumErr, "errors.As should see the original error through the added context")
	check(strings.Contains(tau.StackOf(err), "intSetting"), "the stack should name the function that threw")

	check(settingOr("timeout", 30) == 30, "a missing setting should fall back")
	err = tau.Try(func() { settingOr("retries", 3) })
	check(err != nil, "a malformed setting is not a NotFoundError and must not be swallowed")

	fmt.Printf("port %d\nretries failed: %v\nthrown at:\n%s", port, err, firstFrame(err))
}

func firstFrame(err error) string {
	lines := strings.SplitN(tau.StackOf(err), "\n", 3)
	return strings.Join(lines[:2], "\n") + "\n"
}

func check(ok bool, format string, args ...any) {
	if !ok {
		fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
		os.Exit(1)
	}
}
