package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestHealthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "ok", response["status"])
}

func TestChatEndpoint_InvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Mock endpoint
	router.POST("/api/agent/:id/chat", func(c *gin.Context) {
		var req struct {
			Message string `json:"message" binding:"required"`
			UserID  string `json:"user_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"content": "response"})
	})

	// Test missing fields
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/agent/test/chat", bytes.NewBuffer([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMemoryUpdateEndpoint_InvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Mock endpoint
	router.POST("/api/memory/:id/update", func(c *gin.Context) {
		var req struct {
			BlockName string `json:"block_name" binding:"required"`
			Content   string `json:"content" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "updated"})
	})

	// Test missing fields
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/memory/test/update", bytes.NewBuffer([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

