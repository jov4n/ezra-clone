# Backend - Ezra Clone

The backend is a GoLang monolith that implements the "Memory Operating System" for the AI agent.

## Architecture

- **State Models** (`internal/state/`): Domain models representing the agent's memory structure
- **Graph Repository** (`internal/graph/`): Neo4j operations for persistence
- **LLM Adapter** (`internal/adapter/`): Communication with LiteLLM/Claude
- **Agent Orchestrator** (`internal/agent/`): Core reasoning loop and tool execution
- **HTTP Server** (`cmd/server/`): REST API for the frontend
- **Discord Bot** (`cmd/bot/`): Discord integration

## Running

### HTTP Server
```bash
go run cmd/server/main.go
```

### Discord Bot
```bash
go run cmd/bot/main.go
```

## Configuration

All configuration is loaded from environment variables. See the root `.env.example` for all available options.

Required environment variables:
- `NEO4J_URI` - Neo4j connection URI (default: `bolt://localhost:7687`)
- `NEO4J_USER` - Neo4j username (default: `neo4j`)
- `NEO4J_PASSWORD` - Neo4j password (default: `password`)
- `LITELLM_URL` - LiteLLM proxy URL (default: `http://localhost:4000`)
- `MODEL_ID` - Model identifier (default: `openrouter/anthropic/claude-3.5-sonnet`)
- `OPENROUTER_API_KEY` - OpenRouter API key (required)

Optional:
- `PORT` - Server port (default: `8080`)
- `ENV` - Environment mode (default: `development`)
- `DISCORD_BOT_TOKEN` - Discord bot token (required only for Discord bot)

## Testing

```bash
# Unit tests
go test ./internal/...

# Integration tests (requires Neo4j)
go test ./... -v
```

## Building

```bash
# Build server
go build -o bin/server cmd/server/main.go

# Build bot
go build -o bin/bot cmd/bot/main.go
```

