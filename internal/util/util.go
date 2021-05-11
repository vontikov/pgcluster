package util

import (
	"crypto/rand"
	"io/ioutil"
	"math/big"
	"strings"
)

var hostHostnamePath = "/etc/host_hostname"

// HostHostname returns hostname from file defined in constant hostHostnamePath
// or the defaultValue in case the file does not exits.
func HostHostname(defaultValue string) string {
	if content, err := ioutil.ReadFile(hostHostnamePath); err == nil {
		return strings.Trim(string(content), "\n")
	}
	return defaultValue
}

// RandString returns random string generated from symbols specified by src
func RandString(l int, src string) (string, error) {
	max := big.NewInt(int64(len(src)))
	b := make([]byte, l)
	for i := range b {
		r, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		n := r.Int64()
		b[i] = src[n]
	}
	return string(b), nil
}

// PanicOnError panics if the error is not nil.
func PanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
