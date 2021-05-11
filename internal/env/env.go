package env

import (
	"fmt"
	"os"
)

// Get returns the environment variable specified by the key k.
// If the variable is undefined it returns an error.
func Get(k string) (v string, err error) {
	v, ok := os.LookupEnv(k)
	if !ok {
		err = fmt.Errorf("environment variable (%s) not found", k)
	}
	return
}

// GetOrDefault returns the environment variable specified by the key k, or the
// default value d.
func GetOrDefault(k string, d string) string {
	v, ok := os.LookupEnv(k)
	if ok {
		return v
	}
	return d
}
