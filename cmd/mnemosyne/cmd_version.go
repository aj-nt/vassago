// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"fmt"
	"runtime"
)

// Build variables set via ldflags during compilation.
var (
	Version   = "v0.0.30-dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// cmdVersion handles "mnemosyne version".
func cmdVersion(args []string) {
	fmt.Printf("Mnemosyne %s\n", Version)
	fmt.Printf("  Commit: %s\n", Commit)
	fmt.Printf("  Built:  %s\n", BuildDate)
	fmt.Printf("  Go:     %s\n", runtime.Version())
	fmt.Printf("  OS:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
}