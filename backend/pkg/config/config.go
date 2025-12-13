package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// App
	Port string
	Env  string

	// Neo4j
	Neo4jURI      string
	Neo4jUser     string
	Neo4jPassword string

	// AI
	LiteLLMURL      string
	ModelID         string
	OpenRouterAPIKey string

	// Discord
	DiscordBotToken string
	MimicChannelID  string // Channel ID for mimic mode auto-posts

	// RunPod
	RunPodAPIKey     string
	RunPodEndpointID string
	ComfyUIWorkflowDir string
	ComfyUIOutputDir   string

	// Voice Services
	STTServiceURL      string // Speech-to-text service URL
	TTSServiceURL      string // Text-to-speech service URL
	VoiceReferenceDir  string // Directory for voice reference audio files
	VADThreshold      float64 // Voice activity detection energy threshold
	SilenceDuration    int     // Milliseconds of silence before considering speech ended
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	// Try to load .env file, but don't fail if it doesn't exist
	_ = godotenv.Load()

	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		Env:             getEnv("ENV", "development"),
		Neo4jURI:        getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Neo4jUser:       getEnv("NEO4J_USER", "neo4j"),
		Neo4jPassword:   getEnv("NEO4J_PASSWORD", "password"),
		LiteLLMURL:      getEnv("LITELLM_URL", "http://localhost:4000"),
		ModelID:         getEnv("MODEL_ID", "openrouter/anthropic/claude-3.5-sonnet"),
		OpenRouterAPIKey: getEnv("OPENROUTER_API_KEY", ""),
		DiscordBotToken:  getEnv("DISCORD_BOT_TOKEN", ""),
		MimicChannelID:   getEnv("MIMIC_CHANNEL_ID", "549646869744058378"),
		RunPodAPIKey:     getEnv("RUNPOD_API_KEY", ""),
		RunPodEndpointID: getEnv("RUNPOD_ENDPOINT_ID", ""),
		ComfyUIWorkflowDir: getEnv("COMFYUI_WORKFLOW_DIR", ""),
		ComfyUIOutputDir:   getEnv("COMFYUI_OUTPUT_DIR", "outputs"),
		STTServiceURL:      getEnv("STT_SERVICE_URL", "http://localhost:8001"),
		TTSServiceURL:      getEnv("TTS_SERVICE_URL", "http://localhost:8002"),
		VoiceReferenceDir:   getEnv("VOICE_REFERENCE_DIR", "voice_references"),
		VADThreshold:        getEnvFloat("VAD_THRESHOLD", 0.01),
		SilenceDuration:     getEnvInt("SILENCE_DURATION", 1000),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that required configuration values are set
func (c *Config) Validate() error {
	if c.Neo4jURI == "" {
		return fmt.Errorf("NEO4J_URI is required")
	}
	if c.Neo4jUser == "" {
		return fmt.Errorf("NEO4J_USER is required")
	}
	if c.Neo4jPassword == "" {
		return fmt.Errorf("NEO4J_PASSWORD is required")
	}
	if c.LiteLLMURL == "" {
		return fmt.Errorf("LITELLM_URL is required")
	}
	if c.ModelID == "" {
		return fmt.Errorf("MODEL_ID is required")
	}
	// OpenRouter API key and Discord token are optional for development
	return nil
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		var result float64
		if _, err := fmt.Sscanf(value, "%f", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}


