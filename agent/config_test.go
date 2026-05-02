package agent

import (
	"testing"
)

func TestAgentConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  AgentConfig
		wantErr string // substring that must appear in error, empty = no error expected
	}{
		{
			name: "valid config",
			config: AgentConfig{
				Agent: AgentSection{ID: "test", MaxTurns: 10},
				Provider: ProviderSection{Name: "anthropic", Model: "claude-sonnet-4-6"},
				Routing: RoutingSection{Strategy: "always-primary"},
				Mnemosyne: MnemosyneSection{Address: "localhost:50051"},
				HotBlock: HotBlockSection{
					MemoryCharLimit: 4000,
					UserCharLimit:   2000,
					MinPriority:    2,
				},
				Delegation: DelegationSection{
					Enabled:  false,
					MaxDepth: 1,
					MaxTurns: 10,
				},
				Scheduler: SchedulerSection{
					Enabled:  false,
					Interval: 3600,
				},
			},
			wantErr: "",
		},
		{
			name: "missing agent ID",
			config: AgentConfig{
				Agent:    AgentSection{MaxTurns: 10},
				Provider: ProviderSection{Name: "anthropic", Model: "claude-sonnet-4-6"},
				Mnemosyne: MnemosyneSection{Address: "localhost:50051"},
			},
			wantErr: "agent.id is required",
		},
		{
			name: "zero max turns",
			config: AgentConfig{
				Agent:    AgentSection{ID: "test", MaxTurns: 0},
				Provider: ProviderSection{Name: "anthropic", Model: "claude-sonnet-4-6"},
				Mnemosyne: MnemosyneSection{Address: "localhost:50051"},
			},
			wantErr: "agent.max_turns must be >= 1",
		},
		{
			name: "missing provider name",
			config: AgentConfig{
				Agent:     AgentSection{ID: "test", MaxTurns: 10},
				Provider:  ProviderSection{Model: "claude-sonnet-4-6"},
				Mnemosyne: MnemosyneSection{Address: "localhost:50051"},
			},
			wantErr: "provider.name is required",
		},
		{
			name: "missing provider model",
			config: AgentConfig{
				Agent:     AgentSection{ID: "test", MaxTurns: 10},
				Provider:  ProviderSection{Name: "anthropic"},
				Mnemosyne: MnemosyneSection{Address: "localhost:50051"},
			},
			wantErr: "provider.model is required",
		},
		{
			name: "missing mnemosyne address",
			config: AgentConfig{
				Agent:    AgentSection{ID: "test", MaxTurns: 10},
				Provider: ProviderSection{Name: "anthropic", Model: "claude-sonnet-4-6"},
			},
			wantErr: "mnemosyne.address is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.wantErr)
				} else if !contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate() error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestToAgentConfig(t *testing.T) {
	cfg := AgentConfig{
		Agent:      AgentSection{ID: "test-agent", Name: "Mnemo", MaxTurns: 20, Personality: "direct"},
		Provider:   ProviderSection{Name: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.7},
		Mnemosyne:  MnemosyneSection{Address: "localhost:50051"},
	}

	result := cfg.ToAgentConfig()
	if result.AgentID != "test-agent" {
		t.Errorf("AgentID = %q, want %q", result.AgentID, "test-agent")
	}
	if result.AgentName != "Mnemo" {
		t.Errorf("AgentName = %q, want %q", result.AgentName, "Mnemo")
	}
	if result.MaxTurns != 20 {
		t.Errorf("MaxTurns = %d, want 20", result.MaxTurns)
	}
	if result.Personality != "direct" {
		t.Errorf("Personality = %q, want %q", result.Personality, "direct")
	}
	if result.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", result.Temperature)
	}
}

// helper
func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}