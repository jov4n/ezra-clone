# End-to-End Test Script for Windows PowerShell

Write-Host "Starting E2E Tests..." -ForegroundColor Green

# Check if services are running
Write-Host "`n1. Checking infrastructure..." -ForegroundColor Yellow
$neo4jRunning = docker ps --filter "name=ezra-neo4j" --format "{{.Names}}" | Select-String "ezra-neo4j"
$litellmRunning = docker ps --filter "name=ezra-litellm" --format "{{.Names}}" | Select-String "ezra-litellm"

if (-not $neo4jRunning) {
    Write-Host "ERROR: Neo4j is not running. Start with: docker-compose -f deploy/docker-compose.yml up -d" -ForegroundColor Red
    exit 1
}

if (-not $litellmRunning) {
    Write-Host "ERROR: LiteLLM is not running. Start with: docker-compose -f deploy/docker-compose.yml up -d" -ForegroundColor Red
    exit 1
}

Write-Host "✓ Infrastructure is running" -ForegroundColor Green

# Check if backend server is running
Write-Host "`n2. Checking backend server..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/health" -Method GET -TimeoutSec 2
    if ($response.StatusCode -eq 200) {
        Write-Host "✓ Backend server is running" -ForegroundColor Green
    }
} catch {
    Write-Host "ERROR: Backend server is not running. Start with: go run backend/cmd/server/main.go" -ForegroundColor Red
    exit 1
}

# Test agent state endpoint
Write-Host "`n3. Testing agent state endpoint..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/api/agent/Ezra/state" -Method GET
    if ($response.StatusCode -eq 200) {
        Write-Host "✓ Agent state endpoint works" -ForegroundColor Green
        $state = $response.Content | ConvertFrom-Json
        Write-Host "  Agent: $($state.identity.name)" -ForegroundColor Cyan
        Write-Host "  Memory blocks: $($state.core_memory.Count)" -ForegroundColor Cyan
    }
} catch {
    Write-Host "ERROR: Failed to fetch agent state. Make sure agent is seeded." -ForegroundColor Red
    Write-Host "  Run: go run backend/scripts/seed.go" -ForegroundColor Yellow
    exit 1
}

# Test chat endpoint
Write-Host "`n4. Testing chat endpoint..." -ForegroundColor Yellow
$chatBody = @{
    message = "Hello! What's your name?"
    user_id = "test-user"
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/api/agent/Ezra/chat" -Method POST -Body $chatBody -ContentType "application/json"
    if ($response.StatusCode -eq 200) {
        Write-Host "✓ Chat endpoint works" -ForegroundColor Green
        $result = $response.Content | ConvertFrom-Json
        Write-Host "  Response: $($result.content)" -ForegroundColor Cyan
    }
} catch {
    Write-Host "ERROR: Chat endpoint failed" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

# Test memory update endpoint
Write-Host "`n5. Testing memory update endpoint..." -ForegroundColor Yellow
$memoryBody = @{
    block_name = "test_memory"
    content = "This is a test memory block"
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/api/memory/Ezra/update" -Method POST -Body $memoryBody -ContentType "application/json"
    if ($response.StatusCode -eq 200) {
        Write-Host "✓ Memory update endpoint works" -ForegroundColor Green
    }
} catch {
    Write-Host "ERROR: Memory update endpoint failed" -ForegroundColor Red
    exit 1
}

Write-Host "`n✓ All E2E tests passed!" -ForegroundColor Green

