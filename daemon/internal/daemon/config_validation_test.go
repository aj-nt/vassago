package daemon

import (
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr string // substring that must appear in error, empty = no error expected
	}{
		{
			name: "valid config",
			config: Config{
				Daemon: DaemonConfig{
					Socket: "/tmp/mnemosyne.sock",
					TCP:    "localhost:50051",
				},
				Store: StoreConfig{
					Path:        "/tmp/mnemosyne.db",
					WALMode:     true,
					BusyTimeout: 5000,
				},
				HotBlock: HotBlockConfig{
					MemoryCharLimit:    4000,
					UserCharLimit:      2000,
					MinPriority:        2,
					ObservationTTLDays: 30,
				},
			},
			wantErr: "",
		},
		{
			name: "missing daemon socket and TCP",
			config: Config{
				Daemon: DaemonConfig{},
				Store: StoreConfig{
					Path:        "/tmp/mnemosyne.db",
					WALMode:     true,
					BusyTimeout: 5000,
				},
				HotBlock: HotBlockConfig{
					MemoryCharLimit:    4000,
					UserCharLimit:      2000,
					MinPriority:        2,
					ObservationTTLDays: 30,
				},
			},
			wantErr: "daemon.socket or daemon.tcp must be configured",
		},
		{
			name: "invalid TLS config - only cert",
			config: Config{
				Daemon: DaemonConfig{
					Socket: "/tmp/mnemosyne.sock",
					TLS: TLSConfig{
						Cert: "/path/to/cert",
					},
				},
				Store: StoreConfig{
					Path:        "/tmp/mnemosyne.db",
					WALMode:     true,
					BusyTimeout: 5000,
				},
				HotBlock: HotBlockConfig{
					MemoryCharLimit:    4000,
					UserCharLimit:      2000,
					MinPriority:        2,
					ObservationTTLDays: 30,
				},
			},
			wantErr: "daemon.tls.cert and daemon.tls.key must both be set or both be empty",
		},
		{
			name: "invalid busy timeout",
			config: Config{
				Daemon: DaemonConfig{
					Socket: "/tmp/mnemosyne.sock",
				},
				Store: StoreConfig{
					Path:        "/tmp/mnemosyne.db",
					WALMode:     true,
					BusyTimeout: 50000, // Too high
				},
				HotBlock: HotBlockConfig{
					MemoryCharLimit:    4000,
					UserCharLimit:      2000,
					MinPriority:        2,
					ObservationTTLDays: 30,
				},
			},
			wantErr: "database.busy_timeout must be between 1000 and 30000 ms",
		},
		{
			name: "invalid observation TTL",
			config: Config{
				Daemon: DaemonConfig{
					Socket: "/tmp/mnemosyne.sock",
				},
				Store: StoreConfig{
					Path:        "/tmp/mnemosyne.db",
					WALMode:     true,
					BusyTimeout: 5000,
				},
				HotBlock: HotBlockConfig{
					MemoryCharLimit:    4000,
					UserCharLimit:      2000,
					MinPriority:        2,
					ObservationTTLDays: 500, // Too high
				},
			},
			wantErr: "hot_block.observation_ttl_days must be between 7 and 365",
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

// helper
func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}