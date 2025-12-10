# Infrastructure - Docker Compose

Docker Compose configuration for running Neo4j and LiteLLM.

## Services

### Neo4j
- **Image**: `neo4j:5.19.0-community`
- **Ports**: 
  - 7474 (HTTP/Browser)
  - 7687 (Bolt Protocol)
- **Default Credentials**: `neo4j/password`
- **Data Persistence**: Docker volumes

### LiteLLM
- **Image**: `ghcr.io/berriai/litellm:main-latest`
- **Port**: 4000
- **Model**: `openrouter/anthropic/claude-3.5-sonnet`
- **API Base**: `https://openrouter.ai/api/v1`

## Usage

### Start Services
```bash
docker-compose up -d
```

### Stop Services
```bash
docker-compose down
```

### View Logs
```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f neo4j
docker-compose logs -f litellm
```

### Access Neo4j Browser
Open `http://localhost:7474` in your browser.

Default credentials: `neo4j/password`

## Environment Variables

Create `deploy/.env` from `deploy/.env.example`:

```bash
# Windows (PowerShell)
Copy-Item deploy\.env.example deploy\.env

# Linux/Mac
cp deploy/.env.example deploy/.env
```

Then edit `deploy/.env` and set:
```
OPENROUTER_API_KEY=your_key_here
```

**Important:** The `OPENROUTER_API_KEY` is required for LiteLLM to function. Get your API key from [OpenRouter.ai](https://openrouter.ai/).

## Volumes

Data is persisted in Docker volumes:
- `neo4j-data`: Database files
- `neo4j-logs`: Log files
- `neo4j-import`: Import directory
- `neo4j-plugins`: Plugin directory

## Health Checks

Both services include health checks. Check status:
```bash
docker-compose ps
```

