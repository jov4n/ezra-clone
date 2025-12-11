package tools

import (
	"context"
	"fmt"
	"time"
)

// ============================================================================
// Memory Tool Implementations
// ============================================================================

func (e *Executor) executeMemoryUpdate(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	name, _ := args["name"].(string)
	content, _ := args["content"].(string)

	if name == "" || content == "" {
		return &ToolResult{Success: false, Error: "name and content are required"}
	}

	err := e.repo.UpdateMemory(ctx, execCtx.AgentID, name, content)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Memory '%s' has been saved.", name),
	}
}

func (e *Executor) executeArchivalInsert(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	content, _ := args["content"].(string)
	if content == "" {
		return &ToolResult{Success: false, Error: "content is required"}
	}

	// For now, archival insert uses the same mechanism as memory
	// In a full implementation, this would go to a separate archival storage
	err := e.repo.UpdateMemory(ctx, execCtx.AgentID, fmt.Sprintf("archival_%d", time.Now().Unix()), content)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Message: "Information archived successfully.",
	}
}

func (e *Executor) executeMemorySearch(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	results, err := e.repo.SearchMemory(ctx, execCtx.AgentID, query, limit)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    results,
		Message: fmt.Sprintf("Found %d results for '%s'", len(results), query),
	}
}

