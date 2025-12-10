# Frontend - Agent Development Environment (ADE)

A Next.js 14 dashboard for visualizing and editing the AI agent's memory.

## Features

- **Chat Interface**: Interact with the agent in real-time
- **State Inspector**: View the agent's complete context window as JSON
- **Memory Editor**: Manually edit memory blocks
- **Run Eval**: Execute pre-defined test scenarios

## Development

```bash
# Install dependencies
npm install

# Run development server
npm run dev

# Build for production
npm run build

# Start production server
npm start
```

## Configuration

The frontend proxies API requests to `http://localhost:8080` by default. This is configured in `next.config.js`.

To change the API URL:
- Modify the `rewrites` configuration in `next.config.js`
- Or set `NEXT_PUBLIC_API_URL` environment variable (if supported by your setup)

## Environment Variables

The frontend doesn't require any environment variables by default, as it uses Next.js rewrites to proxy API requests. If you need to configure a different API URL, you can:

1. Modify `next.config.js` directly
2. Create a `.env.local` file (not tracked by git) with custom configuration

## Components

- `ChatInterface`: Real-time chat with the agent
- `StateInspector`: JSON tree viewer for agent state
- `MemoryEditor`: Edit memory blocks
- `ContextVisualizer`: Combined view of chat and state

## Styling

Uses TailwindCSS with dark mode support.

