// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

// Package tools provides agent tool handlers for the Mnemosyne AI agent.
//
// Each tool is registered with a JSON Schema and a handler function. The agent
// calls these tools via the LLM's function-calling interface, and the handlers
// communicate with the daemon through a MnemoClient interface.
//
// Available tools: AddMemory, SearchMemories, ReplaceMemory, RemoveMemory,
// Consolidate, FileRead, FileWrite, FileSearch, Execute, Delegate, ListTodos,
// AddTodo, CompleteTodo, RemoveTodo, Browse, AnalyzeImage, GenerateImage,
// ScheduleCron, SendMessage, SkillsTool, SearchSessions.
//
// Tool arguments are validated against their JSON Schema before dispatch.
// See ValidateArgs in validate.go for details.
package tools