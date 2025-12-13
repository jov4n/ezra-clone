package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/agent"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/state"
	"ezra-clone/backend/internal/tools"
	"ezra-clone/backend/pkg/config"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	if err := logger.Init("development"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer logger.Sync()

	log := logger.Get()
	log.Info("Starting HTTP API server...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", zap.Error(err))
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
	
	// Initialize ComfyUI executor (always initialize for prompt enhancement, RunPod optional for image generation)
	comfyExecutor := tools.NewComfyExecutor(llmAdapter, cfg)
	agentOrch.SetComfyExecutor(comfyExecutor)
	if cfg.RunPodAPIKey != "" && cfg.RunPodEndpointID != "" {
		log.Info("ComfyUI executor initialized with RunPod", zap.String("endpoint_id", cfg.RunPodEndpointID))
	} else {
		log.Info("ComfyUI executor initialized (prompt enhancement only, RunPod not configured)")
	}

	// Setup Gin router
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(ginLogger(log))
	router.Use(gin.Recovery())

	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API routes
	api := router.Group("/api")
	{
		// List all agents
		api.GET("/agents", func(c *gin.Context) {
			ctx := c.Request.Context()

			agents, err := graphRepo.ListAgents(ctx)
			if err != nil {
				log.Error("Failed to list agents", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list agents"})
				return
			}

			c.JSON(http.StatusOK, agents)
		})

		// Get agent state
		api.GET("/agent/:id/state", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			state, err := graphRepo.FetchState(ctx, agentID)
			if err != nil {
				if _, ok := err.(graph.ErrAgentNotFound); ok {
					c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
					return
				}
				log.Error("Failed to fetch agent state", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch state"})
				return
			}

			c.JSON(http.StatusOK, state)
		})

		// Get agent configuration
		api.GET("/agent/:id/config", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			config, err := graphRepo.GetAgentConfig(ctx, agentID)
			if err != nil {
				if _, ok := err.(graph.ErrAgentNotFound); ok {
					c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
					return
				}
				log.Error("Failed to get agent config", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config"})
				return
			}

			// If model is not set, use default from config
			if config.Model == "" {
				config.Model = cfg.ModelID
			}

			c.JSON(http.StatusOK, config)
		})

		// Update agent configuration
		api.PUT("/agent/:id/config", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			var req graph.AgentConfig
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			if err := graphRepo.UpdateAgentConfig(ctx, agentID, req); err != nil {
				if _, ok := err.(graph.ErrAgentNotFound); ok {
					c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
					return
				}
				log.Error("Failed to update agent config", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update config"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "updated"})
		})

		// Get available tools for agent
		api.GET("/agent/:id/tools", func(c *gin.Context) {
			// Tools are the same for all agents, return all available tools
			allTools := tools.GetAllTools()
			c.JSON(http.StatusOK, allTools)
		})

		// Get context window statistics
		api.GET("/agent/:id/context", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			stats, err := graphRepo.GetContextStats(ctx, agentID)
			if err != nil {
				if _, ok := err.(graph.ErrAgentNotFound); ok {
					c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
					return
				}
				log.Error("Failed to get context stats", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get context stats"})
				return
			}

			c.JSON(http.StatusOK, stats)
		})

		// Get archival memories
		api.GET("/agent/:id/archival-memories", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			memories, err := graphRepo.GetArchivalMemories(ctx, agentID)
			if err != nil {
				if _, ok := err.(graph.ErrAgentNotFound); ok {
					c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
					return
				}
				log.Error("Failed to get archival memories", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get archival memories"})
				return
			}

			c.JSON(http.StatusOK, memories)
		})

		// Create archival memory
		api.POST("/agent/:id/archival-memories", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			var req graph.ArchivalMemory
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Set timestamp if not provided
			if req.Timestamp.IsZero() {
				req.Timestamp = time.Now()
			}

			if err := graphRepo.CreateArchivalMemory(ctx, agentID, req); err != nil {
				if _, ok := err.(graph.ErrAgentNotFound); ok {
					c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
					return
				}
				log.Error("Failed to create archival memory", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create archival memory"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "created"})
		})

		// Delete archival memory
		api.DELETE("/agent/:id/archival-memories/:memoryId", func(c *gin.Context) {
			agentID := c.Param("id")
			memoryID := c.Param("memoryId")
			ctx := c.Request.Context()

			if err := graphRepo.DeleteArchivalMemory(ctx, agentID, memoryID); err != nil {
				if _, ok := err.(graph.ErrAgentNotFound); ok {
					c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
					return
				}
				log.Error("Failed to delete archival memory", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete archival memory"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "deleted"})
		})

		// Get all facts for an agent
		api.GET("/agent/:id/facts", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			facts, err := graphRepo.GetAllFacts(ctx, agentID)
			if err != nil {
				log.Error("Failed to get facts", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get facts"})
				return
			}

			c.JSON(http.StatusOK, facts)
		})

		// Get all topics for an agent
		api.GET("/agent/:id/topics", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			topics, err := graphRepo.GetAllTopics(ctx, agentID)
			if err != nil {
				log.Error("Failed to get topics", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get topics"})
				return
			}

			c.JSON(http.StatusOK, topics)
		})

		// Get all messages for an agent
		api.GET("/agent/:id/messages", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()
			limit := 100
			if limitStr := c.Query("limit"); limitStr != "" {
				if parsed, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || parsed != 1 {
					limit = 100
				}
			}

			messages, err := graphRepo.GetAllMessages(ctx, agentID, limit)
			if err != nil {
				log.Error("Failed to get messages", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get messages"})
				return
			}

			c.JSON(http.StatusOK, messages)
		})

		// Get all conversations for an agent
		api.GET("/agent/:id/conversations", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()
			limit := 50
			if limitStr := c.Query("limit"); limitStr != "" {
				if parsed, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || parsed != 1 {
					limit = 50
				}
			}

			conversations, err := graphRepo.GetAllConversations(ctx, agentID, limit)
			if err != nil {
				log.Error("Failed to get conversations", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get conversations"})
				return
			}

			c.JSON(http.StatusOK, conversations)
		})

		// Get all users for an agent
		api.GET("/agent/:id/users", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			users, err := graphRepo.GetAllUsers(ctx, agentID)
			if err != nil {
				log.Error("Failed to get users", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get users"})
				return
			}

			c.JSON(http.StatusOK, users)
		})

		// Create new agent
		api.POST("/agents", func(c *gin.Context) {
			ctx := c.Request.Context()

			var req struct {
				Name              string `json:"name" binding:"required"`
				Model             string `json:"model"`
				SystemInstructions string `json:"system_instructions"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Generate agent ID from name (or use UUID)
			agentID := req.Name
			if err := graphRepo.CreateAgent(ctx, agentID, req.Name); err != nil {
				log.Error("Failed to create agent", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create agent"})
				return
			}

			// Set initial config if provided
			if req.Model != "" || req.SystemInstructions != "" {
				config := graph.AgentConfig{
					Model:              req.Model,
					SystemInstructions: req.SystemInstructions,
				}
				if err := graphRepo.UpdateAgentConfig(ctx, agentID, config); err != nil {
					log.Warn("Failed to set initial config", zap.Error(err))
				}
			}

			// Create default identity
			identity := state.AgentIdentity{
				Name:        req.Name,
				Personality: req.SystemInstructions,
				Capabilities: []string{
					"chat",
					"memory_management",
					"fact_tracking",
					"topic_organization",
				},
			}
			if err := graphRepo.CreateAgentIdentity(ctx, agentID, identity); err != nil {
				log.Warn("Failed to create agent identity", zap.Error(err))
			}

			c.JSON(http.StatusOK, gin.H{
				"id":   agentID,
				"name": req.Name,
			})
		})

		// Chat with agent
		api.POST("/agent/:id/chat", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			var req struct {
				Message string `json:"message" binding:"required"`
				UserID  string `json:"user_id" binding:"required"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			result, err := agentOrch.RunTurn(ctx, agentID, req.UserID, req.Message)
			if err != nil {
				if err == agent.ErrIgnored {
					c.JSON(http.StatusOK, gin.H{
						"ignored": true,
						"content": "",
					})
					return
				}
				log.Error("Failed to run agent turn", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process message"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"content":    result.Content,
				"tool_calls": result.ToolCalls,
				"ignored":    result.Ignored,
			})
		})

		// Update memory block
		api.POST("/memory/:id/update", func(c *gin.Context) {
			agentID := c.Param("id")
			ctx := c.Request.Context()

			var req struct {
				BlockName string `json:"block_name" binding:"required"`
				Content   string `json:"content" binding:"required"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			if err := graphRepo.UpdateMemory(ctx, agentID, req.BlockName, req.Content); err != nil {
				log.Error("Failed to update memory", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update memory"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "updated"})
		})

		// Delete memory block
		api.DELETE("/memory/:id/block/:blockName", func(c *gin.Context) {
			agentID := c.Param("id")
			blockName := c.Param("blockName")
			ctx := c.Request.Context()

			if err := graphRepo.DeleteMemory(ctx, agentID, blockName); err != nil {
				if _, ok := err.(graph.ErrAgentNotFound); ok {
					c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
					return
				}
				log.Error("Failed to delete memory", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete memory"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "deleted"})
		})

		// Get conversation history for a specific channel
		api.GET("/agent/:id/conversation-history", func(c *gin.Context) {
			agentID := c.Param("id")
			channelID := c.Query("channel_id")
			if channelID == "" {
				channelID = "web-" + agentID
			}
			limit := 20
			if limitStr := c.Query("limit"); limitStr != "" {
				if parsed, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || parsed != 1 {
					limit = 20
				}
			}

			ctx := c.Request.Context()
			messages, err := graphRepo.GetConversationHistory(ctx, channelID, limit)
			if err != nil {
				log.Error("Failed to get conversation history", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get conversation history"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"messages":   messages,
				"channel_id": channelID,
			})
		})
	}

	// Start server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	log.Info("Server started", zap.String("port", cfg.Port))

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", zap.Error(err))
	}

	log.Info("Server exited")
}

// ginLogger is a custom logger middleware for Gin
func ginLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		log.Info("HTTP Request",
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
		)
	}
}

