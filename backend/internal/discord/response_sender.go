package discord

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"ezra-clone/backend/internal/agent"
	"ezra-clone/backend/internal/constants"
	"go.uber.org/zap"
)

// sendResponse sends the agent's response to Discord
func (h *Handler) sendResponse(s *discordgo.Session, channelID string, result *agent.TurnResult) {
	// Prepare message content (don't truncate here - let sendLongMessage handle chunking)
	// Apply smart Discord markdown formatting
	messageContent := SmartFormat(result.Content)

	// Convert agent embeds to Discord embeds
	var discordEmbeds []*discordgo.MessageEmbed
	for _, e := range result.Embeds {
		embed := &discordgo.MessageEmbed{
			Title:       e.Title,
			Description: e.Description,
			URL:         e.URL,
			Color:       e.Color,
		}

		// Add fields if present
		for _, f := range e.Fields {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   f.Name,
				Value:  f.Value,
				Inline: f.Inline,
			})
		}

		// Add footer if present
		if e.Footer != "" {
			embed.Footer = &discordgo.MessageEmbedFooter{
				Text: e.Footer,
			}
		}

		discordEmbeds = append(discordEmbeds, embed)
	}

	// Prepare file attachment if image data is present
	var files []*discordgo.File
	var imageEmbed *discordgo.MessageEmbed
	if len(result.ImageData) > 0 {
		imageName := result.ImageName
		if imageName == "" {
			imageName = "image.png"
		}

		// Create a file attachment
		files = append(files, &discordgo.File{
			Name:   imageName,
			Reader: bytes.NewReader(result.ImageData),
		})

		// Create a nice embed for the image
		imageEmbed = &discordgo.MessageEmbed{
			Title:       "🎨 Generated Image",
			Description: messageContent,
			Color:       0x5865F2, // Discord blurple color
			Image: &discordgo.MessageEmbedImage{
				URL: fmt.Sprintf("attachment://%s", imageName), // Reference the attached file
			},
			Footer: &discordgo.MessageEmbedFooter{
				Text: "ComfyUI via RunPod",
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}

		// Add metadata fields if available
		if result.ImageMeta != nil {
			var fields []*discordgo.MessageEmbedField

			if width, ok := result.ImageMeta["width"]; ok {
				if height, ok := result.ImageMeta["height"]; ok {
					fields = append(fields, &discordgo.MessageEmbedField{
						Name:   "Dimensions",
						Value:  fmt.Sprintf("%v × %v", width, height),
						Inline: true,
					})
				}
			}

			if seed, ok := result.ImageMeta["seed"]; ok {
				fields = append(fields, &discordgo.MessageEmbedField{
					Name:   "Seed",
					Value:  fmt.Sprintf("%v", seed),
					Inline: true,
				})
			}

			if elapsed, ok := result.ImageMeta["elapsed_seconds"]; ok {
				if elapsedFloat, ok := elapsed.(float64); ok {
					fields = append(fields, &discordgo.MessageEmbedField{
						Name:   "Generation Time",
						Value:  fmt.Sprintf("%.1fs", elapsedFloat),
						Inline: true,
					})
				}
			}

			if len(fields) > 0 {
				imageEmbed.Fields = fields
			}
		}

		// Add the image embed to the embeds list
		discordEmbeds = append(discordEmbeds, imageEmbed)

		h.logger.Debug("Attaching image to Discord message",
			zap.String("filename", imageName),
			zap.Int("size_bytes", len(result.ImageData)),
		)
	}

	// Send message with embeds and/or file attachment
	if len(discordEmbeds) > 0 || len(files) > 0 {
		// If we have an image embed, don't send content separately (it's in the embed)
		sendContent := messageContent
		if imageEmbed != nil {
			sendContent = "" // Image embed already has the description
		}

		// If sendContent is too long, we need to chunk it even with embeds
		if sendContent != "" && len(sendContent) > constants.DiscordMaxMessageLength {
			// Send embeds first, then chunk the content
			if len(discordEmbeds) > 0 || len(files) > 0 {
				_, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
					Content: "", // Send embeds/files first
					Embeds:  discordEmbeds,
					Files:   files,
				})
				if err != nil {
					h.logger.Error("Failed to send message with embeds/files",
						zap.Error(err),
						zap.String("channel_id", channelID),
					)
				}
			}
			// Now send the content in chunks
			h.sendLongMessage(s, channelID, sendContent)
		} else {
			// Content fits, send everything together
			_, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content: sendContent,
				Embeds:  discordEmbeds,
				Files:   files,
			})
			if err != nil {
				h.logger.Error("Failed to send message with embeds/files",
					zap.Error(err),
					zap.String("channel_id", channelID),
				)
			}
		}
	} else if messageContent != "" {
		// Plain text message - split if too long
		h.sendLongMessage(s, channelID, messageContent)
	}
}

// sendLongMessage splits a message into chunks if it exceeds Discord's character limit
func (h *Handler) sendLongMessage(s *discordgo.Session, channelID, content string) {
	maxLength := constants.DiscordMaxMessageLength

	if len(content) <= maxLength {
		// Message fits in one chunk
		_, err := s.ChannelMessageSend(channelID, content)
		if err != nil {
			h.logger.Error("Failed to send message",
				zap.Error(err),
				zap.String("channel_id", channelID),
			)
		}
		return
	}

	// Split into chunks (reserve space for part indicator)
	// Part indicator format: "*(Part X/Y)*" is about 15 chars, so reserve 20 for safety
	const partIndicatorReserve = 20
	maxChunkLength := maxLength - partIndicatorReserve

	chunks := splitMessage(content, maxChunkLength)

	for i, chunk := range chunks {
		var message string
		if len(chunks) > 1 {
			// Add chunk indicator for multi-part messages
			partIndicator := fmt.Sprintf("*(Part %d/%d)*", i+1, len(chunks))
			message = chunk + "\n" + partIndicator
		} else {
			message = chunk
		}

		// Final safety check - ensure we don't exceed limit
		if len(message) > maxLength {
			// This shouldn't happen if splitMessage works correctly, but be safe
			message = message[:maxLength-3] + "..."
			h.logger.Warn("Chunk still too long after splitting, truncating",
				zap.Int("chunk", i+1),
				zap.Int("length", len(message)),
			)
		}

		_, err := s.ChannelMessageSend(channelID, message)
		if err != nil {
			h.logger.Error("Failed to send message chunk",
				zap.Error(err),
				zap.String("channel_id", channelID),
				zap.Int("chunk", i+1),
				zap.Int("total_chunks", len(chunks)),
			)
			// Stop sending if we hit an error
			break
		}

		// Small delay between chunks to avoid rate limiting (only if not last chunk)
		if i < len(chunks)-1 {
			// Brief pause between messages
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// splitMessage splits a message into chunks of maxLength, ensuring code blocks are not broken
func splitMessage(content string, maxLength int) []string {
	if len(content) <= maxLength {
		return []string{content}
	}

	var chunks []string
	current := ""
	lines := strings.Split(content, "\n")

	// Track if we're inside a code block
	inCodeBlock := false
	codeBlockContent := ""

	for i, line := range lines {
		// Check if this line starts or ends a code block
		// Code blocks can be: ```, ```go, ```python, etc.
		trimmedLine := strings.TrimSpace(line)
		isCodeBlockMarker := strings.HasPrefix(trimmedLine, "```")

		if isCodeBlockMarker {
			if inCodeBlock {
				// Ending a code block - add the closing marker
				codeBlockContent += line
				if i < len(lines)-1 {
					codeBlockContent += "\n"
				}

				// Now we have a complete code block
				// If the code block itself is too large, we need to split it while preserving markers
				if len(codeBlockContent) > maxLength {
					// Code block is too large - split it while preserving structure
					// Extract the opening marker (e.g., "```go" or "```")
					codeBlockLines := strings.Split(codeBlockContent, "\n")
					if len(codeBlockLines) > 0 {
						openingMarker := codeBlockLines[0] // e.g., "```go" or "```"

						// Get the content between markers (skip first and last lines)
						var contentLines []string
						if len(codeBlockLines) > 2 {
							contentLines = codeBlockLines[1 : len(codeBlockLines)-1]
						}
						content := strings.Join(contentLines, "\n")

						// Save current chunk if any
						if current != "" {
							chunks = append(chunks, current)
							current = ""
						}

						// Split the content while preserving code block structure
						// Calculate available space (opening marker + closing marker + newlines)
						markerOverhead := len(openingMarker) + len("```") + 4 // +4 for newlines

						for len(content) > 0 {
							availableSpace := maxLength - markerOverhead
							if availableSpace < 100 {
								// If overhead is too large, just use a smaller chunk
								availableSpace = maxLength / 2
							}

							if len(content) <= availableSpace {
								// Remaining content fits in one chunk
								chunk := openingMarker + "\n" + content + "\n```"
								if i < len(lines)-1 {
									chunk += "\n"
								}
								if current != "" {
									current += "\n" + chunk
								} else {
									current = chunk
								}
								content = ""
							} else {
								// Need to split content
								// Try to split at a newline
								splitIdx := strings.LastIndex(content[:availableSpace], "\n")
								if splitIdx < availableSpace/2 {
									// No good newline found, split at availableSpace
									splitIdx = availableSpace
								}

								chunkContent := content[:splitIdx]
								chunk := openingMarker + "\n" + chunkContent + "\n```"
								chunks = append(chunks, chunk)

								// Remove the split content
								if splitIdx < len(content) {
									content = content[splitIdx:]
									if strings.HasPrefix(content, "\n") {
										content = content[1:]
									}
								} else {
									content = ""
								}
							}
						}
					} else {
						// Fallback: just add the code block as-is (will be truncated by Discord)
						if current != "" {
							chunks = append(chunks, current)
						}
						current = codeBlockContent
					}
				} else {
					// Code block fits - try to add it to current chunk
					if current != "" {
						combinedLength := len(current) + 1 + len(codeBlockContent)
						if combinedLength <= maxLength {
							current += "\n" + codeBlockContent
						} else {
							chunks = append(chunks, current)
							current = codeBlockContent
						}
					} else {
						current = codeBlockContent
					}
				}

				codeBlockContent = ""
				inCodeBlock = false
				continue
			} else {
				// Starting a new code block
				// Save current chunk if any (before starting code block)
				if current != "" {
					chunks = append(chunks, current)
					current = ""
				}
				inCodeBlock = true
				codeBlockContent = line
				if i < len(lines)-1 {
					codeBlockContent += "\n"
				}
				continue
			}
		}

		if inCodeBlock {
			// We're inside a code block - accumulate it
			// We need to keep the entire code block together, so we just accumulate
			// The splitting will happen when we close the code block
			codeBlockContent += line
			if i < len(lines)-1 {
				codeBlockContent += "\n"
			}
			continue
		}

		// Regular line processing (not in code block)
		// Check for inline code (backticks) - try to keep inline code on same line
		lineWithNewline := line
		if i < len(lines)-1 {
			lineWithNewline += "\n"
		}

		// If adding this line would exceed the limit, start a new chunk
		if len(current) > 0 && len(current)+len(lineWithNewline) > maxLength {
			chunks = append(chunks, current)
			current = ""
		}

		// If a single line is too long, split it (but try to preserve inline code)
		if len(line) > maxLength {
			// Save current chunk if any
			if current != "" {
				chunks = append(chunks, current)
				current = ""
			}

			// Try to split at word boundaries or code block boundaries
			remainingLine := line
			for len(remainingLine) > maxLength {
				// Look for a good split point
				splitIdx := maxLength

				// Check if we're in the middle of inline code (backticks)
				lastBacktick := strings.LastIndex(remainingLine[:maxLength], "`")
				if lastBacktick > maxLength/2 {
					// We might be in the middle of inline code, try to find the closing backtick
					nextBacktick := strings.Index(remainingLine[lastBacktick+1:], "`")
					if nextBacktick > 0 && nextBacktick < 100 {
						// Found closing backtick nearby, split after it
						splitIdx = lastBacktick + nextBacktick + 2
					}
				}

				// Try to split at word boundary
				if splitIdx == maxLength {
					spaceIdx := strings.LastIndex(remainingLine[:maxLength], " ")
					if spaceIdx > maxLength*3/4 {
						splitIdx = spaceIdx + 1
					}
				}

				chunks = append(chunks, remainingLine[:splitIdx])
				remainingLine = remainingLine[splitIdx:]
			}
			if remainingLine != "" {
				current = remainingLine
			}
		} else {
			if current != "" {
				current += "\n"
			}
			current += line
		}
	}

	// Handle any remaining code block content
	if inCodeBlock && codeBlockContent != "" {
		if current != "" {
			chunks = append(chunks, current)
			current = ""
		}
		// Add the incomplete code block (shouldn't happen in normal flow, but handle it)
		if len(codeBlockContent) <= maxLength {
			current = codeBlockContent
		} else {
			chunks = append(chunks, codeBlockContent[:maxLength])
			current = codeBlockContent[maxLength:]
		}
	}

	// Add remaining content
	if current != "" {
		chunks = append(chunks, current)
	}

	return chunks
}

