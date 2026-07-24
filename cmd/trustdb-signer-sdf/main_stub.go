//go:build !sdf || !cgo || (!linux && !darwin)

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "trustdb SDF signer requires Linux or macOS, CGO_ENABLED=1, and -tags=sdf")
	os.Exit(1)
}
