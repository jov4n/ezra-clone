package agent

import (
	"context"
	"fmt"
	"strings"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/tools"
	"go.uber.org/zap"
)

// ToolResultProcessor handles processing of tool execution results
type ToolResultProcessor struct {
	logger *zap.Logger
}

// NewToolResultProcessor creates a new tool result processor
func NewToolResultProcessor(logger *zap.Logger) *ToolResultProcessor {
	return &ToolResultProcessor{
		logger: logger,
	}
}

// ProcessToolResults processes tool execution results and extracts relevant data
// Returns: toolResults (for context), imageData, imageName, imageMeta, fetchedURLs, embeds
func (p *ToolResultProcessor) ProcessToolResults(
	ctx context.Context,
	toolCalls []adapter.ToolCall,
	execCtx *tools.ExecutionContext,
	executor *tools.Executor,
	llmResponse *adapter.Response,
	preservedImageData []byte,
	preservedImageName string,
	preservedImageMeta map[string]interface{},
	preservedFetchedURLs []string,
) (
	toolResults []string,
	imageData []byte,
	imageName string,
	imageMeta map[string]interface{},
	fetchedURLs []string,
	embeds []Embed,
	fetchWebpageCount int,
) {
	// Start with preserved values
	imageData = preservedImageData
	imageName = preservedImageName
	imageMeta = preservedImageMeta
	if imageMeta == nil {
		imageMeta = make(map[string]interface{})
	}
	fetchedURLs = preservedFetchedURLs
	if fetchedURLs == nil {
		fetchedURLs = make([]string, 0)
	}

	for _, toolCall := range toolCalls {
		// Track fetch_webpage calls
		if toolCall.Name == tools.ToolFetchWebpage {
			fetchWebpageCount++
		}

		result := executor.Execute(ctx, execCtx, toolCall)

		if result.Success {
			p.logger.Info("Tool executed successfully",
				zap.String("tool", toolCall.Name),
				zap.String("message", result.Message),
			)

			// Capture tool results for context
			// Track fetch_webpage URLs to prevent duplicates and include content
			if toolCall.Name == tools.ToolFetchWebpage && result.Data != nil {
				if webpageData, ok := result.Data.(map[string]interface{}); ok {
					url, _ := webpageData["url"].(string)
					content, _ := webpageData["content"].(string)

					if url != "" {
						fetchedURLs = append(fetchedURLs, url)

						// Include article content in tool results for summarization
						// Truncate to reasonable size (5000 chars per article) to avoid overwhelming the LLM
						if content != "" {
							// Truncate content to 5000 chars per article for summarization
							maxContentLength := 5000
							if len(content) > maxContentLength {
								// Try to truncate at a sentence boundary
								truncated := content[:maxContentLength]
								if lastPeriod := strings.LastIndex(truncated, "."); lastPeriod > maxContentLength*3/4 {
									content = truncated[:lastPeriod+1] + "... [content truncated for summarization]"
								} else {
									content = truncated + "... [content truncated for summarization]"
								}
							}
							toolResults = append(toolResults, fmt.Sprintf("[ARTICLE %d from %s]:\n%s", fetchWebpageCount, url, content))
						} else {
							// No content, just URL
							if result.Message != "" {
								toolResults = append(toolResults, fmt.Sprintf("[%s] Fetched: %s - %s", toolCall.Name, url, result.Message))
							} else {
								toolResults = append(toolResults, fmt.Sprintf("[%s] Fetched: %s", toolCall.Name, url))
							}
						}
					}
				}
			}

			// For web_search, include the actual search results data so LLM can see URLs
			if toolCall.Name == tools.ToolWebSearch && result.Data != nil {
				if searchData, ok := result.Data.(map[string]interface{}); ok {
					if resultsRaw, ok := searchData["results"]; ok {
						if results, ok := resultsRaw.([]tools.SearchResult); ok && len(results) > 0 {
							var resultLines []string
							resultLines = append(resultLines, fmt.Sprintf("[%s]: Found %d search results (ARTICLE URLs to fetch):", toolCall.Name, len(results)))
							for i, r := range results {
								if i >= 5 {
									break
								}
								// Make it very clear these are article URLs
								resultLines = append(resultLines, fmt.Sprintf("  ARTICLE %d: %s", i+1, r.Title))
								resultLines = append(resultLines, fmt.Sprintf("    URL: %s", r.URL))
								if r.Snippet != "" {
									// Include snippet but truncate if too long
									snippet := r.Snippet
									if len(snippet) > 200 {
										snippet = snippet[:197] + "..."
									}
									resultLines = append(resultLines, fmt.Sprintf("    Preview: %s", snippet))
								}
							}
							resultLines = append(resultLines, "IMPORTANT: These are ARTICLE URLs. Use fetch_webpage with these URLs to read the actual articles.")
							toolResults = append(toolResults, strings.Join(resultLines, "\n"))
						} else {
							// Fallback to message if format is unexpected
							if result.Message != "" {
								toolResults = append(toolResults, fmt.Sprintf("[%s]: %s", toolCall.Name, result.Message))
							}
						}
					} else {
						// Fallback to message if no results
						if result.Message != "" {
							toolResults = append(toolResults, fmt.Sprintf("[%s]: %s", toolCall.Name, result.Message))
						}
					}
				} else {
					// Fallback to message if data format is unexpected
					if result.Message != "" {
						toolResults = append(toolResults, fmt.Sprintf("[%s]: %s", toolCall.Name, result.Message))
					}
				}
			} else if toolCall.Name == tools.ToolSummarizeWebsite && result.Data != nil {
				// Extract and include the summary in tool results
				if summaryData, ok := result.Data.(map[string]interface{}); ok {
					url, _ := summaryData["url"].(string)
					summary, _ := summaryData["summary"].(string)
					title, _ := summaryData["title"].(string)

					if summary != "" {
						var summaryLines []string
						if title != "" {
							summaryLines = append(summaryLines, fmt.Sprintf("[SUMMARY of %s]:", url))
							summaryLines = append(summaryLines, fmt.Sprintf("Title: %s", title))
						} else {
							summaryLines = append(summaryLines, fmt.Sprintf("[SUMMARY of %s]:", url))
						}
						summaryLines = append(summaryLines, summary)
						toolResults = append(toolResults, strings.Join(summaryLines, "\n"))
					} else if result.Message != "" {
						toolResults = append(toolResults, fmt.Sprintf("[%s]: %s", toolCall.Name, result.Message))
					}
				} else if result.Message != "" {
					toolResults = append(toolResults, fmt.Sprintf("[%s]: %s", toolCall.Name, result.Message))
				}
			} else if toolCall.Name != tools.ToolFetchWebpage && toolCall.Name != tools.ToolSummarizeWebsite && result.Message != "" {
				// Don't add fetch_webpage or summarize_website results here - we handle them above
				toolResults = append(toolResults, fmt.Sprintf("[%s]: %s", toolCall.Name, result.Message))
			}

			// Check for image data from image generation tool
			if toolCall.Name == tools.ToolGenerateImageWithRunPod && result.Data != nil {
				if dataMap, ok := result.Data.(map[string]interface{}); ok {
					if imgData, ok := dataMap["image_data"].([]byte); ok && len(imgData) > 0 {
						imageData = imgData
						if format, ok := dataMap["image_format"].(string); ok {
							imageName = fmt.Sprintf("image.%s", format)
						} else {
							imageName = "image.png"
						}

						// Extract metadata for embed
						imageMeta = make(map[string]interface{})
						if seed, ok := dataMap["seed"]; ok {
							imageMeta["seed"] = seed
						}
						if width, ok := dataMap["width"]; ok {
							imageMeta["width"] = width
						}
						if height, ok := dataMap["height"]; ok {
							imageMeta["height"] = height
						}
						if workflow, ok := dataMap["workflow"]; ok {
							imageMeta["workflow"] = workflow
						}
						if elapsed, ok := dataMap["elapsed_seconds"]; ok {
							imageMeta["elapsed_seconds"] = elapsed
						}

						p.logger.Debug("Captured image data from tool result",
							zap.Int("image_size", len(imageData)),
							zap.String("image_name", imageName),
						)
					}
				}
			}

			// For informational tools, use result data to build response and embeds
			// BUT: Don't set content for web_search if we're in a multi-step operation
			// (let the LLM recurse to fetch/summarize articles)
			if isInformationalTool(toolCall.Name) && result.Data != nil {
				response, toolEmbeds := formatToolResponseWithEmbeds(toolCall.Name, result)
				// Only set content if it's not web_search (web_search should recurse to fetch articles)
				// OR if we already have content from LLM
				if response != "" {
					if toolCall.Name != tools.ToolWebSearch {
						// For non-web-search tools, set content normally
						if llmResponse.Content == "" {
							llmResponse.Content = response
						}
					} else {
						// For web_search, only set content if LLM already provided content
						// This allows recursion when user wants to fetch/summarize articles
						if llmResponse.Content != "" {
							// LLM provided content, so append search results
							llmResponse.Content += "\n\n" + response
						}
						// Otherwise, leave content empty to trigger recursion
					}
				}
				if len(toolEmbeds) > 0 {
					embeds = append(embeds, toolEmbeds...)
				}
			}

			// If send_message tool was used, capture the message as content
			if toolCall.Name == tools.ToolSendMessage && result.Message != "" {
				if llmResponse.Content == "" {
					llmResponse.Content = result.Message
				}
			}
		} else {
			p.logger.Warn("Tool execution failed",
				zap.String("tool", toolCall.Name),
				zap.String("error", result.Error),
			)
			toolResults = append(toolResults, fmt.Sprintf("[%s] ERROR: %s", toolCall.Name, result.Error))
		}
	}

	return toolResults, imageData, imageName, imageMeta, fetchedURLs, embeds, fetchWebpageCount
}

