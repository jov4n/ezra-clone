package tools

import (
	"context"
	"fmt"
	"strings"
)

// generateStylePromptWithRAG generates style prompt with RAG memory context
func (d *DiscordExecutor) generateStylePromptWithRAG(ctx context.Context, profile *PersonalityProfile, userID string) string {
	basePrompt := generateStylePrompt(profile)

	// If repository available, retrieve relevant memories
	if d.repo != nil {
		// Use a general query to get user's preferences/opinions
		memories, err := d.repo.RetrieveUserPersonalityMemories(ctx, userID, "preference opinion fact", 5)
		if err == nil && len(memories) > 0 {
			var b strings.Builder
			b.WriteString(basePrompt)
			b.WriteString("\n\nREFERENCE MEMORIES (approved facts - do not invent new ones):\n")
			for _, mem := range memories {
				if mem.Content != "" {
					b.WriteString(fmt.Sprintf("- %s\n", mem.Content))
				}
			}
			b.WriteString("\nUse these memories to stay consistent. Do not claim knowledge beyond these approved memories.\n")
			return b.String()
		}
	}

	return basePrompt
}

func generateStylePrompt(profile *PersonalityProfile) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("You ARE %s. You are writing as yourself on Discord.\n", profile.Username))
	b.WriteString("Write exactly as you normally would. Be authentic to your own communication style.\n\n")

	b.WriteString("STYLE RULES:\n")
	switch profile.Capitalization {
	case "lowercase":
		b.WriteString("- lowercase style: mostly lowercase, minimal sentence caps.\n")
	case "uppercase":
		b.WriteString("- emphasis caps: occasional ALL CAPS for emphasis.\n")
	case "mixed":
		b.WriteString("- mixed casual capitalization.\n")
	default:
		b.WriteString("- normal capitalization.\n")
	}

	b.WriteString(fmt.Sprintf("- punctuation: %s\n", profile.PunctuationStyle))

	if len(profile.ToneIndicators) > 0 {
		b.WriteString("- tone: " + strings.Join(profile.ToneIndicators, ", ") + "\n")
	}

	if len(profile.CommonWords) > 0 {
		b.WriteString("- common words: " + strings.Join(profile.CommonWords, ", ") + "\n")
	}

	if len(profile.CommonPhrases) > 0 {
		b.WriteString("- common phrases: " + strings.Join(profile.CommonPhrases, ", ") + "\n")
	}

	if len(profile.EmojiUsage) > 0 {
		b.WriteString("- emoji set: " + strings.Join(profile.EmojiUsage, " ") + "\n")
	} else {
		b.WriteString("- emoji: rarely\n")
	}

	// Formatting habits summary
	b.WriteString("- formatting habits:\n")
	b.WriteString(fmt.Sprintf("  - code ticks rate ~%.2f, code blocks ~%.2f, multiline ~%.2f, ellipses ~%.2f\n",
		profile.FormatHabits.CodeTicksRate,
		profile.FormatHabits.CodeBlockRate,
		profile.FormatHabits.MultiLineRate,
		profile.FormatHabits.EllipsisRate,
	))

	// Message length guidance
	if profile.AvgMessageLength < 50 {
		b.WriteString("- message length: short and concise\n")
	} else if profile.AvgMessageLength > 150 {
		b.WriteString("- message length: longer, detailed messages\n")
	}

	b.WriteString("\nIMPORTANT GUIDELINES:\n")
	b.WriteString("- Write naturally and authentically in your own style.\n")
	b.WriteString("- Do NOT quote the provided examples verbatim - use them as style reference only.\n")
	b.WriteString("- Stay true to your communication patterns and vocabulary.\n")
	b.WriteString("- Be authentic to yourself in every response.\n")

	// Optional: provide 3–5 short examples but instruct "pattern only"
	if len(profile.SampleMessages) > 0 {
		b.WriteString("\nEXAMPLES (pattern only, never copy):\n")
		for _, s := range profile.SampleMessages {
			s = strings.ReplaceAll(s, "\n", " ")
			if len(s) > 140 {
				s = s[:140] + "…"
			}
			b.WriteString("- " + s + "\n")
		}
	}

	b.WriteString("\nRespond naturally as yourself. Be authentic to your communication style in every message.\n")

	return b.String()
}

