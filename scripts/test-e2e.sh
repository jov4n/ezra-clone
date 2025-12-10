#!/bin/bash

# End-to-End Test Script for Unix/Linux

echo "Starting E2E Tests..."

# Check if services are running
echo ""
echo "1. Checking infrastructure..."
if ! docker ps --filter "name=ezra-neo4j" --format "{{.Names}}" | grep -q "ezra-neo4j"; then
    echo "ERROR: Neo4j is not running. Start with: docker-compose -f deploy/docker-compose.yml up -d"
    exit 1
fi

if ! docker ps --filter "name=ezra-litellm" --format "{{.Names}}" | grep -q "ezra-litellm"; then
    echo "ERROR: LiteLLM is not running. Start with: docker-compose -f deploy/docker-compose.yml up -d"
    exit 1
fi

echo "✓ Infrastructure is running"

# Check if backend server is running
echo ""
echo "2. Checking backend server..."
if ! curl -s http://localhost:8080/health > /dev/null; then
    echo "ERROR: Backend server is not running. Start with: go run backend/cmd/server/main.go"
    exit 1
fi

echo "✓ Backend server is running"

# Test agent state endpoint
echo ""
echo "3. Testing agent state endpoint..."
STATE_RESPONSE=$(curl -s http://localhost:8080/api/agent/Ezra/state)
if [ $? -eq 0 ]; then
    echo "✓ Agent state endpoint works"
    AGENT_NAME=$(echo $STATE_RESPONSE | jq -r '.identity.name // "unknown"')
    MEMORY_COUNT=$(echo $STATE_RESPONSE | jq -r '.core_memory | length')
    echo "  Agent: $AGENT_NAME"
    echo "  Memory blocks: $MEMORY_COUNT"
else
    echo "ERROR: Failed to fetch agent state. Make sure agent is seeded."
    echo "  Run: go run backend/scripts/seed.go"
    exit 1
fi

# Test chat endpoint
echo ""
echo "4. Testing chat endpoint..."
CHAT_RESPONSE=$(curl -s -X POST http://localhost:8080/api/agent/Ezra/chat \
    -H "Content-Type: application/json" \
    -d '{"message": "Hello! What'\''s your name?", "user_id": "test-user"}')

if [ $? -eq 0 ]; then
    echo "✓ Chat endpoint works"
    CONTENT=$(echo $CHAT_RESPONSE | jq -r '.content // "no content"')
    echo "  Response: $CONTENT"
else
    echo "ERROR: Chat endpoint failed"
    exit 1
fi

# Test memory update endpoint
echo ""
echo "5. Testing memory update endpoint..."
UPDATE_RESPONSE=$(curl -s -X POST http://localhost:8080/api/memory/Ezra/update \
    -H "Content-Type: application/json" \
    -d '{"block_name": "test_memory", "content": "This is a test memory block"}')

if [ $? -eq 0 ]; then
    echo "✓ Memory update endpoint works"
else
    echo "ERROR: Memory update endpoint failed"
    exit 1
fi

echo ""
echo "✓ All E2E tests passed!"

