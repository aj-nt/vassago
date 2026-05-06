// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import "testing"

func TestDefaultToolBudget(t *testing.T) {
	b := DefaultToolBudget()
	if b.FileRead != 5 {
		t.Errorf("Expected FileRead=5, got %d", b.FileRead)
	}
	if b.FileSearch != 3 {
		t.Errorf("Expected FileSearch=3, got %d", b.FileSearch)
	}
	if b.Execute != 10 {
		t.Errorf("Expected Execute=10, got %d", b.Execute)
	}
}

func TestToolBudgetConfig_CheckBudget(t *testing.T) {
	b := DefaultToolBudget()

	// Under limit → allowed
	if !b.CheckBudget("FileRead", 0) {
		t.Error("Expected FileRead call 0 to be allowed")
	}
	if !b.CheckBudget("FileRead", 4) {
		t.Error("Expected FileRead call 4 to be allowed")
	}

	// At limit → blocked
	if b.CheckBudget("FileRead", 5) {
		t.Error("Expected FileRead call 5 to be blocked (at limit)")
	}

	// Over limit → blocked
	if b.CheckBudget("FileRead", 100) {
		t.Error("Expected FileRead call 100 to be blocked")
	}

	// Unknown tool → unlimited
	if !b.CheckBudget("SearchMemories", 999) {
		t.Error("Expected unknown tool to have unlimited budget")
	}
}

func TestToolBudgetConfig_LimitFor(t *testing.T) {
	b := DefaultToolBudget()
	if b.LimitFor("FileRead") != 5 {
		t.Errorf("Expected FileRead limit 5, got %d", b.LimitFor("FileRead"))
	}
	if b.LimitFor("FileSearch") != 3 {
		t.Errorf("Expected FileSearch limit 3, got %d", b.LimitFor("FileSearch"))
	}
	if b.LimitFor("Execute") != 10 {
		t.Errorf("Expected Execute limit 10, got %d", b.LimitFor("Execute"))
	}
	if b.LimitFor("UnknownTool") != 0 {
		t.Errorf("Expected unknown tool limit 0 (unlimited), got %d", b.LimitFor("UnknownTool"))
	}
}

func TestToolBudgetConfig_CustomBudget(t *testing.T) {
	// Custom budget for testing
	b := ToolBudgetConfig{
		FileRead:    1,
		FileSearch: 2,
		Execute:     0, // unlimited
	}

	if b.CheckBudget("FileRead", 0) != true {
		t.Error("Expected FileRead call 0 to be allowed with custom limit 1")
	}
	if b.CheckBudget("FileRead", 1) != false {
		t.Error("Expected FileRead call 1 to be blocked with custom limit 1")
	}
	if b.CheckBudget("Execute", 999) != true {
		t.Error("Expected Execute to be unlimited with custom limit 0")
	}
}

func TestToolBudgetConfig_BudgetExceededMessage(t *testing.T) {
	b := DefaultToolBudget()
	msg := b.BudgetExceededMessage("FileRead", 5)
	if msg == "" {
		t.Error("Expected non-empty message for budget-exceeded tool")
	}
	if msg != FormatBudgetExceeded("FileRead", 5, 5) {
		t.Errorf("Expected FormatBudgetExceeded message, got %q", msg)
	}

	// Unlimited tools return empty
	msg = b.BudgetExceededMessage("SearchMemories", 1)
	if msg != "" {
		t.Errorf("Expected empty message for unlimited tool, got %q", msg)
	}
}