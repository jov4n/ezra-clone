package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/agent"
	"ezra-clone/backend/internal/discord"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/tools"
	"ezra-clone/backend/pkg/config"
	"ezra-clone/backend/pkg/logger"

	"github.com/bwmarrin/discordgo"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	if err := logger.Init("development"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer logger.Sync()

	log := logger.Get()
	log.Info("Starting Discord bot...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", zap.Error(err))
	}

	if cfg.DiscordBotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is required")
	}

	// Initialize Neo4j driver
	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPassword, ""),
	)
	if err != nil {
		log.Fatal("Failed to create Neo4j driver", zap.Error(err))
	}
	defer driver.Close(context.Background())

	// Verify Neo4j connection
	ctx := context.Background()
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatal("Failed to verify Neo4j connectivity", zap.Error(err))
	}

	// Initialize dependencies
	graphRepo := graph.NewRepository(driver)
	llmAdapter := adapter.NewLLMAdapter(cfg.LiteLLMURL, cfg.OpenRouterAPIKey, cfg.ModelID)
	agentOrch := agent.NewOrchestrator(graphRepo, llmAdapter)

	// Set LLM adapter for website summarization (uses LiteLLM)
	agentOrch.SetLLMAdapterForTools(llmAdapter)

	// Create Discord session
	dg, err := discordgo.New("Bot " + cfg.DiscordBotToken)
	if err != nil {
		log.Fatal("Failed to create Discord session", zap.Error(err))
	}

	// Create Discord executor for Discord-specific tools
	discordExecutor := tools.NewDiscordExecutor(dg, log)
	discordExecutor.SetRepository(graphRepo) // Enable RAG memory access
	agentOrch.SetDiscordExecutor(discordExecutor)

	// Initialize ComfyUI executor (always initialize for prompt enhancement, RunPod optional for image generation)
	comfyExecutor := tools.NewComfyExecutor(llmAdapter, cfg)
	agentOrch.SetComfyExecutor(comfyExecutor)
	if cfg.RunPodAPIKey != "" && cfg.RunPodEndpointID != "" {
		log.Info("ComfyUI executor initialized with RunPod", zap.String("endpoint_id", cfg.RunPodEndpointID))
	} else {
		log.Info("ComfyUI executor initialized (prompt enhancement only, RunPod not configured)")
	}

	// Initialize Music executor
	musicExecutor := tools.NewMusicExecutor(dg, log, llmAdapter)
	agentOrch.SetMusicExecutor(musicExecutor)
	log.Info("Music executor initialized")

	// Initialize Mimic background task
	mimicTask := tools.NewMimicBackgroundTask(
		agentOrch.GetToolExecutor(),
		llmAdapter,
		dg,
		cfg,
		log,
	)
	agentOrch.SetMimicBackgroundTask(mimicTask)
	log.Info("Mimic background task initialized",
		zap.String("mimic_channel_id", cfg.MimicChannelID),
	)

	// Create shutdown channel for programmatic shutdown
	shutdownChan := make(chan os.Signal, 1)

	// Initialize System executor with shutdown function
	systemExecutor := tools.NewSystemExecutor(dg, log, func() {
		shutdownChan <- os.Interrupt
	})
	agentOrch.SetSystemExecutor(systemExecutor)
	log.Info("System executor initialized")

	// Create message handler
	messageHandler := discord.NewHandler(agentOrch, graphRepo, log)

	// Add message handler
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		messageHandler.HandleMessage(s, m)
	})

	// Set intents (including voice state for music bot)
	// Required intents:
	// - IntentsGuilds: Access to guild information
	// - IntentsGuildMessages: Read messages in guild channels
	// - IntentsDirectMessages: Read DM messages
	// - IntentsGuildVoiceStates: Track voice state changes (REQUIRED for voice connections)
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsGuildVoiceStates

	// Log intents for debugging
	log.Info("Discord bot intents configured",
		zap.Bool("guilds", (dg.Identify.Intents&discordgo.IntentsGuilds) != 0),
		zap.Bool("guild_messages", (dg.Identify.Intents&discordgo.IntentsGuildMessages) != 0),
		zap.Bool("direct_messages", (dg.Identify.Intents&discordgo.IntentsDirectMessages) != 0),
		zap.Bool("guild_voice_states", (dg.Identify.Intents&discordgo.IntentsGuildVoiceStates) != 0),
	)

	// Open connection
	if err := dg.Open(); err != nil {
		log.Fatal("Failed to open Discord connection", zap.Error(err))
	}
	defer dg.Close()

	log.Info("Discord bot is running. Press CTRL-C to exit.")

	// Wait for interrupt signal (from CTRL-C or programmatic shutdown)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-shutdownChan

	log.Info("Shutting down Discord bot...")
}
