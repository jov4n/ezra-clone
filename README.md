# Ezra Clone - Stateful AI Agent Platform

A complete monorepo containing a **Stateful AI Agent Platform** with a Memory Operating System, Discord bot integration, and an Agent Development Environment (ADE) dashboard. This platform enables you to build AI agents that maintain persistent memory, learn from interactions, and can be deployed across multiple platforms.

## What is This?

Ezra Clone is a production-ready AI agent platform that combines:

- **Persistent Memory**: Agents remember conversations, facts, and user preferences across sessions
- **Graph-Based Knowledge**: Uses Neo4j to store and query relationships between users, topics, facts, and memories
- **Multi-Platform Support**: Works as a Discord bot and via a web dashboard
- **Developer Tools**: Built-in dashboard for visualizing and editing agent memory in real-time
- **Extensible Tool System**: Agents can use web search, GitHub integration, and custom tools

Think of it as a "Memory Operating System" for AI agents - the backend manages state and reasoning, while the Discord bot and frontend dashboard provide different interfaces to interact with the same agent.

## Architecture

The platform consists of three independent components that share state through Neo4j:

### 1. **HTTP API Server**
A GoLang service that provides REST API endpoints for the frontend:
- Connects directly to Neo4j for agent state and memory
- Handles reasoning via LiteLLM/Claude
- Executes tools and actions
- Processes agent logic independently
- Provides REST API endpoints for the web dashboard

**Location**: `backend/cmd/server/`

**Key Features**:
- Independent agent orchestrator instance
- Direct Neo4j database access
- No dependency on Discord bot
- Can run standalone for web-only deployments

### 2. **Discord Bot**
A GoLang process that handles Discord interactions:
- Connects directly to Neo4j (same database as server)
- Listens for Discord messages (mentions and DMs)
- Processes messages with its own agent orchestrator
- Sends agent responses back to Discord
- Handles Discord-specific features (embeds, long messages, etc.)
- Supports language preferences and user context

**Location**: `backend/cmd/bot/`

**Key Features**:
- Independent agent orchestrator instance
- Direct Neo4j database access
- No dependency on HTTP server
- Can run standalone for Discord-only deployments
- Shares agent state with server through Neo4j

### 3. **The Dashboard (ADE)**
A Next.js frontend for developers to interact with and debug agents:
- Real-time chat interface with the agent (via HTTP API)
- Visualize agent's complete context window
- Edit memory blocks directly
- View tool calls and agent reasoning
- Monitor agent state changes

**Location**: `frontend/`

### Shared Components

Both the HTTP server and Discord bot use the same shared codebase:
- **Agent Orchestrator** (`internal/agent/`) - Core reasoning and tool execution
- **Graph Repository** (`internal/graph/`) - Neo4j operations
- **LLM Adapter** (`internal/adapter/`) - LiteLLM/Claude communication
- **Tools** (`internal/tools/`) - Tool executors (web, GitHub, memory, etc.)

### State Coordination

- **Neo4j Database**: Shared state storage for all agent data
  - Agent memory blocks
  - User context and facts
  - Conversation history
  - Topics and relationships
- **No Direct Communication**: Server and bot don't communicate with each other
- **Shared State**: Both read/write to the same Neo4j database, ensuring consistency

## Features

### Discord Bot Features

- **Smart Message Handling**
  - Responds to mentions (`@bot`) and direct messages
  - Automatically ignores messages not directed at it (lurker mode)
  - Handles long messages by splitting into chunks

- **Language Preferences**
  - Automatically detects and stores user language preferences
  - Supports multiple languages (French, Spanish, German, Italian, Portuguese, Japanese, Chinese, Korean, Russian, Pig Latin)
  - Responds in user's preferred language when set

- **Rich Responses**
  - Supports Discord embeds for formatted responses
  - Handles code blocks and formatting
  - Splits long messages intelligently

- **User Context**
  - Remembers users across conversations
  - Tracks user interests and preferences
  - Builds relationships between users and topics

- **Discord-Specific Tools**
  - Read message history from channels
  - Get user and channel information
  - Search messages in Discord

### Frontend Dashboard Features

- **Chat Interface**
  - Real-time conversation with the agent
  - Shows tool calls and execution results
  - Displays response times and token usage
  - Auto-scrolling message history

- **State Inspector**
  - View complete agent context window as JSON
  - Real-time updates (polls every 2 seconds)
  - Expandable/collapsible JSON tree
  - See all memory blocks, facts, and user context

- **Memory Editor**
  - Edit core memory blocks directly
  - Create new memory blocks
  - Update agent identity, instructions, and persona
  - Changes take effect immediately

- **Tools Sidebar**
  - Visual reference of all available tools
  - Organized by category (Memory, Knowledge, Topics, Web, GitHub, Discord)
  - Shows tool descriptions and capabilities

- **Core Memory Sidebar**
  - View all core memory blocks
  - Quick access to agent's identity and instructions
  - Real-time updates when memory changes

## Tech Stack

- **Backend**: Go 1.22+, gin-gonic (HTTP), discordgo, neo4j-go-driver
- **AI Layer**: OpenAI Protocol (via go-openai) → LiteLLM Proxy → OpenRouter (Claude 3.5 Sonnet)
- **Database**: Neo4j 5.19 (Graph DB) with vector indexing
- **Frontend**: Next.js 14 (App Router), TypeScript, TailwindCSS
- **Infrastructure**: Docker Compose
- **Logging**: Zap (structured logging)

## Prerequisites

- **Go 1.22+** - [Download](https://go.dev/dl/)
- **Node.js 18+** - [Download](https://nodejs.org/)
- **Docker & Docker Compose** - [Download](https://www.docker.com/products/docker-desktop)
- **OpenRouter API Key** - [Get one here](https://openrouter.ai/) (free tier available)
- **Discord Bot Token** (optional) - [Create a bot](https://discord.com/developers/applications)

## Quick Start

### 1. Clone and Setup Environment

```bash
# Clone the repository
git clone https://github.com/jov4n/ezra-clone.git
cd ezra-clone

# Create environment files
# Windows (PowerShell)
Copy-Item .env.example .env
Copy-Item deploy\.env.example deploy\.env

# Linux/Mac
cp .env.example .env
cp deploy/.env.example deploy/.env
```

### 2. Configure Environment Variables

Edit `.env` (root directory):
```env
# Required
OPENROUTER_API_KEY=your_openrouter_api_key_here

# Optional (defaults shown)
PORT=8080
ENV=development
NEO4J_URI=bolt://localhost:7687
NEO4J_USER=neo4j
NEO4J_PASSWORD=password
LITELLM_URL=http://localhost:4000
MODEL_ID=openrouter/anthropic/claude-3.5-sonnet

# Discord Bot (only if using Discord bot)
DISCORD_BOT_TOKEN=your_discord_bot_token_here
```

Edit `deploy/.env`:
```env
OPENROUTER_API_KEY=your_openrouter_api_key_here
```

### 3. Start Infrastructure

```bash
cd deploy
docker-compose up -d
```

This starts:
- **Neo4j** on ports 7474 (HTTP/Browser) and 7687 (Bolt)
- **LiteLLM** on port 4000

Verify services are running:
```bash
docker ps
```

Access Neo4j Browser at `http://localhost:7474` (default: `neo4j/password`)

### 4. Seed Database

Create the default "Ezra" agent with initial memory:

```bash
go run backend/scripts/seed.go
```

**Alternative**: To completely reset and re-seed:
```bash
go run backend/scripts/reset_and_seed.go
```

⚠️ **Warning**: The reset script deletes all existing data!

### 5. Start Backend Services

**HTTP API Server** (required for frontend):
```bash
go run backend/cmd/server/main.go
```

Server runs on `http://localhost:8080`

**Discord Bot** (optional, can run independently):
```bash
go run backend/cmd/bot/main.go
```

The bot will connect to Discord and start listening for messages.

**Note**: The server and bot are independent processes. You can run:
- Only the server (for web dashboard)
- Only the bot (for Discord functionality)
- Both (for full functionality)

Both processes share the same Neo4j database, so agent state is synchronized between them.

### 6. Start Frontend

```bash
cd frontend
npm install
npm run dev
```

Frontend runs on `http://localhost:3000`

## Project Structure

```
ezra-clone/
├── backend/                    # GoLang Backend
│   ├── cmd/
│   │   ├── server/             # HTTP API Server
│   │   └── bot/                # Discord Bot Process
│   ├── internal/
│   │   ├── adapter/            # LLM Client (OpenAI/LiteLLM)
│   │   ├── agent/              # Core Agent Logic & Orchestrator
│   │   ├── graph/              # Neo4j Repository & Cypher Queries
│   │   ├── state/              # Domain Models
│   │   └── tools/              # Tool Executors
│   ├── pkg/
│   │   ├── config/             # Configuration Management
│   │   └── logger/             # Logging Utilities
│   └── scripts/                # Database Scripts
│       ├── seed.go             # Seed database
│       └── reset_and_seed.go   # Reset and seed
├── frontend/                   # Next.js Frontend
│   ├── app/                    # App Router Pages
│   ├── components/             # React Components
│   │   ├── ChatArea.tsx        # Main chat interface
│   │   ├── ChatInterface.tsx  # Chat component
│   │   ├── StateInspector.tsx # JSON state viewer
│   │   ├── MemoryEditor.tsx   # Memory block editor
│   │   ├── CoreMemorySidebar.tsx # Memory sidebar
│   │   └── ToolsSidebar.tsx   # Tools reference
│   └── lib/                    # API Clients
├── deploy/                     # Docker Infrastructure
│   ├── docker-compose.yml      # Service definitions
│   └── litellm_config.yaml     # LiteLLM configuration
└── scripts/                    # Utility Scripts
```

## API Endpoints

### Agent State

**GET** `/api/agent/:id/state`
Returns the complete context window for an agent.

Response:
```json
{
  "core_memory": [
    {
      "name": "identity",
      "content": "I am Ezra, a helpful AI assistant."
    }
  ],
  "archival_memory": [],
  "facts": [],
  "topics": [],
  "users": []
}
```

### Chat

**POST** `/api/agent/:id/chat`
Sends a message to the agent.

Request:
```json
{
  "message": "Hello! What's your name?",
  "user_id": "user123"
}
```

Response:
```json
{
  "content": "Hello! I'm Ezra, a helpful AI assistant.",
  "tool_calls": [
    {
      "name": "core_memory_search",
      "arguments": {},
      "result": "..."
    }
  ],
  "ignored": false
}
```

### Memory Update

**POST** `/api/memory/:id/update`
Manually updates a memory block.

Request:
```json
{
  "block_name": "identity",
  "content": "I am Jarvis, a helpful assistant."
}
```

## Agent Capabilities

The agent has access to a comprehensive set of tools organized into categories:

### Memory Tools
- `core_memory_insert` - Create new memory blocks
- `core_memory_replace` - Update existing memory blocks
- `archival_memory_insert` - Archive information for long-term storage
- `archival_memory_search` - Search archived memories
- `memory_search` - Search across all memories

### Knowledge Management
- `create_fact` - Store facts and link them to topics/users
- `search_facts` - Search for facts about specific topics
- `get_user_context` - Get comprehensive information about a user

### Topic Management
- `create_topic` - Create topics to organize knowledge
- `link_topics` - Create relationships between topics
- `find_related_topics` - Find topics related to a given topic
- `link_user_to_topic` - Record a user's interest in a topic

### Conversation Tools
- `get_conversation_history` - Retrieve recent messages
- `send_message` - Send a response to the user

### Discord Tools (Discord bot only)
- `discord_read_history` - Read message history from a Discord channel
- `discord_get_user_info` - Get information about a Discord user
- `discord_get_channel_info` - Get information about a Discord channel
- `discord_search_messages` - Search messages in Discord

### Web & External Tools
- `web_search` - Search the web for information
- `fetch_webpage` - Read content from a URL
- `github_repo_info` - Get information about a GitHub repository
- `github_search` - Search GitHub for repositories, code, or issues
- `github_read_file` - Read a file from a GitHub repository
- `github_list_org_repos` - List an organization's repos

### Personality Tools
- `mimic_personality` - Analyze and mimic a user's communication style
- `revert_personality` - Stop mimicking and return to normal personality
- `analyze_user_style` - Analyze a user's communication style

## Usage Examples

### Discord Bot

1. **Mention the bot** in a channel:
   ```
   @Ezra What's your name?
   ```

2. **Send a direct message** to the bot

3. **Set language preferences**:
   ```
   @Ezra never forget that @user is from france, speak french when he talks to you
   ```

4. **Ask about users**:
   ```
   @Ezra what do you know about @user?
   ```

### Frontend Dashboard

1. **Open** `http://localhost:3000`
2. **Chat** with the agent in the center panel
3. **View state** - See the agent's complete context in the right sidebar
4. **Edit memory** - Click on memory blocks to edit them directly
5. **Monitor tools** - See tool calls and results in the chat interface

## Testing

### Backend Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run specific package tests
go test ./backend/internal/agent/...
```

### Integration Tests

Requires running Neo4j:
```bash
# Start Neo4j
cd deploy
docker-compose up -d neo4j

# Run tests
go test ./... -v
```

## Development

### Backend Development

```bash
# Install dependencies
go mod download

# Run server with hot reload (requires air or similar)
air

# Build binaries
go build -o bin/server backend/cmd/server/main.go
go build -o bin/bot backend/cmd/bot/main.go
```

### Frontend Development

```bash
cd frontend

# Install dependencies
npm install

# Run development server
npm run dev

# Build for production
npm run build

# Start production server
npm start
```

### Database Scripts

```bash
# Seed database
go run backend/scripts/seed.go

# Reset and seed (⚠️ deletes all data)
go run backend/scripts/reset_and_seed.go

# Run migration (if needed)
go run backend/scripts/migrate_enhanced_schema.go
```

## Troubleshooting

### Neo4j Connection Issues
- Ensure Neo4j is running: `docker ps`
- Check Neo4j logs: `docker logs ezra-neo4j`
- Verify credentials in `.env`
- Access Neo4j browser at `http://localhost:7474` (default: `neo4j/password`)
- Test connection: `cypher-shell -u neo4j -p password`

### LiteLLM Issues
- Check LiteLLM logs: `docker logs ezra-litellm`
- Verify `OPENROUTER_API_KEY` is set in `deploy/.env`
- Test LiteLLM health: `curl http://localhost:4000/health`
- Ensure OpenRouter API key is valid and has credits
- Check model availability on OpenRouter

### Frontend API Errors
- Ensure backend server is running on port 8080
- Check browser console for CORS errors
- Verify API URL in `frontend/lib/api.ts`
- Check backend logs for API errors
- Ensure Next.js rewrites are working (check `next.config.js`)

### Backend Issues
- Verify all environment variables are set correctly
- Check Go version: `go version` (requires 1.22+)
- Run tests: `go test ./...`
- Check logs for detailed error messages
- Verify Neo4j and LiteLLM are accessible

### Discord Bot Issues
- Verify `DISCORD_BOT_TOKEN` is set in `.env`
- Check bot has proper intents enabled in Discord Developer Portal
- Ensure bot has permissions in the server/channel
- Check bot logs for connection errors
- Verify bot is online in Discord

## Additional Documentation

- [Backend README](backend/README.md) - Backend-specific documentation
- [Frontend README](frontend/README.md) - Frontend-specific documentation
- [Deploy README](deploy/README.md) - Docker and infrastructure setup

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT

## Acknowledgments

- Built with [Claude 3.5 Sonnet](https://www.anthropic.com/claude) via [OpenRouter](https://openrouter.ai/)
- Uses [LiteLLM](https://github.com/BerriAI/litellm) for LLM proxy
- Graph database powered by [Neo4j](https://neo4j.com/)
- Frontend built with [Next.js](https://nextjs.org/) and [TailwindCSS](https://tailwindcss.com/)

---

**Made for building better AI agents**
