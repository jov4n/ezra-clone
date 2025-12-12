package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================================
// Discord Tool Implementations
// ============================================================================

func (e *Executor) executeDiscordReadHistory(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available (only works in Discord bot context)"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required"}
	}

	limit := 50
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	fromUserID, _ := args["from_user_id"].(string)

	messages, err := e.discordExecutor.ReadChannelHistory(ctx, channelID, limit, fromUserID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    messages,
		Message: fmt.Sprintf("Retrieved %d messages from channel", len(messages)),
	}
}

func (e *Executor) executeDiscordGetUserInfo(ctx context.Context, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available"}
	}

	userID, _ := args["user_id"].(string)
	if userID == "" {
		return &ToolResult{Success: false, Error: "user_id is required"}
	}

	userInfo, err := e.discordExecutor.GetUserInfo(ctx, userID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    userInfo,
	}
}

func (e *Executor) executeDiscordGetChannelInfo(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required"}
	}

	channelInfo, err := e.discordExecutor.GetChannelInfo(ctx, channelID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    channelInfo,
	}
}

// Admin user ID - only this user can access codebase reading
const AdminUserID = "121439120240279555"

func (e *Executor) executeReadCodebase(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	// Check if Discord executor is available
	if e.discordExecutor == nil || e.discordExecutor.session == nil {
		return &ToolResult{Success: false, Error: "Discord not available (only works in Discord bot context)"}
	}

	// Check if user is the admin
	if execCtx.UserID != AdminUserID {
		return &ToolResult{
			Success: false,
			Error:   "Unauthorized: This tool is only available to the bot administrator via DM",
		}
	}

	// Check if it's a DM
	channelInfo, err := e.discordExecutor.GetChannelInfo(ctx, execCtx.ChannelID)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to get channel info: %v", err)}
	}

	if channelInfo.Type != "dm" {
		return &ToolResult{
			Success: false,
			Error:   "Unauthorized: This tool can only be used in DMs",
		}
	}

	// Get query parameters
	query, _ := args["query"].(string)
	filePath, _ := args["file_path"].(string)
	maxResults := 5
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
		if maxResults > 20 {
			maxResults = 20
		}
		if maxResults < 1 {
			maxResults = 5
		}
	}

	// If file_path is provided, read that specific file
	if filePath != "" {
		return e.readSpecificFile(ctx, filePath)
	}

	// Otherwise, perform intelligent search
	if query == "" {
		return &ToolResult{
			Success: false,
			Error:   "Either 'query' or 'file_path' must be provided",
		}
	}

	return e.searchCodebase(ctx, query, maxResults)
}

// isSensitiveFile checks if a file should be excluded (env vars, secrets, etc.)
func isSensitiveFile(path string) bool {
	path = strings.ToLower(path)
	
	// Exclude common sensitive file patterns
	sensitivePatterns := []string{
		".env",
		"config.go", // May contain secrets
		"secrets",
		"credentials",
		"private",
		"token",
		"key",
		"password",
		"secret",
		"node_modules",
		".git",
		"vendor",
		"*.exe",
		"*.dll",
		"*.so",
		"go.sum", // Can be large and not useful
	}
	
	for _, pattern := range sensitivePatterns {
		if strings.Contains(path, pattern) {
			return true
		}
	}
	
	// Exclude binary files
	ext := filepath.Ext(path)
	binaryExts := []string{".exe", ".dll", ".so", ".bin", ".o", ".a"}
	for _, be := range binaryExts {
		if ext == be {
			return true
		}
	}
	
	return false
}

// shouldExcludeFile checks if a file should be excluded from codebase reading
func shouldExcludeFile(path string) bool {
	// Get relative path from backend directory
	relPath := path
	if strings.Contains(path, "backend") {
		parts := strings.Split(path, "backend")
		if len(parts) > 1 {
			relPath = "backend" + parts[1]
		}
	}
	
	// Exclude sensitive files
	if isSensitiveFile(relPath) {
		return true
	}
	
	// Exclude test files for now (can be enabled later)
	if strings.Contains(relPath, "_test.go") {
		return false // Allow test files for now
	}
	
	return false
}

// readSpecificFile reads a specific file from the codebase
func (e *Executor) readSpecificFile(ctx context.Context, filePath string) *ToolResult {
	// Normalize path - ensure it's relative to backend or workspace root
	normalizedPath := filePath
	if !strings.HasPrefix(filePath, "backend/") && !filepath.IsAbs(filePath) {
		normalizedPath = filepath.Join("backend", filePath)
	}
	
	// Check if file should be excluded
	if shouldExcludeFile(normalizedPath) {
		return &ToolResult{
			Success: false,
			Error:   "Access denied: This file contains sensitive information and cannot be read",
		}
	}
	
	// Try to read the file
	content, err := os.ReadFile(normalizedPath)
	if err != nil {
		// Try with absolute path from workspace
		workspaceRoot := filepath.Join("..", normalizedPath)
		content, err = os.ReadFile(workspaceRoot)
		if err != nil {
			// Try just the file path as-is
			content, err = os.ReadFile(filePath)
			if err != nil {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("Failed to read file: %v. Make sure the path is correct relative to the backend directory.", err),
				}
			}
		}
	}
	
	// Filter out environment variables from content
	filteredContent := filterEnvVars(string(content))
	
	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"file_path": normalizedPath,
			"content":   filteredContent,
			"lines":     strings.Count(filteredContent, "\n") + 1,
		},
		Message: fmt.Sprintf("Read file: %s (%d lines)", normalizedPath, strings.Count(filteredContent, "\n")+1),
	}
}

// filterEnvVars removes or redacts environment variable patterns from code
func filterEnvVars(content string) string {
	lines := strings.Split(content, "\n")
	var filtered []string
	
	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		
		// Skip lines that look like they're setting env vars with actual values
		if strings.Contains(lowerLine, "os.getenv") || 
		   strings.Contains(lowerLine, "os.setenv") ||
		   strings.Contains(lowerLine, "getenv(") {
			// Keep the line but it's safe (just reading, not exposing)
			filtered = append(filtered, line)
			continue
		}
		
		// Redact common env var patterns
		patterns := []string{
			"discord_bot_token",
			"neo4j_password",
			"neo4j_user",
			"openrouter_api_key",
			"runpod_api_key",
			"api_key",
			"secret",
			"password",
			"token",
		}
		
		shouldRedact := false
		for _, pattern := range patterns {
			if strings.Contains(lowerLine, pattern) && 
			   (strings.Contains(lowerLine, "=") || strings.Contains(lowerLine, ":")) {
				// This might be an env var assignment
				if strings.Contains(lowerLine, "\"") || strings.Contains(lowerLine, "'") {
					// Has quotes, might be a value - redact it
					shouldRedact = true
					break
				}
			}
		}
		
		if shouldRedact {
			// Replace the value part with [REDACTED]
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				filtered = append(filtered, parts[0]+"= [REDACTED]")
			} else {
				filtered = append(filtered, line+" // [REDACTED: potential env var]")
			}
		} else {
			filtered = append(filtered, line)
		}
	}
	
	return strings.Join(filtered, "\n")
}

// searchCodebase performs an intelligent search through the codebase
func (e *Executor) searchCodebase(ctx context.Context, query string, maxResults int) *ToolResult {
	// Get workspace root - try multiple possible locations
	var workspaceRoot string
	possibleRoots := []string{
		"backend",           // If running from workspace root
		".",                 // If running from backend directory
		"..",                // If running from backend/cmd/bot
		"../..",             // If running from backend/cmd/bot (deeper)
	}
	
	for _, root := range possibleRoots {
		// Check if this looks like the backend directory (has internal/ and pkg/)
		testPath := filepath.Join(root, "internal")
		if info, err := os.Stat(testPath); err == nil && info.IsDir() {
			workspaceRoot = root
			break
		}
	}
	
	// Default to "backend" if nothing found
	if workspaceRoot == "" {
		workspaceRoot = "backend"
	}
	
	// Build list of relevant files
	var relevantFiles []string
	err := filepath.Walk(workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		
		// Skip directories
		if info.IsDir() {
			// Skip certain directories
			dirName := strings.ToLower(info.Name())
			if dirName == "node_modules" || dirName == ".git" || dirName == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		
		// Only process Go files, markdown, and config files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".go" && ext != ".md" && ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}
		
		// Check if file should be excluded
		if shouldExcludeFile(path) {
			return nil
		}
		
		relevantFiles = append(relevantFiles, path)
		return nil
	})
	
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to walk codebase: %v", err),
		}
	}
	
	// Search through files
	type match struct {
		file    string
		content string
		score   int
	}
	var matches []match
	
	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower)
	
	for _, file := range relevantFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		
		contentStr := string(content)
		contentLower := strings.ToLower(contentStr)
		
		// Calculate relevance score
		score := 0
		
		// Exact phrase match
		if strings.Contains(contentLower, queryLower) {
			score += 100
		}
		
		// Word matches
		for _, word := range queryWords {
			if len(word) < 3 {
				continue
			}
			count := strings.Count(contentLower, word)
			score += count * 10
		}
		
		// File path match
		if strings.Contains(strings.ToLower(file), queryLower) {
			score += 50
		}
		
		// Function/type name matches (common Go patterns)
		if strings.Contains(contentStr, "func "+query) || 
		   strings.Contains(contentStr, "type "+query) ||
		   strings.Contains(contentStr, "const "+query) ||
		   strings.Contains(contentStr, "var "+query) {
			score += 30
		}
		
		if score > 0 {
			// Extract relevant snippet (around matches)
			snippet := extractRelevantSnippet(contentStr, queryLower, 20)
			matches = append(matches, match{
				file:    file,
				content: filterEnvVars(snippet),
				score:   score,
			})
		}
	}
	
	// Sort by score (descending)
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[i].score < matches[j].score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
	
	// Limit results
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}
	
	// Format results
	var results []map[string]interface{}
	for _, m := range matches {
		results = append(results, map[string]interface{}{
			"file":    m.file,
			"snippet": m.content,
			"score":   m.score,
		})
	}
	
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"query":   query,
				"results": []interface{}{},
			},
			Message: "No matching code found. Try a different query or specify a file_path.",
		}
	}
	
	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"query":   query,
			"results": results,
		},
		Message: fmt.Sprintf("Found %d matching code snippet(s)", len(results)),
	}
}

// extractRelevantSnippet extracts a relevant code snippet around matches
func extractRelevantSnippet(content, query string, contextLines int) string {
	lines := strings.Split(content, "\n")
	queryLower := strings.ToLower(query)
	
	// Find lines with matches
	var matchIndices []int
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), queryLower) {
			matchIndices = append(matchIndices, i)
		}
	}
	
	if len(matchIndices) == 0 {
		// No matches, return first part of file
		if len(lines) > contextLines*2 {
			return strings.Join(lines[:contextLines*2], "\n") + "\n..."
		}
		return content
	}
	
	// Get context around first match
	firstMatch := matchIndices[0]
	start := firstMatch - contextLines
	if start < 0 {
		start = 0
	}
	end := firstMatch + contextLines
	if end > len(lines) {
		end = len(lines)
	}
	
	snippet := strings.Join(lines[start:end], "\n")
	
	// If there are more matches, indicate that
	if len(matchIndices) > 1 {
		snippet += "\n... (more matches in file) ..."
	}
	
	return snippet
}

