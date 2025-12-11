package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

// RunPodClient handles communication with RunPod Serverless API
type RunPodClient struct {
	apiKey     string
	endpointID string
	httpClient *http.Client
	logger     *zap.Logger
}

// JobRequest represents a job submission request
type JobRequest struct {
	Input map[string]interface{} `json:"input"`
}

// JobResponse represents the response from job submission
type JobResponse struct {
	ID string `json:"id"`
}

// JobStatus represents the status of a job
type JobStatus struct {
	Status string                 `json:"status"`
	Output map[string]interface{} `json:"output,omitempty"`
	Error  string                 `json:"error,omitempty"`
}

// ImageData represents image data from RunPod response
type ImageData struct {
	Data string `json:"data"` // base64 encoded image
}

// NewRunPodClient creates a new RunPod client
func NewRunPodClient(apiKey, endpointID string) *RunPodClient {
	return &RunPodClient{
		apiKey:     apiKey,
		endpointID: endpointID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.Get(),
	}
}

// SubmitJob submits a workflow to RunPod Serverless API
func (c *RunPodClient) SubmitJob(ctx context.Context, workflowPayload map[string]interface{}) (string, error) {
	url := fmt.Sprintf("https://api.runpod.ai/v2/%s/run", c.endpointID)

	reqBody := JobRequest{
		Input: workflowPayload,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Body = io.NopCloser(bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	c.logger.Debug("Submitting job to RunPod",
		zap.String("endpoint", c.endpointID),
		zap.String("url", url),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to submit job: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("RunPod API error",
			zap.Int("status_code", resp.StatusCode),
			zap.String("endpoint_id", c.endpointID),
			zap.String("url", url),
			zap.String("response_body", string(body)),
		)
		return "", fmt.Errorf("RunPod API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	if len(body) == 0 {
		return "", fmt.Errorf("empty response from RunPod API")
	}

	var jobResp JobResponse
	if err := json.Unmarshal(body, &jobResp); err != nil {
		c.logger.Error("Failed to decode RunPod response",
			zap.Error(err),
			zap.String("response_body", string(body)),
		)
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if jobResp.ID == "" {
		return "", fmt.Errorf("empty job ID in response")
	}

	c.logger.Info("Job submitted successfully", zap.String("job_id", jobResp.ID))
	return jobResp.ID, nil
}

// PollStatus polls for job completion
func (c *RunPodClient) PollStatus(ctx context.Context, jobID string, maxPolls int, pollInterval time.Duration) (*JobStatus, error) {
	url := fmt.Sprintf("https://api.runpod.ai/v2/%s/status/%s", c.endpointID, jobID)

	c.logger.Debug("Polling job status",
		zap.String("job_id", jobID),
		zap.Int("max_polls", maxPolls),
	)

	for i := 0; i < maxPolls; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.logger.Warn("Poll request failed, retrying",
				zap.Error(err),
				zap.Int("attempt", i+1),
			)
			time.Sleep(pollInterval)
			continue
		}

		var status JobStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			resp.Body.Close()
			c.logger.Warn("Failed to decode status, retrying",
				zap.Error(err),
				zap.Int("attempt", i+1),
			)
			time.Sleep(pollInterval)
			continue
		}
		resp.Body.Close()

		c.logger.Debug("Job status",
			zap.String("job_id", jobID),
			zap.String("status", status.Status),
			zap.Int("poll", i+1),
		)

		switch status.Status {
		case "COMPLETED":
			return &status, nil
		case "FAILED":
			return &status, fmt.Errorf("job failed: %s", status.Error)
		case "IN_QUEUE", "IN_PROGRESS":
			// Continue polling
		default:
			c.logger.Warn("Unknown job status", zap.String("status", status.Status))
		}

		// Wait before next poll
		if i < maxPolls-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(pollInterval):
			}
		}
	}

	return nil, fmt.Errorf("job did not complete within %d polls", maxPolls)
}

// GetJobOutput extracts image data from a completed job
func (c *RunPodClient) GetJobOutput(status *JobStatus) ([]byte, error) {
	if status.Output == nil {
		return nil, fmt.Errorf("no output in job status")
	}

	images, ok := status.Output["images"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no images in output")
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("empty images array")
	}

	// Get first image
	imageObj, ok := images[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid image object format")
	}

	dataStr, ok := imageObj["data"].(string)
	if !ok {
		return nil, fmt.Errorf("no data field in image object")
	}

	// Decode base64 image
	imageBytes, err := base64.StdEncoding.DecodeString(dataStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 image: %w", err)
	}

	return imageBytes, nil
}

