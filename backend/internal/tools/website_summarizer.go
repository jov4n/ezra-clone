package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"ezra-clone/backend/internal/adapter"
	"go.uber.org/zap"
)

// executeSummarizeWebsite summarizes a website by fetching it and using OpenRouter to generate a summary
func (e *Executor) executeSummarizeWebsite(ctx context.Context, args map[string]interface{}) *ToolResult {
	urlStr, _ := args["url"].(string)
	if urlStr == "" {
		return &ToolResult{Success: false, Error: "url is required"}
	}

	// Check if LLM adapter is available
	if e.llmAdapter == nil {
		return &ToolResult{
			Success: false,
			Error:   "LLM adapter not configured. Cannot generate summary.",
		}
	}

	e.logger.Info("Summarizing website",
		zap.String("url", urlStr),
	)

	// First, fetch the webpage content using existing fetch_webpage logic
	fetchResult := e.executeFetchWebpage(ctx, args)
	if !fetchResult.Success {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to fetch webpage: %s", fetchResult.Error),
		}
	}

	// Extract content from the fetch result
	var content string
	var title string

	if data, ok := fetchResult.Data.(map[string]interface{}); ok {
		// Try to get full_text first, then content, then fallback
		if fullText, ok := data["full_text"].(string); ok && fullText != "" {
			content = fullText
		} else if contentStr, ok := data["content"].(string); ok && contentStr != "" {
			content = contentStr
		} else {
			return &ToolResult{
				Success: false,
				Error:   "No content extracted from webpage",
			}
		}

		// Get title if available
		if titleStr, ok := data["title"].(string); ok {
			title = titleStr
		}
	} else {
		return &ToolResult{
			Success: false,
			Error:   "Unexpected response format from webpage fetch",
		}
	}

	if content == "" {
		return &ToolResult{
			Success: false,
			Error:   "No content extracted from webpage",
		}
	}

	// Use smart multi-stage summarization for long content
	var summary string
	var err error
	
	if len(content) > 8000 {
		// Use multi-stage summarization: chunk → summarize chunks → final summary
		e.logger.Info("Using multi-stage summarization for long content",
			zap.Int("content_length", len(content)),
			zap.String("url", urlStr),
		)
		summary, err = e.generateMultiStageSummary(ctx, content, title)
	} else {
		// Use simple summarization for shorter content
		e.logger.Info("Using simple summarization for short content",
			zap.Int("content_length", len(content)),
			zap.String("url", urlStr),
		)
		summary, err = e.callLLMForSummary(ctx, content)
	}
	
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to generate summary: %v", err),
		}
	}

	// Build response
	responseData := map[string]interface{}{
		"url":     urlStr,
		"summary": summary,
	}

	if title != "" {
		responseData["title"] = title
	}

	return &ToolResult{
		Success: true,
		Data:    responseData,
		Message: fmt.Sprintf("Generated summary for %s", urlStr),
	}
}

// callLLMForSummary calls LLM adapter (via LiteLLM) to generate a simple summary (for short content)
func (e *Executor) callLLMForSummary(ctx context.Context, text string) (string, error) {
	systemPrompt := "Provide a concise one-paragraph summary focusing on main purpose and key offerings."
	userPrompt := fmt.Sprintf("Summarize this website content:\n\n%s", text)

	response, err := e.llmAdapter.Generate(ctx, systemPrompt, userPrompt, []adapter.Tool{})
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	if response.Content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	return strings.TrimSpace(response.Content), nil
}

// generateMultiStageSummary performs multi-stage summarization:
// 1. Smart chunk the content
// 2. Summarize each chunk (extract important info)
// 3. Combine chunk summaries into final summary
func (e *Executor) generateMultiStageSummary(ctx context.Context, content, title string) (string, error) {
	e.logger.Info("Starting multi-stage summarization",
		zap.Int("content_length", len(content)),
		zap.String("title", title),
	)

	// Step 1: Smart chunk the content
	// Max chunk size for input to LLM for chunk summarization
	// Aim for ~3000 tokens, which is roughly 12000 characters (4 chars/token)
	// OpenRouter models typically have 8k-128k context windows, so 3k tokens is safe for a chunk.
	const maxChunkCharSize = 12000
	chunks := smartChunkContent(content, maxChunkCharSize)
	
	e.logger.Info("Content chunked for multi-stage summarization",
		zap.Int("num_chunks", len(chunks)),
		zap.Int("max_chunk_size", maxChunkCharSize),
	)

	var chunkSummaries []string
	for i, chunk := range chunks {
		e.logger.Info("Summarizing chunk", zap.Int("current_chunk", i+1), zap.Int("total_chunks", len(chunks)))
		chunkSummary, err := e.summarizeChunk(ctx, chunk, i+1, len(chunks))
		if err != nil {
			e.logger.Warn("Failed to summarize chunk, using chunk content as fallback",
				zap.Int("chunk_index", i),
				zap.Error(err),
			)
			// Fallback: if summarization fails, use the chunk content itself (truncated)
			if len(chunk) > 1000 { // Truncate chunk content if too long for summary
				chunkSummary = chunk[:1000] + "... (original chunk content)"
			} else {
				chunkSummary = chunk + " (original chunk content)"
			}
		}
		chunkSummaries = append(chunkSummaries, chunkSummary)
	}

	// Step 2: Combine chunk summaries and generate final summary
	combinedSummaries := strings.Join(chunkSummaries, "\n\n---\n\n")
	e.logger.Info("Combining chunk summaries into final summary",
		zap.Int("combined_length", len(combinedSummaries)),
	)

	// Truncate combined summaries if they exceed the LLM's input limit for the final summary
	// Assuming a max input of ~6000 tokens for the final summary prompt, which is ~24000 characters
	const maxFinalSummaryInputChars = 24000
	if len(combinedSummaries) > maxFinalSummaryInputChars {
		combinedSummaries = combinedSummaries[:maxFinalSummaryInputChars] + "\n\n... [intermediate summaries truncated]"
		e.logger.Warn("Combined chunk summaries truncated for final summarization input",
			zap.Int("truncated_length", len(combinedSummaries)),
		)
	}

	finalSummary, err := e.combineChunkSummaries(ctx, chunkSummaries, title)
	if err != nil {
		return "", fmt.Errorf("failed to generate final summary from chunks: %w", err)
	}

	e.logger.Info("Multi-stage summarization completed successfully",
		zap.Int("final_summary_length", len(finalSummary)),
	)
	return finalSummary, nil
}

// summarizeChunk summarizes a single chunk, extracting only important/vital information
func (e *Executor) summarizeChunk(ctx context.Context, chunk string, chunkNum, totalChunks int) (string, error) {
	systemPrompt := "Extract and summarize ONLY the most important and vital information from this content chunk. Focus on key facts, main points, significant insights, and essential details. Omit filler, repetition, and less critical information. Keep it concise but comprehensive."
	userPrompt := fmt.Sprintf("Content chunk %d of %d:\n\n%s\n\nExtract and summarize the most important information from this chunk.", chunkNum, totalChunks, chunk)

	response, err := e.llmAdapter.Generate(ctx, systemPrompt, userPrompt, []adapter.Tool{})
	if err != nil {
		return "", fmt.Errorf("failed to summarize chunk: %w", err)
	}

	if response.Content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	return strings.TrimSpace(response.Content), nil
}

// combineChunkSummaries combines multiple chunk summaries into a final comprehensive summary
func (e *Executor) combineChunkSummaries(ctx context.Context, chunkSummaries []string, title string) (string, error) {
	// Combine all chunk summaries
	combinedSummaries := strings.Join(chunkSummaries, "\n\n---\n\n")
	
	systemPrompt := "Create a comprehensive, well-structured summary that synthesizes information from multiple content chunks. Focus on the main purpose, key offerings, important insights, and essential details. Write a cohesive one-paragraph summary that captures the essence of the entire content."
	
	userPrompt := fmt.Sprintf("Title: %s\n\nSummaries from content chunks:\n\n%s\n\nCreate a comprehensive final summary that synthesizes all the important information above.", title, combinedSummaries)

	response, err := e.llmAdapter.Generate(ctx, systemPrompt, userPrompt, []adapter.Tool{})
	if err != nil {
		return "", fmt.Errorf("failed to combine summaries: %w", err)
	}

	if response.Content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	return strings.TrimSpace(response.Content), nil
}

// smartChunkContent intelligently splits content into chunks at natural boundaries
// It tries to split at paragraph breaks first, then sentence breaks, avoiding mid-word splits
func smartChunkContent(content string, maxChunkSize int) []string {
	if len(content) <= maxChunkSize {
		return []string{content}
	}

	var chunks []string
	remaining := content

	for len(remaining) > maxChunkSize {
		// Try to find a good split point
		chunk := remaining[:maxChunkSize]
		
		// First, try to split at a paragraph break (double newline)
		if idx := strings.LastIndex(chunk, "\n\n"); idx > maxChunkSize*3/4 {
			chunks = append(chunks, strings.TrimSpace(remaining[:idx+2]))
			remaining = strings.TrimSpace(remaining[idx+2:])
			continue
		}
		
		// Then try to split at a single newline (paragraph end)
		if idx := strings.LastIndex(chunk, "\n"); idx > maxChunkSize*3/4 {
			chunks = append(chunks, strings.TrimSpace(remaining[:idx+1]))
			remaining = strings.TrimSpace(remaining[idx+1:])
			continue
		}
		
		// Try to split at sentence boundaries (period, exclamation, question mark followed by space)
		sentenceEnd := regexp.MustCompile(`[.!?]\s+`)
		matches := sentenceEnd.FindAllStringIndex(chunk, -1)
		if len(matches) > 0 {
			// Use the last sentence boundary that's in the last quarter of the chunk
			for i := len(matches) - 1; i >= 0; i-- {
				idx := matches[i][1]
				if idx > maxChunkSize*3/4 {
					chunks = append(chunks, strings.TrimSpace(remaining[:idx]))
					remaining = strings.TrimSpace(remaining[idx:])
					goto nextChunk
				}
			}
		}
		
		// Last resort: split at word boundary (space)
		if idx := strings.LastIndex(chunk, " "); idx > maxChunkSize*2/3 {
			chunks = append(chunks, strings.TrimSpace(remaining[:idx]))
			remaining = strings.TrimSpace(remaining[idx:])
		} else {
			// No good split point, force split (shouldn't happen often)
			chunks = append(chunks, strings.TrimSpace(remaining[:maxChunkSize]))
			remaining = strings.TrimSpace(remaining[maxChunkSize:])
		}
		
	nextChunk:
		continue
	}

	if len(remaining) > 0 {
		chunks = append(chunks, strings.TrimSpace(remaining))
	}

	return chunks
}

