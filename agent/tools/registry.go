package tools

import "fmt"

// ToolHandler processes a tool call and returns (result, isError).
type ToolHandler func(args map[string]interface{}) (string, bool)

// ToolRegistry maps tool names to their handler functions.
type ToolRegistry struct {
	handlers map[string]ToolHandler
	schemas  map[string]map[string]interface{}
}

// NewToolRegistry creates a new tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		handlers: make(map[string]ToolHandler),
		schemas:  make(map[string]map[string]interface{}),
	}
}

// Register adds a tool handler with its JSON schema definition.
func (r *ToolRegistry) Register(name string, handler ToolHandler, schema map[string]interface{}) {
	r.handlers[name] = handler
	r.schemas[name] = schema
}

// Get returns a tool handler by name.
func (r *ToolRegistry) Get(name string) (ToolHandler, error) {
	h, ok := r.handlers[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return h, nil
}

// List returns all registered tool names.
func (r *ToolRegistry) List() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// Schemas returns all tool schemas for LLM tool definitions.
func (r *ToolRegistry) Schemas() map[string]map[string]interface{} {
	return r.schemas
}
