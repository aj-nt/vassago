package context

// ProviderCapabilities describes what a provider supports.
// The pipeline dispatches on capabilities, not provider identity.
type ProviderCapabilities struct {
	SupportsStreaming      bool
	SupportsFunctionCalling bool
	SupportsPromptCaching bool
	SupportsVision        bool
	MaxContextTokens      int
}
