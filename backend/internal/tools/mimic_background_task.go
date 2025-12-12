package tools

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/pkg/config"
	"go.uber.org/zap"
)

// MimicBackgroundTask manages automatic posting when mimic mode is active
type MimicBackgroundTask struct {
	executor      *Executor
	llm           *adapter.LLMAdapter
	discordSession *discordgo.Session
	config        *config.Config
	logger        *zap.Logger
	stopChan      chan struct{}
	running       bool
	agentID       string
}

// NewMimicBackgroundTask creates a new background task manager
func NewMimicBackgroundTask(executor *Executor, llm *adapter.LLMAdapter, discordSession *discordgo.Session, cfg *config.Config, logger *zap.Logger) *MimicBackgroundTask {
	return &MimicBackgroundTask{
		executor:      executor,
		llm:           llm,
		discordSession: discordSession,
		config:        cfg,
		logger:        logger,
		stopChan:      make(chan struct{}),
		running:       false,
	}
}

// Start begins the background task for an agent
func (m *MimicBackgroundTask) Start(agentID string) {
	if m.running {
		m.logger.Warn("Background task already running", zap.String("agent_id", agentID))
		return
	}

	m.agentID = agentID
	m.running = true
	m.stopChan = make(chan struct{})

	// Register message handler for responding to posts
	m.discordSession.AddHandler(m.handleChannelMessage)

	go m.runLoop(agentID)

	m.logger.Info("Mimic background task started",
		zap.String("agent_id", agentID),
	)
}

// Stop stops the background task
func (m *MimicBackgroundTask) Stop() {
	if !m.running {
		return
	}

	m.running = false
	close(m.stopChan)

	// Note: Discord handlers can't be easily removed, but we check m.running in handleChannelMessage
	// so it will effectively stop responding

	m.logger.Info("Mimic background task stopped",
		zap.String("agent_id", m.agentID),
	)
}

// handleChannelMessage handles messages in the mimic channel and randomly responds
func (m *MimicBackgroundTask) handleChannelMessage(s *discordgo.Session, msg *discordgo.MessageCreate) {
	// Only process messages in the configured mimic channel
	if msg.ChannelID != m.config.MimicChannelID {
		return
	}

	// Ignore if not running
	if !m.running {
		return
	}

	// Ignore messages from the bot itself
	if msg.Author.ID == s.State.User.ID {
		return
	}

	// Ignore bot messages
	if msg.Author.Bot {
		return
	}

	// Ignore empty messages
	if strings.TrimSpace(msg.Content) == "" {
		return
	}

	// Randomly decide whether to respond (30% chance)
	if rand.Float32() > 0.3 {
		return
	}

	// Get mimic state
	mimicState := m.executor.GetMimicState(m.agentID)
	if mimicState == nil || !mimicState.Active || mimicState.MimicProfile == nil {
		return
	}

	profile := mimicState.MimicProfile

	m.logger.Info("Mimic responding to message",
		zap.String("agent_id", m.agentID),
		zap.String("channel_id", msg.ChannelID),
		zap.String("responding_to_user", msg.Author.Username),
	)

	// Generate response in user's style
	ctx := context.Background()
	response, err := m.generateResponseToMessage(ctx, profile, msg.Content, msg.Author.Username)
	if err != nil {
		m.logger.Error("Failed to generate response",
			zap.Error(err),
		)
		return
	}

	// Post response
	_, err = s.ChannelMessageSend(msg.ChannelID, response)
	if err != nil {
		m.logger.Error("Failed to send response",
			zap.Error(err),
		)
		return
	}

	m.logger.Info("Mimic response sent",
		zap.String("agent_id", m.agentID),
		zap.String("channel_id", msg.ChannelID),
	)
}

// runLoop runs the main loop that posts at random intervals
func (m *MimicBackgroundTask) runLoop(agentID string) {
	for {
		// Random interval between 20 minutes and 1 hour
		interval := time.Duration(20+rand.Intn(40)) * time.Minute

		select {
		case <-m.stopChan:
			return
		case <-time.After(interval):
			// Check if still mimicking
			if !m.executor.IsMimicking(agentID) {
				m.logger.Debug("Mimic mode deactivated, stopping background task")
				m.running = false
				return
			}

			// Generate and post content
			if err := m.generateAndPost(agentID); err != nil {
				m.logger.Error("Failed to generate and post content",
					zap.String("agent_id", agentID),
					zap.Error(err),
				)
			}
		}
	}
}

// generateAndPost generates interesting content and posts it
func (m *MimicBackgroundTask) generateAndPost(agentID string) error {
	ctx := context.Background()

	// Get mimic state
	mimicState := m.executor.GetMimicState(agentID)
	if mimicState == nil || !mimicState.Active || mimicState.MimicProfile == nil {
		return fmt.Errorf("mimic state not active")
	}

	profile := mimicState.MimicProfile
	channelID := m.config.MimicChannelID
	if channelID == "" {
		return fmt.Errorf("mimic channel ID not configured")
	}

	m.logger.Info("Generating content for mimic post",
		zap.String("agent_id", agentID),
		zap.String("mimicking_user", profile.Username),
		zap.String("channel_id", channelID),
	)

	// Randomly decide whether to search the web (50% chance)
	shouldSearch := rand.Float32() < 0.5
	var postMessage string
	var err error

	if shouldSearch {
		// Generate a search query based on user's interests
		searchQuery, err := m.generateSearchQuery(ctx, profile)
		if err != nil {
			m.logger.Warn("Failed to generate search query, falling back to direct post",
				zap.Error(err),
			)
			shouldSearch = false
		} else {
			// Search the web
			searchResult := m.executor.executeWebSearch(ctx, map[string]interface{}{
				"query": searchQuery,
			})
			if !searchResult.Success {
				m.logger.Warn("Web search failed, falling back to direct post",
					zap.String("error", searchResult.Error),
				)
				shouldSearch = false
			} else {
				// Extract search results
				searchData, ok := searchResult.Data.(map[string]interface{})
				if !ok {
					m.logger.Warn("Invalid search result format, falling back to direct post")
					shouldSearch = false
				} else {
					resultsRaw, ok := searchData["results"]
					if !ok {
						m.logger.Warn("No results in search data, falling back to direct post")
						shouldSearch = false
					} else {
						// Parse results - could be []SearchResult or []interface{}
						var title, url, snippet string
						
						switch results := resultsRaw.(type) {
						case []SearchResult:
							if len(results) == 0 {
								m.logger.Warn("No search results found, falling back to direct post")
								shouldSearch = false
							} else {
								firstResult := results[0]
								title = firstResult.Title
								url = firstResult.URL
								snippet = firstResult.Snippet
							}
						case []interface{}:
							if len(results) == 0 {
								m.logger.Warn("No search results found, falling back to direct post")
								shouldSearch = false
							} else {
								firstResult, ok := results[0].(map[string]interface{})
								if !ok {
									m.logger.Warn("Invalid result format, falling back to direct post")
									shouldSearch = false
								} else {
									title, _ = firstResult["title"].(string)
									url, _ = firstResult["url"].(string)
									snippet, _ = firstResult["snippet"].(string)
								}
							}
						default:
							m.logger.Warn("Unexpected results type, falling back to direct post")
							shouldSearch = false
						}

						if shouldSearch && (title == "" || url == "") {
							m.logger.Warn("Invalid search result: missing title or URL, falling back to direct post")
							shouldSearch = false
						}

						if shouldSearch {
							// Generate a post message in the user's style with the article
							postMessage, err = m.generatePostMessage(ctx, profile, title, url, snippet)
							if err != nil {
								m.logger.Warn("Failed to generate post message from search, falling back to direct post",
									zap.Error(err),
								)
								shouldSearch = false
							}
						}
					}
				}
			}
		}
	}

	// If we didn't search or search failed, generate a direct post
	if !shouldSearch {
		postMessage, err = m.generateDirectPost(ctx, profile)
		if err != nil {
			return fmt.Errorf("failed to generate direct post: %w", err)
		}
	}

	// Post to Discord
	_, err = m.discordSession.ChannelMessageSend(channelID, postMessage)
	if err != nil {
		return fmt.Errorf("failed to post message: %w", err)
	}

	m.logger.Info("Mimic post sent successfully",
		zap.String("agent_id", agentID),
		zap.String("channel_id", channelID),
		zap.Bool("used_web_search", shouldSearch),
	)

	return nil
}

// generateSearchQuery uses LLM to generate a search query based on user's interests
func (m *MimicBackgroundTask) generateSearchQuery(ctx context.Context, profile *PersonalityProfile) (string, error) {
	// Build prompt based on user's interests from profile
	prompt := fmt.Sprintf(`Based on this user's communication style and interests, generate a search query for something they would find fascinating or want to share.

User profile:
- Common words/phrases: %s
- Tone: %s
- Interests (from messages): %s

Generate a single, specific search query (2-5 words) for something interesting, recent, or relevant that this person would want to share. Examples: "AI breakthroughs 2024", "new game releases", "tech news", "programming tips", "music releases".

Search query:`, 
		joinStrings(profile.CommonWords, ", "),
		joinStrings(profile.ToneIndicators, ", "),
		joinStrings(profile.CommonPhrases, ", "),
	)

	// Use LLM to generate query
	systemPrompt := "You are a helpful assistant that generates search queries. Respond with only the search query, nothing else."
	response, err := m.llm.Generate(ctx, systemPrompt, prompt, []adapter.Tool{})
	if err != nil {
		return "", err
	}

	query := response.Content
	if query == "" {
		// Fallback to a generic query based on common words
		if len(profile.CommonWords) > 0 {
			query = profile.CommonWords[0] + " news"
		} else {
			query = "interesting news"
		}
	}

	// Clean up the query
	query = cleanQuery(query)

	return query, nil
}

// generatePostMessage generates a post in the user's style
func (m *MimicBackgroundTask) generatePostMessage(ctx context.Context, profile *PersonalityProfile, title, url, snippet string) (string, error) {
	prompt := fmt.Sprintf(`You are posting as %s. Write a short Discord message (1-2 sentences max) sharing this article in their style.

Article:
Title: %s
URL: %s
Snippet: %s

Style guidelines:
- Capitalization: %s
- Punctuation: %s
- Tone: %s
- Common phrases: %s
- Emoji usage: %s

Write a casual, engaging message that they would write. Include the URL. Keep it authentic to their style.`,
		profile.Username,
		title,
		url,
		snippet,
		profile.Capitalization,
		profile.PunctuationStyle,
		joinStrings(profile.ToneIndicators, ", "),
		joinStrings(profile.CommonPhrases, ", "),
		joinStrings(profile.EmojiUsage, " "),
	)

	// Use the style prompt as system prompt and the post request as user message
	response, err := m.llm.Generate(ctx, profile.StylePrompt, prompt, []adapter.Tool{})
	if err != nil {
		return "", err
	}

	message := response.Content
	if message == "" {
		// Fallback message
		message = fmt.Sprintf("%s\n%s", title, url)
	}

	return message, nil
}

// generateDirectPost generates a post without web search - just something the user would naturally post
func (m *MimicBackgroundTask) generateDirectPost(ctx context.Context, profile *PersonalityProfile) (string, error) {
	prompt := fmt.Sprintf(`You are %s. Write a short Discord message (1-2 sentences max) that they would naturally post. This could be:
- A thought or observation
- A reaction to something
- A casual comment
- Something they find interesting or want to share
- A random thought

Style guidelines:
- Capitalization: %s
- Punctuation: %s
- Tone: %s
- Common phrases: %s
- Emoji usage: %s

Write something authentic to their style. Don't reference external links or articles - just a natural post they would make.`,
		profile.Username,
		profile.Capitalization,
		profile.PunctuationStyle,
		joinStrings(profile.ToneIndicators, ", "),
		joinStrings(profile.CommonPhrases, ", "),
		joinStrings(profile.EmojiUsage, " "),
	)

	// Use the style prompt as system prompt and the post request as user message
	response, err := m.llm.Generate(ctx, profile.StylePrompt, prompt, []adapter.Tool{})
	if err != nil {
		return "", err
	}

	message := response.Content
	if message == "" {
		// Fallback message based on common phrases
		if len(profile.CommonPhrases) > 0 {
			message = profile.CommonPhrases[0] + "..."
		} else if len(profile.CommonWords) > 0 {
			message = profile.CommonWords[0] + " is interesting"
		} else {
			message = "hmm"
		}
	}

	return message, nil
}

// generateResponseToMessage generates a response to a message in the user's style
func (m *MimicBackgroundTask) generateResponseToMessage(ctx context.Context, profile *PersonalityProfile, originalMessage, authorUsername string) (string, error) {
	prompt := fmt.Sprintf(`You are %s. Someone posted this message in the channel:

"%s" (by %s)

Write a short response (1-2 sentences max) that they would naturally post. This could be:
- A reaction or comment
- Agreement or disagreement
- A question
- A related thought
- A casual response

Style guidelines:
- Capitalization: %s
- Punctuation: %s
- Tone: %s
- Common phrases: %s
- Emoji usage: %s

Write something authentic to their style. Keep it natural and conversational.`,
		profile.Username,
		originalMessage,
		authorUsername,
		profile.Capitalization,
		profile.PunctuationStyle,
		joinStrings(profile.ToneIndicators, ", "),
		joinStrings(profile.CommonPhrases, ", "),
		joinStrings(profile.EmojiUsage, " "),
	)

	// Use the style prompt as system prompt and the response request as user message
	response, err := m.llm.Generate(ctx, profile.StylePrompt, prompt, []adapter.Tool{})
	if err != nil {
		return "", err
	}

	message := response.Content
	if message == "" {
		// Fallback response based on common phrases
		if len(profile.CommonPhrases) > 0 {
			message = profile.CommonPhrases[0]
		} else {
			message = "interesting"
		}
	}

	return message, nil
}

// Helper functions
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return "none"
	}
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func cleanQuery(query string) string {
	// Remove quotes, newlines, and extra spaces
	query = strings.TrimSpace(query)
	query = strings.Trim(query, `"'`)
	query = strings.ReplaceAll(query, "\n", " ")
	query = strings.ReplaceAll(query, "\r", " ")
	
	// Remove extra spaces
	words := strings.Fields(query)
	return strings.Join(words, " ")
}

