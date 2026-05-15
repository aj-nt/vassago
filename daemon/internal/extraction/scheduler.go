// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package extraction

import (
	"context"
	"sync"
	"time"
)

// TriggerFunc is called when extraction should run for a session.
type TriggerFunc func(sessionID string)

// Scheduler manages debounced memory extraction triggers per session.
// Each Notify call resets the debounce timer. After the quiet period
// (DebounceInterval), the onTrigger callback fires. The hard cap ensures
// extraction occurs at least every HardCapInterval even if messages
// keep arriving.
type Scheduler struct {
	config    Config
	onTrigger TriggerFunc

	mu       sync.Mutex
	sessions map[string]*sessionState
	ctx      context.Context
	cancel   context.CancelFunc
}

type sessionState struct {
	lastMessage    time.Time
	lastExtraction time.Time
	debounceTimer  *time.Timer
	hardCapTimer   *time.Timer
}

// NewScheduler creates a new extraction scheduler.
// If DebounceInterval is 0, the scheduler starts in disabled mode.
func NewScheduler(config Config, onTrigger TriggerFunc) *Scheduler {
	return &Scheduler{
		config:    config,
		onTrigger: onTrigger,
		sessions:  make(map[string]*sessionState),
	}
}

// Enabled returns true if the scheduler will actually trigger extractions.
func (s *Scheduler) Enabled() bool {
	return s.config.DebounceInterval > 0
}

// Start begins the scheduler's background goroutine.
func (s *Scheduler) Start(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
}

// Stop cancels all pending timers and shuts down.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, state := range s.sessions {
		if state.debounceTimer != nil {
			state.debounceTimer.Stop()
		}
		if state.hardCapTimer != nil {
			state.hardCapTimer.Stop()
		}
	}
	s.sessions = make(map[string]*sessionState)

	if s.cancel != nil {
		s.cancel()
	}
}

// Notify indicates new messages have arrived for a session.
// This resets the debounce timer. If the hard cap timer isn't running,
// it starts one. Extraction fires on the first of: debounce quiet period
// elapsed, or hard cap elapsed.
func (s *Scheduler) Notify(sessionID string) {
	if !s.Enabled() {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	state, exists := s.sessions[sessionID]
	if !exists {
		state = &sessionState{lastExtraction: now}
		s.sessions[sessionID] = state
	}
	state.lastMessage = now

	// Reset debounce timer
	if state.debounceTimer != nil {
		state.debounceTimer.Stop()
	}
	debounceDur := time.Duration(s.config.DebounceInterval) * time.Second
	state.debounceTimer = time.AfterFunc(debounceDur, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if st, ok := s.sessions[sessionID]; ok {
			st.lastExtraction = time.Now()
		}
		if s.onTrigger != nil {
			s.onTrigger(sessionID)
		}
	})

	// Start hard cap timer if not already running
	if state.hardCapTimer == nil {
		hardCapDur := time.Duration(s.config.HardCapInterval) * time.Second
		state.hardCapTimer = time.AfterFunc(hardCapDur, func() {
			s.mu.Lock()
			if st, ok := s.sessions[sessionID]; ok {
				st.lastExtraction = time.Now()
				// Stop debounce since hard cap fired
				if st.debounceTimer != nil {
					st.debounceTimer.Stop()
					st.debounceTimer = nil
				}
				// Clear hard cap for next cycle
				st.hardCapTimer = nil
			}
			s.mu.Unlock()
			if s.onTrigger != nil {
				s.onTrigger(sessionID)
			}
		})
	}
}
