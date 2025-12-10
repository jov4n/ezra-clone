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

