// This file is part of Vassago.
// See LICENSE for license information.

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

// cmdVersion handles "vassago version".
func cmdVersion(args []string) {
	fmt.Printf("Vassago %s\n", Version)
	fmt.Printf("  Commit: %s\n", Commit)
	fmt.Printf("  Built:  %s\n", BuildDate)
	fmt.Printf("  Go:     %s\n", runtime.Version())
	fmt.Printf("  OS:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
