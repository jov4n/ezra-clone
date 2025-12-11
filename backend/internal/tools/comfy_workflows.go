package tools

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

// ListAvailableWorkflows lists all JSON workflow files from the configured directory
func ListAvailableWorkflows(workflowDir string) ([]string, error) {
	if workflowDir == "" {
		return []string{}, nil
	}

	dir := filepath.Clean(workflowDir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []string{}, nil
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	var workflows []string
	for _, file := range files {
		workflows = append(workflows, filepath.Base(file))
	}

	return workflows, nil
}

// LoadWorkflow loads a workflow from a JSON file
func LoadWorkflow(workflowDir, name string) (map[string]interface{}, error) {
	if workflowDir == "" {
		return nil, fmt.Errorf("workflow directory not configured")
	}

	workflowPath := filepath.Join(workflowDir, name)
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		available, _ := ListAvailableWorkflows(workflowDir)
		return nil, fmt.Errorf("workflow '%s' not found. Available workflows: %v", name, available)
	}

	var workflow map[string]interface{}
	if err := json.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("failed to parse workflow JSON: %w", err)
	}

	return workflow, nil
}

// CreateZImageTurboWorkflow creates a programmatic Z-Image Turbo workflow
// Based on the reference implementation from img_zurbo.ipynb
func CreateZImageTurboWorkflow(prompt string, seed *int, width, height, steps int, cfg float64) map[string]interface{} {
	if seed == nil {
		rand.Seed(time.Now().UnixNano())
		s := rand.Intn(1 << 32)
		seed = &s
	}

	workflow := map[string]interface{}{
		"9": map[string]interface{}{
			"inputs": map[string]interface{}{
				"filename_prefix": "Z-Image\\ComfyUI",
				"images": []interface{}{"43", 0},
			},
			"class_type": "SaveImage",
			"_meta":      map[string]interface{}{"title": "Save Image"},
		},
		"39": map[string]interface{}{
			"inputs": map[string]interface{}{
				"clip_name": "qwen_3_4b.safetensors",
				"type":      "lumina2",
				"device":    "default",
			},
			"class_type": "CLIPLoader",
			"_meta":      map[string]interface{}{"title": "Load CLIP"},
		},
		"40": map[string]interface{}{
			"inputs": map[string]interface{}{
				"vae_name": "ae.safetensors",
			},
			"class_type": "VAELoader",
			"_meta":      map[string]interface{}{"title": "Load VAE"},
		},
		"41": map[string]interface{}{
			"inputs": map[string]interface{}{
				"width":      width,
				"height":     height,
				"batch_size": 1,
			},
			"class_type": "EmptySD3LatentImage",
			"_meta":      map[string]interface{}{"title": "EmptySD3LatentImage"},
		},
		"42": map[string]interface{}{
			"inputs": map[string]interface{}{
				"conditioning": []interface{}{"45", 0},
			},
			"class_type": "ConditioningZeroOut",
			"_meta":      map[string]interface{}{"title": "ConditioningZeroOut"},
		},
		"43": map[string]interface{}{
			"inputs": map[string]interface{}{
				"samples": []interface{}{"44", 0},
				"vae":     []interface{}{"40", 0},
			},
			"class_type": "VAEDecode",
			"_meta":      map[string]interface{}{"title": "VAE Decode"},
		},
		"44": map[string]interface{}{
			"inputs": map[string]interface{}{
				"seed":         *seed,
				"steps":        steps,
				"cfg":          cfg,
				"sampler_name": "res_multistep",
				"scheduler":    "simple",
				"denoise":      1,
				"model":        []interface{}{"48", 0},
				"positive":     []interface{}{"45", 0},
				"negative":     []interface{}{"42", 0},
				"latent_image": []interface{}{"41", 0},
			},
			"class_type": "KSampler",
			"_meta":      map[string]interface{}{"title": "KSampler"},
		},
		"45": map[string]interface{}{
			"inputs": map[string]interface{}{
				"text": prompt,
				"clip": []interface{}{"39", 0},
			},
			"class_type": "CLIPTextEncode",
			"_meta":      map[string]interface{}{"title": "CLIP Text Encode (Prompt)"},
		},
		"48": map[string]interface{}{
			"inputs": map[string]interface{}{
				"unet_name":   "z_image_turbo_bf16.safetensors",
				"weight_dtype": "fp8_e4m3fn",
			},
			"class_type": "UNETLoader",
			"_meta":      map[string]interface{}{"title": "Unet Loader"},
		},
	}

	return map[string]interface{}{
		"workflow": workflow,
	}
}

// PrepareWorkflowForAPI prepares a workflow for RunPod API submission
// Handles both API-format (dict) and UI-format (list) workflows
func PrepareWorkflowForAPI(workflow map[string]interface{}, prompt string, seed *int, width, height int) (map[string]interface{}, error) {
	if seed == nil {
		rand.Seed(time.Now().UnixNano())
		s := rand.Intn(1 << 32)
		seed = &s
	}

	var workflowNodes map[string]interface{}

	// Check if workflow is in UI format (has "nodes" array)
	if nodes, ok := workflow["nodes"].([]interface{}); ok {
		// Convert UI format to API format
		workflowNodes = make(map[string]interface{})
		for _, nodeRaw := range nodes {
			node, ok := nodeRaw.(map[string]interface{})
			if !ok {
				continue
			}

			nodeID, ok := node["id"].(float64)
			if !ok {
				continue
			}

			nodeIDStr := fmt.Sprintf("%.0f", nodeID)
			nodeType, _ := node["type"].(string)

			// Convert inputs
			inputs := make(map[string]interface{})
			if inputsList, ok := node["inputs"].([]interface{}); ok {
				for _, inpRaw := range inputsList {
					inp, ok := inpRaw.(map[string]interface{})
					if !ok {
						continue
					}
					name, _ := inp["name"].(string)
					link, ok := inp["link"].(float64)
					if name != "" && ok {
						inputs[name] = []interface{}{fmt.Sprintf("%.0f", link), 0}
					}
				}
			}

			// Map widgets_values to inputs for common node types
			if widgets, ok := node["widgets_values"].([]interface{}); ok {
				switch nodeType {
				case "CLIPTextEncode":
					if len(widgets) >= 1 {
						inputs["text"] = widgets[0]
					}
				case "VAELoader":
					if len(widgets) >= 1 {
						inputs["vae_name"] = widgets[0]
					}
				case "CheckpointLoaderSimple":
					if len(widgets) >= 1 {
						inputs["ckpt_name"] = widgets[0]
					}
				case "EmptyLatentImage", "EmptySD3LatentImage":
					if len(widgets) >= 2 {
						inputs["width"] = widgets[0]
						inputs["height"] = widgets[1]
						if len(widgets) >= 3 {
							inputs["batch_size"] = widgets[2]
						}
					}
				case "RandomNoise":
					if len(widgets) >= 1 {
						inputs["noise_seed"] = widgets[0]
					}
				case "KSampler":
					if len(widgets) >= 4 {
						inputs["seed"] = widgets[0]
						inputs["steps"] = widgets[2]
						inputs["cfg"] = widgets[3]
						if len(widgets) >= 5 {
							inputs["sampler_name"] = widgets[4]
						}
						if len(widgets) >= 6 {
							inputs["scheduler"] = widgets[5]
						}
						if len(widgets) >= 7 {
							inputs["denoise"] = widgets[6]
						}
					}
				}
			}

			workflowNodes[nodeIDStr] = map[string]interface{}{
				"inputs":    inputs,
				"class_type": nodeType,
				"_meta":     map[string]interface{}{"title": nodeType},
			}
		}
	} else {
		// Already in API format
		workflowNodes = workflow
	}

	// Apply parameter overrides
	for _, nodeDataRaw := range workflowNodes {
		nodeData, ok := nodeDataRaw.(map[string]interface{})
		if !ok {
			continue
		}

		nodeType, _ := nodeData["class_type"].(string)
		inputs, _ := nodeData["inputs"].(map[string]interface{})
		if inputs == nil {
			inputs = make(map[string]interface{})
			nodeData["inputs"] = inputs
		}

		switch nodeType {
		case "CLIPTextEncode":
			inputs["text"] = prompt
		case "RandomNoise":
			inputs["noise_seed"] = *seed
		case "KSampler":
			inputs["seed"] = *seed
		case "EmptyLatentImage", "EmptySD3LatentImage":
			inputs["width"] = width
			inputs["height"] = height
		}
	}

	return map[string]interface{}{
		"workflow": workflowNodes,
	}, nil
}

// GetModifiableNodes extracts key nodes that are commonly modified in workflows
func GetModifiableNodes(workflow map[string]interface{}) map[string]map[string]interface{} {
	modifiable := make(map[string]map[string]interface{})

	targetTypes := map[string]string{
		"CLIPTextEncode":      "prompt",
		"RandomNoise":         "seed",
		"EmptyLatentImage":    "dimensions",
		"EmptySD3LatentImage": "dimensions",
		"UNETLoader":          "model",
		"LoraLoaderModelOnly": "lora",
		"KSampler":            "sampler_settings",
		"BasicScheduler":       "scheduler_settings",
	}

	// Handle API format
	for nodeID, nodeDataRaw := range workflow {
		nodeData, ok := nodeDataRaw.(map[string]interface{})
		if !ok {
			continue
		}

		nodeType, _ := nodeData["class_type"].(string)
		if purpose, exists := targetTypes[nodeType]; exists {
			modifiable[nodeID] = map[string]interface{}{
				"type":           nodeType,
				"purpose":        purpose,
				"current_inputs": nodeData["inputs"],
			}
		}
	}

	return modifiable
}

