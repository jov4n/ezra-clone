package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetImageGenerationTools returns ComfyUI image generation tools
func GetImageGenerationTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolGenerateImageWithRunPod,
				Description: "Generate an image using ComfyUI workflows on RunPod. This tool loads or creates a workflow, submits it to RunPod, polls for completion, and saves the generated image. Use this after enhancing the prompt and selecting a workflow.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"prompt": map[string]interface{}{
							"type":        "string",
							"description": "Enhanced prompt for image generation (should be enhanced using enhance_prompt tool first)",
						},
						"workflow_name": map[string]interface{}{
							"type":        "string",
							"description": "Name of workflow JSON file (optional, leave empty to use programmatic Z-Image Turbo workflow)",
						},
						"width": map[string]interface{}{
							"type":        "integer",
							"description": "Image width in pixels (default: 1280)",
						},
						"height": map[string]interface{}{
							"type":        "integer",
							"description": "Image height in pixels (default: 1440)",
						},
						"seed": map[string]interface{}{
							"type":        "integer",
							"description": "Random seed for reproducibility (optional, random if not provided)",
						},
					},
					"required": []string{"prompt"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolEnhancePrompt,
				Description: "Enhance a user's image generation prompt using Z-Image Turbo methodology. This optimizes the prompt for the Qwen 3.4B CLIP model used in Z-Image Turbo workflows. Call this FIRST before generating images.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_request": map[string]interface{}{
							"type":        "string",
							"description": "The original user request/prompt to enhance",
						},
					},
					"required": []string{"user_request"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolSelectWorkflow,
				Description: "Select the best workflow for image generation based on the user's request and enhanced prompt. By default, returns None to use the programmatic Z-Image Turbo workflow. Call this AFTER enhance_prompt.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_request": map[string]interface{}{
							"type":        "string",
							"description": "Original user request",
						},
						"enhanced_prompt": map[string]interface{}{
							"type":        "string",
							"description": "Enhanced prompt from enhance_prompt tool",
						},
					},
					"required": []string{"user_request", "enhanced_prompt"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolListWorkflows,
				Description: "List available ComfyUI workflow JSON files from the configured workflow directory.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
					"required":   []string{},
				},
			},
		},
	}
}

