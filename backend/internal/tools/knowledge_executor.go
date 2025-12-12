package tools

import (
	"context"
	"fmt"
	"strings"
)

// ============================================================================
// Knowledge Tool Implementations
// ============================================================================

func (e *Executor) executeCreateFact(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	content, _ := args["content"].(string)
	if content == "" {
		return &ToolResult{Success: false, Error: "content is required"}
	}

	source, _ := args["source"].(string)
	
	var topics []string
	if topicsArg, ok := args["topics"].([]interface{}); ok {
		for _, t := range topicsArg {
			if ts, ok := t.(string); ok {
				topics = append(topics, ts)
			}
		}
	}

	fact, err := e.repo.CreateFact(ctx, execCtx.AgentID, content, source, execCtx.UserID, topics)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    fact,
		Message: fmt.Sprintf("Fact stored: %s", content),
	}
}

func (e *Executor) executeSearchFacts(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return &ToolResult{Success: false, Error: "topic is required"}
	}

	facts, err := e.repo.GetFactsAboutTopic(ctx, topic)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    facts,
		Message: fmt.Sprintf("Found %d facts about '%s'", len(facts), topic),
	}
}

func (e *Executor) executeGetUserContext(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	userID, _ := args["user_id"].(string)
	if userID == "" {
		userID = execCtx.UserID
	}

	userCtx, err := e.repo.GetUserContext(ctx, userID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	// Build a conversational-friendly message with context for the LLM
	// Format it naturally so the LLM can respond conversationally
	var messageParts []string
	
	if userCtx != nil {
		// Collect potential names from facts (facts that mention names)
		nameKeywords := make(map[string]bool)
		var personalFacts []string
		var otherFacts []string
		
		for _, fact := range userCtx.Facts {
			factLower := strings.ToLower(fact.Content)
			// Check if this fact is about a name (various patterns)
			if strings.Contains(factLower, "name is") || 
			   strings.Contains(factLower, "my name") ||
			   strings.Contains(factLower, "user's name") ||
			   strings.Contains(factLower, "user name") ||
			   strings.Contains(factLower, "i'm ") ||
			   strings.Contains(factLower, "i am ") {
				personalFacts = append(personalFacts, fact.Content)
				
				// Try to extract the name for filtering - handle multiple patterns
				var name string
				if strings.Contains(factLower, "name is") {
					// Handle "name is X" or "name's X" patterns
					parts := strings.Split(fact.Content, "name is")
					if len(parts) > 1 {
						name = strings.TrimSpace(parts[1])
					}
				} else if strings.Contains(factLower, "user's name") {
					parts := strings.Split(fact.Content, "name")
					if len(parts) > 1 {
						// Get text after "name"
						afterName := strings.Join(parts[1:], "name")
						if strings.Contains(afterName, "is") {
							nameParts := strings.Split(afterName, "is")
							if len(nameParts) > 1 {
								name = strings.TrimSpace(nameParts[1])
							}
						}
					}
				}
				
				// Clean up the name and add to keywords
				if name != "" {
					// Remove punctuation and common words
					name = strings.Trim(name, ".,!?;: ")
					// Split and take first word (the actual name)
					nameWords := strings.Fields(name)
					if len(nameWords) > 0 {
						nameKeywords[strings.ToLower(nameWords[0])] = true
					}
				}
			} else {
				otherFacts = append(otherFacts, fact.Content)
			}
		}
		
		// Filter out topics that are likely names (appear in name-related facts)
		var actualInterests []string
		for _, topic := range userCtx.Topics {
			topicLower := strings.ToLower(topic.Name)
			// Skip if this topic matches a name we found in facts
			if !nameKeywords[topicLower] {
				actualInterests = append(actualInterests, topic.Name)
			}
		}
		
		// Build a natural, conversational summary for the LLM
		if len(personalFacts) > 0 || len(otherFacts) > 0 || len(actualInterests) > 0 {
			// List all facts naturally
			allFacts := append(personalFacts, otherFacts...)
			if len(allFacts) > 0 {
				messageParts = append(messageParts, "Facts about the user:")
				for _, fact := range allFacts {
					messageParts = append(messageParts, fmt.Sprintf("- %s", fact))
				}
			}
			
			// Only mention interests if there are actual interests (not just names)
			if len(actualInterests) > 0 {
				if len(messageParts) > 0 {
					messageParts = append(messageParts, "")
				}
				messageParts = append(messageParts, "Topics of interest:")
				for _, interest := range actualInterests {
					messageParts = append(messageParts, fmt.Sprintf("- %s", interest))
				}
			}
		} else {
			messageParts = append(messageParts, "No information found yet about this user.")
		}
	}

	return &ToolResult{
		Success: true,
		Data:    userCtx,
		Message: strings.Join(messageParts, "\n"),
	}
}

