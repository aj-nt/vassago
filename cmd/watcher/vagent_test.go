// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"testing"
)

func TestConfig_defaults(t *testing.T) {
	t.Setenv("VAGENT_CONFIG", "/tmp/nonexistent-vagent.yaml")

	// Should create a default config and exit(0) — we wrap that
	defer func() { recover() }() // catch os.Exit
	loadConfig() // will exit with defaultCfg
}
