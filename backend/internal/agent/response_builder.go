package agent

import (
	"ezra-clone/backend/internal/adapter"
)

// BuildTurnResult builds a TurnResult from LLM response and processed tool data
func BuildTurnResult(
	llmResponse *adapter.Response,
	embeds []Embed,
	imageData []byte,
	imageName string,
	imageMeta map[string]interface{},
) *TurnResult {
	return &TurnResult{
		Content:   llmResponse.Content,
		ToolCalls: llmResponse.ToolCalls,
		Ignored:   false,
		Embeds:    embeds,
		ImageData: imageData,
		ImageName: imageName,
		ImageMeta: imageMeta,
	}
}

