package graph

import (
	"context"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// TestRepository requires a running Neo4j instance
// Set NEO4J_URI, NEO4J_USER, NEO4J_PASSWORD environment variables
func TestRepository_CreateAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	driver, err := createTestDriver()
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	defer driver.Close(ctx)

	repo := NewRepository(driver)
	agentID := "test-agent-" + time.Now().Format("20060102150405")

	// Clean up
	defer func() {
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)
		_, _ = session.Run(ctx, "MATCH (a:Agent {id: $id}) DETACH DELETE a", map[string]interface{}{"id": agentID})
	}()

	err = repo.CreateAgent(ctx, agentID, "Test Agent")
	if err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}

	// Verify agent exists
	state, err := repo.FetchState(ctx, agentID)
	if err != nil {
		t.Fatalf("FetchState failed: %v", err)
	}
	if state.Identity.Name != "Test Agent" {
		t.Errorf("Expected agent name 'Test Agent', got '%s'", state.Identity.Name)
	}
}

func TestRepository_UpdateMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	driver, err := createTestDriver()
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	defer driver.Close(ctx)

	repo := NewRepository(driver)
	agentID := "test-agent-" + time.Now().Format("20060102150405")

	// Setup
	err = repo.CreateAgent(ctx, agentID, "Test Agent")
	if err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}

	// Clean up
	defer func() {
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)
		_, _ = session.Run(ctx, "MATCH (a:Agent {id: $id}) DETACH DELETE a", map[string]interface{}{"id": agentID})
	}()

	// Test creating new memory block
	err = repo.UpdateMemory(ctx, agentID, "test_memory", "Test content")
	if err != nil {
		t.Fatalf("UpdateMemory failed: %v", err)
	}

	// Verify memory was created
	state, err := repo.FetchState(ctx, agentID)
	if err != nil {
		t.Fatalf("FetchState failed: %v", err)
	}

	found := false
	for _, mem := range state.CoreMemory {
		if mem.Name == "test_memory" && mem.Content == "Test content" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Memory block not found after creation")
	}

	// Test updating existing memory block
	err = repo.UpdateMemory(ctx, agentID, "test_memory", "Updated content")
	if err != nil {
		t.Fatalf("UpdateMemory failed: %v", err)
	}

	// Verify memory was updated
	state, err = repo.FetchState(ctx, agentID)
	if err != nil {
		t.Fatalf("FetchState failed: %v", err)
	}

	found = false
	for _, mem := range state.CoreMemory {
		if mem.Name == "test_memory" && mem.Content == "Updated content" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Memory block not updated correctly")
	}
}

func TestRepository_FetchState_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	driver, err := createTestDriver()
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	defer driver.Close(ctx)

	repo := NewRepository(driver)
	_, err = repo.FetchState(ctx, "non-existent-agent")
	if err == nil {
		t.Error("Expected error for non-existent agent")
	}
	if _, ok := err.(ErrAgentNotFound); !ok {
		t.Errorf("Expected ErrAgentNotFound, got %T", err)
	}
}

func createTestDriver() (neo4j.DriverWithContext, error) {
	uri := "bolt://localhost:7687"
	user := "neo4j"
	password := "password"

	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""))
	if err != nil {
		return nil, err
	}

	// Verify connection
	ctx := context.Background()
	if err := driver.VerifyConnectivity(ctx); err != nil {
		driver.Close(ctx)
		return nil, err
	}

	return driver, nil
}

