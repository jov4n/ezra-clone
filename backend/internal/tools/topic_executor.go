package tools

import (
	"context"
	"fmt"
)

// ============================================================================
// Topic Tool Implementations
// ============================================================================

func (e *Executor) executeCreateTopic(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	name, _ := args["name"].(string)
	if name == "" {
		return &ToolResult{Success: false, Error: "name is required"}
	}

	description, _ := args["description"].(string)

	topic, err := e.repo.CreateTopic(ctx, name, description)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    topic,
		Message: fmt.Sprintf("Topic '%s' created.", name),
	}
}

func (e *Executor) executeLinkTopics(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	topic1, _ := args["topic1"].(string)
	topic2, _ := args["topic2"].(string)
	relationship, _ := args["relationship"].(string)

	if topic1 == "" || topic2 == "" {
		return &ToolResult{Success: false, Error: "topic1 and topic2 are required"}
	}

	if relationship == "" {
		relationship = "RELATED_TO"
	}

	err := e.repo.LinkTopics(ctx, topic1, topic2, relationship)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Linked '%s' to '%s' with relationship '%s'", topic1, topic2, relationship),
	}
}

func (e *Executor) executeFindRelated(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return &ToolResult{Success: false, Error: "topic is required"}
	}

	depth := 2
	if d, ok := args["depth"].(float64); ok {
		depth = int(d)
	}

	topics, err := e.repo.GetRelatedTopics(ctx, topic, depth)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    topics,
		Message: fmt.Sprintf("Found %d related topics", len(topics)),
	}
}

func (e *Executor) executeLinkUserTopic(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return &ToolResult{Success: false, Error: "topic is required"}
	}

	userID, _ := args["user_id"].(string)
	if userID == "" {
		userID = execCtx.UserID
	}

	err := e.repo.LinkUserToTopic(ctx, userID, topic)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Linked user to topic '%s'", topic),
	}
}

