package provider

import (
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// ProviderRegistry maps provider names to Provider implementations.
// Supports fallback chains for resilience.
type ProviderRegistry struct {
	providers     map[string]Provider
	fallbackChain []string
	primary       string // Primary provider name (set from config)
}

// NewProviderRegistry creates a new registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
		primary:   "openai", // backwards-compatible default
	}
}

// Register adds a provider to the registry.
func (r *ProviderRegistry) Register(name string, p Provider) {
	r.providers[name] = p
}

// SetPrimary sets the primary provider name.
func (r *ProviderRegistry) SetPrimary(name string) {
	r.primary = name
}

// Primary returns the primary provider name.
func (r *ProviderRegistry) Primary() string {
	return r.primary
}

// Get returns a provider by name, or an error if not found.
func (r *ProviderRegistry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", name)
	}
	return p, nil
}

// SetFallbackChain configures the fallback chain (primary → fallback1 → fallback2).
func (r *ProviderRegistry) SetFallbackChain(chain []string) {
	r.fallbackChain = chain
}

// GetPrimary returns the primary provider, trying the configured primary name.
func (r *ProviderRegistry) GetPrimary() (Provider, error) {
	return r.GetWithFallback(r.primary)
}

// GetWithFallback returns the named provider, or tries fallbacks if it fails.
func (r *ProviderRegistry) GetWithFallback(primary string) (Provider, error) {
	// Try primary first
	if p, err := r.Get(primary); err == nil {
		return p, nil
	}
	// Try fallback chain
	for _, name := range r.fallbackChain {
		if name == primary {
			continue
		}
		if p, err := r.Get(name); err == nil {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no available provider (primary: %s, fallbacks: %v)", primary, r.fallbackChain)
}

// List returns all registered provider names.
func (r *ProviderRegistry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// CapabilitiesOf returns the capabilities for a named provider.
func (r *ProviderRegistry) CapabilitiesOf(name string) (agentctx.ProviderCapabilities, error) {
	p, err := r.Get(name)
	if err != nil {
		return agentctx.ProviderCapabilities{}, err
	}
	return p.Capabilities(), nil
}