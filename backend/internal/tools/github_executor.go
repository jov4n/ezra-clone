package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ============================================================================
// GitHub Tool Implementations
// ============================================================================

func (e *Executor) executeGitHubRepoInfo(ctx context.Context, args map[string]interface{}) *ToolResult {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)

	if owner == "" || repo == "" {
		return &ToolResult{Success: false, Error: "owner and repo are required"}
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	
	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "EzraBot/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("GitHub API error: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return &ToolResult{Success: false, Error: "Repository not found"}
	}

	body, _ := io.ReadAll(resp.Body)
	var repoInfo map[string]interface{}
	if err := json.Unmarshal(body, &repoInfo); err != nil {
		return &ToolResult{Success: false, Error: "Failed to parse response"}
	}

	// Extract relevant info
	result := map[string]interface{}{
		"name":          repoInfo["name"],
		"full_name":     repoInfo["full_name"],
		"description":   repoInfo["description"],
		"stars":         repoInfo["stargazers_count"],
		"forks":         repoInfo["forks_count"],
		"language":      repoInfo["language"],
		"open_issues":   repoInfo["open_issues_count"],
		"url":           repoInfo["html_url"],
		"default_branch": repoInfo["default_branch"],
		"created_at":    repoInfo["created_at"],
		"updated_at":    repoInfo["updated_at"],
		"topics":        repoInfo["topics"],
	}

	return &ToolResult{
		Success: true,
		Data:    result,
	}
}

func (e *Executor) executeGitHubSearch(ctx context.Context, args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}
	}

	searchType, _ := args["type"].(string)
	if searchType == "" {
		searchType = "repositories"
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	apiURL := fmt.Sprintf("https://api.github.com/search/%s?q=%s&per_page=%d",
		searchType, url.QueryEscape(query), limit)

	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "EzraBot/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("GitHub API error: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var searchResult map[string]interface{}
	if err := json.Unmarshal(body, &searchResult); err != nil {
		return &ToolResult{Success: false, Error: "Failed to parse response"}
	}

	return &ToolResult{
		Success: true,
		Data:    searchResult,
		Message: fmt.Sprintf("Found %v results", searchResult["total_count"]),
	}
}

func (e *Executor) executeGitHubListOrgRepos(ctx context.Context, args map[string]interface{}) *ToolResult {
	org, _ := args["org"].(string)
	if org == "" {
		return &ToolResult{Success: false, Error: "org is required"}
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	// GitHub API: list org repos sorted by most recently updated
	apiURL := fmt.Sprintf("https://api.github.com/orgs/%s/repos?sort=updated&direction=desc&per_page=%d",
		url.QueryEscape(org), limit)

	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "EzraBot/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("GitHub API error: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Organization '%s' not found", org)}
	}

	body, _ := io.ReadAll(resp.Body)
	var repos []map[string]interface{}
	if err := json.Unmarshal(body, &repos); err != nil {
		return &ToolResult{Success: false, Error: "Failed to parse response"}
	}

	if len(repos) == 0 {
		return &ToolResult{
			Success: true,
			Message: fmt.Sprintf("No public repositories found for organization '%s'", org),
		}
	}

	// Format the results nicely
	var results []map[string]interface{}
	for _, repo := range repos {
		results = append(results, map[string]interface{}{
			"name":        repo["name"],
			"full_name":   repo["full_name"],
			"description": repo["description"],
			"language":    repo["language"],
			"updated_at":  repo["updated_at"],
			"pushed_at":   repo["pushed_at"],
			"url":         repo["html_url"],
			"stars":       repo["stargazers_count"],
		})
	}

	// Get the most recently updated repo for a nice summary
	mostRecent := results[0]
	
	return &ToolResult{
		Success: true,
		Data:    results,
		Message: fmt.Sprintf("Found %d repos. Most recently updated: %s (updated: %v)", 
			len(results), mostRecent["name"], mostRecent["updated_at"]),
	}
}

func (e *Executor) executeGitHubReadFile(ctx context.Context, args map[string]interface{}) *ToolResult {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	path, _ := args["path"].(string)
	branch, _ := args["branch"].(string)

	if owner == "" || repo == "" || path == "" {
		return &ToolResult{Success: false, Error: "owner, repo, and path are required"}
	}

	if branch == "" {
		branch = "main"
	}

	// Use raw content URL
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		owner, repo, branch, path)

	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	req.Header.Set("User-Agent", "EzraBot/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to fetch file: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// Try with 'master' branch
		if branch == "main" {
			args["branch"] = "master"
			return e.executeGitHubReadFile(ctx, args)
		}
		return &ToolResult{Success: false, Error: "File not found"}
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100000)) // 100KB limit
	content := string(body)

	// Truncate if too long
	if len(content) > 10000 {
		content = content[:10000] + "\n... (truncated)"
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":    path,
			"content": content,
		},
	}
}

