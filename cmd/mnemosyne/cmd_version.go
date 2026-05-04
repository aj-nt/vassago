// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"fmt"
	"runtime"
)

// cmdVersion handles "mnemosyne version".
func cmdVersion(args []string) {
	fmt.Println("Mnemosyne v0.19.0")
	fmt.Printf("  Go:     %s\n", runtime.Version())
	fmt.Printf("  OS:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
}