// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

func setupTestRegistry() *provider.ProviderRegistry {
	return provider.NewProviderRegistry()
}

func setupTestToolReg() *tools.ToolRegistry {
	return tools.NewToolRegistry()
}

func setupTestRouter() *provider.ModelRouter {
	return provider.NewModelRouter("always-primary")
}