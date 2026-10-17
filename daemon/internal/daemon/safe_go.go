// This file is part of Vassago.
// See LICENSE for license information.

package daemon

import (
	"log/slog"
	"runtime/debug"
)

// safeGo runs fn in a goroutine with panic recovery.
// If fn panics, the error is logged with a stack trace and the goroutine
// terminates cleanly — the daemon stays up.
func safeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("background goroutine panicked",
					"goroutine", name,
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
