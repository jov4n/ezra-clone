package state

import (
	"fmt"
	"time"
)

// ContextWindow represents the complete context that the LLM sees
// This is the "Memory OS" structure that gets serialized to JSON for the system prompt
type ContextWindow struct {
	Identity    AgentIdentity       `json:"identity"`
	CoreMemory  []MemoryBlock       `json:"core_memory"`  // Editable instructions
	ArchivalRefs []ArchivalPointer  `json:"archival_refs"` // Read-only history references
	UserContext map[string]interface{} `json:"user_context"` // Per-user context
}

// AgentIdentity represents the agent's core identity and personality
type AgentIdentity struct {
	Name        string   `json:"name"`
	Personality string   `json:"personality"`
	Capabilities []string `json:"capabilities"`
}

// MemoryBlock represents a single editable memory/instruction block
type MemoryBlock struct {
	Name      string    `json:"name"`      // Block identifier (e.g., "coding_style", "identity")
	Content   string    `json:"content"`   // The rule/instruction text
	UpdatedAt time.Time `json:"updated_at"`
}

// ArchivalPointer represents a reference to archived conversation history
type ArchivalPointer struct {
	Summary        string    `json:"summary"`
	Timestamp      time.Time `json:"timestamp"`
	RelevanceScore float64   `json:"relevance_score"`
}

// Validate checks if the ContextWindow is valid
func (cw *ContextWindow) Validate() error {
	if cw.Identity.Name == "" {
		return ErrInvalidContextWindow{Field: "identity.name", Reason: "cannot be empty"}
	}
	for i, block := range cw.CoreMemory {
		if err := block.Validate(); err != nil {
			return ErrInvalidMemoryBlock{Index: i, Err: err}
		}
	}
	return nil
}

// Validate checks if the MemoryBlock is valid
func (mb *MemoryBlock) Validate() error {
	if mb.Name == "" {
		return ErrInvalidMemoryBlock{Err: fmt.Errorf("name cannot be empty")}
	}
	return nil
}

// Errors

type ErrInvalidContextWindow struct {
	Field  string
	Reason string
}

func (e ErrInvalidContextWindow) Error() string {
	return fmt.Sprintf("invalid context window: %s - %s", e.Field, e.Reason)
}

type ErrInvalidMemoryBlock struct {
	Index int
	Err   error
}

func (e ErrInvalidMemoryBlock) Error() string {
	if e.Index >= 0 {
		return fmt.Sprintf("invalid memory block at index %d: %v", e.Index, e.Err)
	}
	return fmt.Sprintf("invalid memory block: %v", e.Err)
}

