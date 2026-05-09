// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// newTestModel creates a minimal teaModel suitable for unit testing.
func newTestModel() teaModel {
	ta := textarea.New()
	ta.SetWidth(80)
	ta.SetHeight(1)

	vp := viewport.New(80, 20)

	return teaModel{
		input:      ta,
		viewport:   vp,
		phase:      phaseReady,
		approvalCh: make(chan bool, 1),
		width:      80,
		height:     24,
		historyIdx: -1,
		interrupt:  agentctx.NewInterruptSignal(),
	}
}

// TestApprovalRequestMsgTransition tests that receiving an approvalRequestMsg
// transitions the model to phaseApproval and sets pending state.
func TestApprovalRequestMsgTransition(t *testing.T) {
	m := newTestModel()
	m.phase = phaseStreaming

	msg := approvalRequestMsg{
		toolName: "FileWrite",
		args:     map[string]interface{}{"path": "/tmp/test.txt", "content": "hello"},
	}

	updated, _ := m.Update(msg)
	model := updated.(teaModel)

	if model.phase != phaseApproval {
		t.Errorf("expected phaseApproval, got %v", model.phase)
	}
	if model.pendingApprovalToolName != "FileWrite" {
		t.Errorf("expected pendingApprovalToolName='FileWrite', got %q", model.pendingApprovalToolName)
	}
	if model.pendingApprovalArgs["path"] != "/tmp/test.txt" {
		t.Errorf("expected pendingApprovalArgs path='/tmp/test.txt', got %v", model.pendingApprovalArgs["path"])
	}
}

// TestApprovalRequestMsgWithCommand tests that Execute tools show their command in the prompt.
func TestApprovalRequestMsgWithCommand(t *testing.T) {
	m := newTestModel()
	m.phase = phaseToolCall
	m.chatHistory = []string{"previous message"}

	msg := approvalRequestMsg{
		toolName: "Execute",
		args:     map[string]interface{}{"command": "rm -rf /tmp/test"},
	}

	updated, _ := m.Update(msg)
	model := updated.(teaModel)

	if model.phase != phaseApproval {
		t.Errorf("expected phaseApproval, got %v", model.phase)
	}
	// Chat history should now include the approval prompt
	if len(model.chatHistory) <= 1 {
		t.Error("expected chatHistory to have approval prompt added")
	}
}

// TestApprovalApproveWithEnter tests that pressing Enter with "y" approves the tool.
func TestApprovalApproveWithEnter(t *testing.T) {
	m := newTestModel()
	m.phase = phaseApproval
	m.pendingApprovalToolName = "FileWrite"
	m.input.SetValue("y")

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, _ := m.Update(keyMsg)
	model := updated.(teaModel)

	// Should have sent true to the approval channel
	select {
	case approved := <-m.approvalCh:
		if !approved {
			t.Error("expected approval to be true for 'y' input")
		}
	default:
		t.Error("expected approval result on channel")
	}

	// Phase should transition back to tool call (streaming continues)
	if model.phase != phaseToolCall {
		t.Errorf("expected phaseToolCall after approval, got %v", model.phase)
	}
}

// TestApprovalDenyWithEnter tests that pressing Enter with "n" denies the tool.
func TestApprovalDenyWithEnter(t *testing.T) {
	m := newTestModel()
	m.phase = phaseApproval
	m.pendingApprovalToolName = "FileWrite"
	m.input.SetValue("n")

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, _ := m.Update(keyMsg)
	model := updated.(teaModel)

	// Should have sent false to the approval channel
	select {
	case approved := <-m.approvalCh:
		if approved {
			t.Error("expected approval to be false for 'n' input")
		}
	default:
		t.Error("expected approval result on channel")
	}

	if model.phase != phaseToolCall {
		t.Errorf("expected phaseToolCall after denial, got %v", model.phase)
	}
}

// TestApprovalDenyWithCtrlC tests that Ctrl+C denies the tool and resets state.
func TestApprovalDenyWithCtrlC(t *testing.T) {
	m := newTestModel()
	m.phase = phaseApproval
	m.pendingApprovalToolName = "Execute"
	m.responding = true
	m.thinking = true
	// Create a cancel context so cancel() is available
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel

	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updated, _ := m.Update(keyMsg)
	model := updated.(teaModel)

	// Should have sent false to the approval channel
	select {
	case approved := <-m.approvalCh:
		if approved {
			t.Error("expected approval to be false for Ctrl+C")
		}
	default:
		t.Error("expected approval result on channel")
	}

	// Should reset to ready phase
	if model.phase != phaseReady {
		t.Errorf("expected phaseReady after Ctrl+C denial, got %v", model.phase)
	}
	if model.responding {
		t.Error("expected responding=false after Ctrl+C denial")
	}
	if model.thinking {
		t.Error("expected thinking=false after Ctrl+C denial")
	}
}

// TestApprovalEmptyInput tests that pressing Enter with empty input denies the tool.
func TestApprovalEmptyInput(t *testing.T) {
	m := newTestModel()
	m.phase = phaseApproval
	m.pendingApprovalToolName = "FileWrite"

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, _ := m.Update(keyMsg)
	model := updated.(teaModel)

	select {
	case approved := <-m.approvalCh:
		if approved {
			t.Error("expected approval to be false for empty input")
		}
	default:
		t.Error("expected approval result on channel")
	}

	if model.phase != phaseToolCall {
		t.Errorf("expected phaseToolCall after denial, got %v", model.phase)
	}
}

// TestApprovalYesInput tests that "yes" also approves.
func TestApprovalYesInput(t *testing.T) {
	m := newTestModel()
	m.phase = phaseApproval
	m.pendingApprovalToolName = "FileWrite"
	m.input.SetValue("yes")

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, _ := m.Update(keyMsg)
	model := updated.(teaModel)

	select {
	case approved := <-m.approvalCh:
		if !approved {
			t.Error("expected approval to be true for 'yes' input")
		}
	default:
		t.Error("expected approval result on channel")
	}

	if model.phase != phaseToolCall {
		t.Errorf("expected phaseToolCall after approval, got %v", model.phase)
	}
}