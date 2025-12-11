package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/pkg/config"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

// ComfyExecutor handles ComfyUI image generation tool execution
type ComfyExecutor struct {
	runpodClient   *RunPodClient
	promptEnhancer *PromptEnhancer
	llmAdapter     *adapter.LLMAdapter
	config         *config.Config
	logger         *zap.Logger
}

// NewComfyExecutor creates a new ComfyUI executor
func NewComfyExecutor(llmAdapter *adapter.LLMAdapter, cfg *config.Config) *ComfyExecutor {
	var runpodClient *RunPodClient
	if cfg.RunPodAPIKey != "" && cfg.RunPodEndpointID != "" {
		runpodClient = NewRunPodClient(cfg.RunPodAPIKey, cfg.RunPodEndpointID)
	}

	return &ComfyExecutor{
		runpodClient:   runpodClient,
		promptEnhancer: NewPromptEnhancer(llmAdapter),
		llmAdapter:     llmAdapter,
		config:         cfg,
		logger:         logger.Get(),
	}
}

// executeEnhancePrompt enhances a user prompt using Z-Image Turbo methodology
func (e *Executor) executeEnhancePrompt(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	userRequest, _ := args["user_request"].(string)
	if userRequest == "" {
		return &ToolResult{
			Success: false,
			Error:   "user_request is required",
		}
	}

	// Get ComfyExecutor (prompt enhancement doesn't require RunPod)
	if e.comfyExecutor == nil {
		return &ToolResult{
			Success: false,
			Error:   "ComfyUI executor not initialized",
		}
	}

	enhanced, err := e.comfyExecutor.promptEnhancer.Enhance(ctx, userRequest)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to enhance prompt: %v", err),
		}
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"enhanced_prompt": enhanced,
			"original":       userRequest,
		},
		Message: "Prompt enhanced successfully",
	}
}

// executeSelectWorkflow selects the best workflow for the request
func (e *Executor) executeSelectWorkflow(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	userRequest, _ := args["user_request"].(string)
	_, _ = args["enhanced_prompt"].(string) // Enhanced prompt available but not used for now

	// For now, default to programmatic workflow (None)
	// This matches the reference implementation behavior
	e.logger.Debug("Selecting workflow",
		zap.String("user_request", truncateString(userRequest, 50)),
	)

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"workflow_name": nil,
			"reasoning":     "Using proven programmatic Z-Image Turbo workflow",
		},
		Message: "Selected programmatic Z-Image Turbo workflow",
	}
}

// executeListWorkflows lists available workflow JSON files
func (e *Executor) executeListWorkflows(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.comfyExecutor == nil {
		return &ToolResult{
			Success: false,
			Error:   "ComfyUI executor not initialized",
		}
	}

	workflows, err := ListAvailableWorkflows(e.comfyExecutor.config.ComfyUIWorkflowDir)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to list workflows: %v", err),
		}
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"workflows": workflows,
			"count":     len(workflows),
		},
		Message: fmt.Sprintf("Found %d workflows", len(workflows)),
	}
}

// executeGenerateImageWithRunPod generates an image using RunPod
func (e *Executor) executeGenerateImageWithRunPod(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.comfyExecutor == nil || e.comfyExecutor.runpodClient == nil {
		return &ToolResult{
			Success: false,
			Error:   "RunPod not configured (missing API key or endpoint ID)",
		}
	}

	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return &ToolResult{
			Success: false,
			Error:   "prompt is required",
		}
	}

	workflowName, _ := args["workflow_name"].(string)
	width := 1280
	height := 1440
	seed := (*int)(nil)

	if w, ok := args["width"].(float64); ok {
		width = int(w)
	}
	if h, ok := args["height"].(float64); ok {
		height = int(h)
	}
	if s, ok := args["seed"].(float64); ok {
		seedVal := int(s)
		seed = &seedVal
	}

	e.logger.Info("Starting image generation",
		zap.String("workflow", workflowName),
		zap.Int("width", width),
		zap.Int("height", height),
	)

	startTime := time.Now()

	// Load or create workflow
	var workflowPayload map[string]interface{}
	if workflowName == "" || workflowName == "<nil>" {
		// Use programmatic Z-Image Turbo workflow
		e.logger.Debug("Using programmatic Z-Image Turbo workflow")
		workflowPayload = CreateZImageTurboWorkflow(prompt, seed, width, height, 4, 1.0)
	} else {
		// Load workflow from file
		workflow, err := LoadWorkflow(e.comfyExecutor.config.ComfyUIWorkflowDir, workflowName)
		if err != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to load workflow: %v", err),
			}
		}

		prepared, err := PrepareWorkflowForAPI(workflow, prompt, seed, width, height)
		if err != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to prepare workflow: %v", err),
			}
		}
		workflowPayload = prepared
	}

	// Log workflow payload for debugging (first 500 chars)
	workflowJSON, _ := json.Marshal(workflowPayload)
	workflowStr := string(workflowJSON)
	if len(workflowStr) > 500 {
		workflowStr = workflowStr[:500] + "..."
	}
	e.logger.Debug("Submitting workflow to RunPod",
		zap.String("workflow_preview", workflowStr),
	)

	// Submit job to RunPod
	jobID, err := e.comfyExecutor.runpodClient.SubmitJob(ctx, workflowPayload)
	if err != nil {
		e.logger.Error("Failed to submit job to RunPod",
			zap.Error(err),
			zap.String("endpoint_id", e.comfyExecutor.config.RunPodEndpointID),
		)
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to submit job to RunPod: %v. Please verify your RUNPOD_ENDPOINT_ID is correct and the endpoint exists.", err),
		}
	}

	e.logger.Info("Job submitted", zap.String("job_id", jobID))

	// Poll for completion
	status, err := e.comfyExecutor.runpodClient.PollStatus(ctx, jobID, 120, 5*time.Second)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Job failed or timed out: %v", err),
			Data: map[string]interface{}{
				"job_id": jobID,
			},
		}
	}

	if status.Status != "COMPLETED" {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Job status: %s, error: %s", status.Status, status.Error),
			Data: map[string]interface{}{
				"job_id": jobID,
			},
		}
	}

	// Extract image data
	imageBytes, err := e.comfyExecutor.runpodClient.GetJobOutput(status)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to extract image: %v", err),
			Data: map[string]interface{}{
				"job_id": jobID,
			},
		}
	}

	if seed == nil {
		rand.Seed(time.Now().UnixNano())
		s := rand.Intn(1 << 32)
		seed = &s
	}

	elapsed := time.Since(startTime).Seconds()

	e.logger.Info("Image generated successfully",
		zap.Int("image_size_bytes", len(imageBytes)),
		zap.Float64("elapsed_seconds", elapsed),
	)

	// Return image data in result for Discord attachment
	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"image_data":     imageBytes, // Image bytes for Discord attachment
			"image_format":   "png",
			"seed":           *seed,
			"width":          width,
			"height":         height,
			"workflow":       workflowName,
			"job_id":         jobID,
			"elapsed_seconds": elapsed,
		},
		Message: fmt.Sprintf("Image generated successfully in %.1fs", elapsed),
	}
}



