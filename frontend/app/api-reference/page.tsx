'use client';

import { useState } from 'react';
import { Search, Copy, Check, Code, BookOpen } from 'lucide-react';

interface Endpoint {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE';
  path: string;
  description: string;
  parameters?: {
    path?: Record<string, string>;
    query?: Record<string, string>;
    body?: Record<string, any>;
  };
  response: any;
  example?: {
    request?: string;
    response?: string;
  };
}

const endpoints: Endpoint[] = [
  {
    method: 'GET',
    path: '/health',
    description: 'Health check endpoint to verify server status',
    response: { status: 'ok' },
    example: {
      response: JSON.stringify({ status: 'ok' }, null, 2),
    },
  },
  {
    method: 'GET',
    path: '/api/agents',
    description: 'List all available agents',
    response: [
      {
        id: 'string',
        name: 'string',
        created_at: 'string (ISO 8601)',
      },
    ],
    example: {
      response: JSON.stringify(
        [
          { id: 'Ezra', name: 'Ezra', created_at: '2024-01-01T00:00:00Z' },
        ],
        null,
        2
      ),
    },
  },
  {
    method: 'POST',
    path: '/api/agents',
    description: 'Create a new agent',
    parameters: {
      body: {
        name: 'string (required)',
        model: 'string (optional)',
        system_instructions: 'string (optional)',
      },
    },
    response: {
      id: 'string',
      name: 'string',
    },
    example: {
      request: JSON.stringify(
        {
          name: 'MyAgent',
          model: 'openrouter/anthropic/claude-3.5-sonnet',
          system_instructions: 'You are a helpful assistant.',
        },
        null,
        2
      ),
      response: JSON.stringify(
        { id: 'MyAgent', name: 'MyAgent' },
        null,
        2
      ),
    },
  },
  {
    method: 'GET',
    path: '/api/agent/:id/state',
    description: 'Get the current state of an agent (context window, memory, etc.)',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
    },
    response: {
      identity: {
        name: 'string',
        personality: 'string',
        capabilities: ['string'],
      },
      core_memory: [
        {
          name: 'string',
          content: 'string',
          updated_at: 'string',
        },
      ],
      archival_refs: [
        {
          summary: 'string',
          timestamp: 'string',
          relevance_score: 'number',
        },
      ],
      user_context: {},
    },
  },
  {
    method: 'GET',
    path: '/api/agent/:id/config',
    description: 'Get agent configuration (model, system instructions)',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
    },
    response: {
      model: 'string',
      system_instructions: 'string',
    },
    example: {
      response: JSON.stringify(
        {
          model: 'openrouter/anthropic/claude-3.5-sonnet',
          system_instructions: 'You are a helpful assistant.',
        },
        null,
        2
      ),
    },
  },
  {
    method: 'PUT',
    path: '/api/agent/:id/config',
    description: 'Update agent configuration',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
      body: {
        model: 'string (optional)',
        system_instructions: 'string (optional)',
      },
    },
    response: { status: 'updated' },
    example: {
      request: JSON.stringify(
        {
          model: 'openrouter/anthropic/claude-3-opus',
          system_instructions: 'Updated instructions',
        },
        null,
        2
      ),
    },
  },
  {
    method: 'GET',
    path: '/api/agent/:id/tools',
    description: 'Get all tools available to an agent',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
    },
    response: [
      {
        type: 'function',
        function: {
          name: 'string',
          description: 'string',
          parameters: {},
        },
      },
    ],
  },
  {
    method: 'GET',
    path: '/api/agent/:id/context',
    description: 'Get context window statistics (token usage)',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
    },
    response: {
      used_tokens: 'number',
      total_tokens: 'number',
    },
    example: {
      response: JSON.stringify(
        { used_tokens: 1234, total_tokens: 8192 },
        null,
        2
      ),
    },
  },
  {
    method: 'POST',
    path: '/api/agent/:id/chat',
    description: 'Send a chat message to an agent',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
      body: {
        message: 'string (required)',
        user_id: 'string (required)',
      },
    },
    response: {
      content: 'string',
      tool_calls: [
        {
          id: 'string',
          name: 'string',
          arguments: {},
        },
      ],
      ignored: 'boolean',
    },
    example: {
      request: JSON.stringify(
        {
          message: 'Hello, how are you?',
          user_id: 'developer',
        },
        null,
        2
      ),
      response: JSON.stringify(
        {
          content: "I'm doing well, thank you!",
          tool_calls: [],
          ignored: false,
        },
        null,
        2
      ),
    },
  },
  {
    method: 'GET',
    path: '/api/agent/:id/archival-memories',
    description: 'Get all archival memories for an agent',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
    },
    response: [
      {
        id: 'string',
        summary: 'string',
        content: 'string',
        timestamp: 'string',
        relevance_score: 'number',
      },
    ],
  },
  {
    method: 'POST',
    path: '/api/agent/:id/archival-memories',
    description: 'Create a new archival memory',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
      body: {
        summary: 'string (required)',
        content: 'string (required)',
        timestamp: 'string (optional, ISO 8601)',
        relevance_score: 'number (optional, 0-1)',
      },
    },
    response: { status: 'created' },
    example: {
      request: JSON.stringify(
        {
          summary: 'User preference',
          content: 'User prefers dark mode',
          relevance_score: 0.8,
        },
        null,
        2
      ),
    },
  },
  {
    method: 'DELETE',
    path: '/api/agent/:id/archival-memories/:memoryId',
    description: 'Delete an archival memory',
    parameters: {
      path: {
        id: 'string - Agent ID',
        memoryId: 'string - Memory ID',
      },
    },
    response: { status: 'deleted' },
  },
  {
    method: 'POST',
    path: '/api/memory/:id/update',
    description: 'Update a memory block (PERSONA, HUMAN, or custom)',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
      body: {
        block_name: 'string (required)',
        content: 'string (required)',
      },
    },
    response: { status: 'updated' },
    example: {
      request: JSON.stringify(
        {
          block_name: 'persona',
          content: 'You are a helpful AI assistant.',
        },
        null,
        2
      ),
    },
  },
  {
    method: 'DELETE',
    path: '/api/memory/:id/block/:blockName',
    description: 'Delete a memory block',
    parameters: {
      path: {
        id: 'string - Agent ID',
        blockName: 'string - Memory block name',
      },
    },
    response: { status: 'deleted' },
  },
  {
    method: 'GET',
    path: '/api/agent/:id/facts',
    description: 'Get all facts known by an agent',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
    },
    response: [
      {
        id: 'string',
        content: 'string',
        source: 'string',
        confidence: 'number',
        created_at: 'string',
      },
    ],
  },
  {
    method: 'GET',
    path: '/api/agent/:id/topics',
    description: 'Get all topics related to an agent',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
    },
    response: [
      {
        id: 'string',
        name: 'string',
        description: 'string',
      },
    ],
  },
  {
    method: 'GET',
    path: '/api/agent/:id/messages',
    description: 'Get all messages sent by or to an agent',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
      query: {
        limit: 'number (optional, default: 100)',
      },
    },
    response: [
      {
        id: 'string',
        content: 'string',
        role: 'string (user|agent)',
        platform: 'string (discord|web)',
        timestamp: 'string',
      },
    ],
  },
  {
    method: 'GET',
    path: '/api/agent/:id/conversations',
    description: 'Get all conversations for an agent',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
      query: {
        limit: 'number (optional, default: 50)',
      },
    },
    response: [
      {
        id: 'string',
        channel_id: 'string',
        platform: 'string',
        started_at: 'string',
      },
    ],
  },
  {
    method: 'GET',
    path: '/api/agent/:id/users',
    description: 'Get all users that have interacted with an agent',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
    },
    response: [
      {
        id: 'string',
        discord_id: 'string',
        discord_username: 'string',
        web_id: 'string',
        preferred_language: 'string',
        first_seen: 'string',
        last_seen: 'string',
      },
    ],
  },
  {
    method: 'GET',
    path: '/api/agent/:id/conversation-history',
    description: 'Get conversation history for a specific channel',
    parameters: {
      path: {
        id: 'string - Agent ID',
      },
      query: {
        channel_id: 'string (optional)',
        limit: 'number (optional, default: 20)',
      },
    },
    response: {
      messages: [
        {
          id: 'string',
          content: 'string',
          role: 'string',
          platform: 'string',
          timestamp: 'string',
        },
      ],
      channel_id: 'string',
    },
  },
];

const methodColors = {
  GET: 'bg-blue-600',
  POST: 'bg-green-600',
  PUT: 'bg-yellow-600',
  DELETE: 'bg-red-600',
};

export default function APIReferencePage() {
  const [searchQuery, setSearchQuery] = useState('');
  const [copiedEndpoint, setCopiedEndpoint] = useState<string | null>(null);

  const filteredEndpoints = endpoints.filter(
    (endpoint) =>
      endpoint.path.toLowerCase().includes(searchQuery.toLowerCase()) ||
      endpoint.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
      endpoint.method.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const copyToClipboard = (text: string, endpointPath: string) => {
    navigator.clipboard.writeText(text);
    setCopiedEndpoint(endpointPath);
    setTimeout(() => setCopiedEndpoint(null), 2000);
  };

  const getMethodColor = (method: string) => {
    return methodColors[method as keyof typeof methodColors] || 'bg-gray-600';
  };

  return (
    <div className="min-h-screen bg-gray-950 text-white">
      {/* Header */}
      <header className="bg-gray-900 border-b border-gray-800 px-6 py-4">
        <div className="max-w-7xl mx-auto flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <BookOpen size={24} className="text-blue-400" />
            <h1 className="text-2xl font-bold">API Reference</h1>
          </div>
          <div className="text-sm text-gray-400">
            Base URL: <code className="text-blue-400">http://localhost:8080</code>
          </div>
        </div>
      </header>

      <div className="max-w-7xl mx-auto px-6 py-8">
        {/* Search */}
        <div className="mb-8">
          <div className="relative">
            <Search
              size={20}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
            />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search endpoints..."
              className="w-full pl-10 pr-4 py-3 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
        </div>

        {/* Endpoints List */}
        <div className="space-y-6">
          {filteredEndpoints.map((endpoint, idx) => (
            <div
              key={idx}
              className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden"
            >
              {/* Endpoint Header */}
              <div className="px-6 py-4 border-b border-gray-800 flex items-center justify-between">
                <div className="flex items-center space-x-4 flex-1">
                  <span
                    className={`px-3 py-1 rounded text-sm font-semibold text-white ${getMethodColor(
                      endpoint.method
                    )}`}
                  >
                    {endpoint.method}
                  </span>
                  <code className="text-lg font-mono text-blue-400">
                    {endpoint.path}
                  </code>
                </div>
                <button
                  onClick={() => copyToClipboard(endpoint.path, endpoint.path)}
                  className="p-2 text-gray-400 hover:text-white transition-colors"
                  title="Copy endpoint path"
                >
                  {copiedEndpoint === endpoint.path ? (
                    <Check size={18} className="text-green-400" />
                  ) : (
                    <Copy size={18} />
                  )}
                </button>
              </div>

              {/* Description */}
              <div className="px-6 py-4">
                <p className="text-gray-300 mb-4">{endpoint.description}</p>

                {/* Parameters */}
                {endpoint.parameters && (
                  <div className="mb-4">
                    <h3 className="text-sm font-semibold text-gray-400 mb-2 uppercase tracking-wide">
                      Parameters
                    </h3>
                    <div className="space-y-3">
                      {endpoint.parameters.path && (
                        <div>
                          <div className="text-xs text-gray-500 mb-1">Path Parameters:</div>
                          <div className="bg-gray-800 rounded p-3">
                            {Object.entries(endpoint.parameters.path).map(
                              ([key, value]) => (
                                <div key={key} className="text-sm font-mono">
                                  <span className="text-blue-400">{key}</span>:{' '}
                                  <span className="text-gray-300">{value}</span>
                                </div>
                              )
                            )}
                          </div>
                        </div>
                      )}
                      {endpoint.parameters.query && (
                        <div>
                          <div className="text-xs text-gray-500 mb-1">Query Parameters:</div>
                          <div className="bg-gray-800 rounded p-3">
                            {Object.entries(endpoint.parameters.query).map(
                              ([key, value]) => (
                                <div key={key} className="text-sm font-mono">
                                  <span className="text-blue-400">{key}</span>:{' '}
                                  <span className="text-gray-300">{value}</span>
                                </div>
                              )
                            )}
                          </div>
                        </div>
                      )}
                      {endpoint.parameters.body && (
                        <div>
                          <div className="text-xs text-gray-500 mb-1">Request Body:</div>
                          <div className="bg-gray-800 rounded p-3">
                            {Object.entries(endpoint.parameters.body).map(
                              ([key, value]) => (
                                <div key={key} className="text-sm font-mono">
                                  <span className="text-blue-400">{key}</span>:{' '}
                                  <span className="text-gray-300">{value}</span>
                                </div>
                              )
                            )}
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                )}

                {/* Response */}
                <div className="mb-4">
                  <h3 className="text-sm font-semibold text-gray-400 mb-2 uppercase tracking-wide">
                    Response
                  </h3>
                  <div className="bg-gray-800 rounded p-3">
                    <pre className="text-xs text-gray-300 overflow-x-auto">
                      {typeof endpoint.response === 'string'
                        ? endpoint.response
                        : JSON.stringify(endpoint.response, null, 2)}
                    </pre>
                  </div>
                </div>

                {/* Examples */}
                {endpoint.example && (
                  <div className="space-y-3">
                    {endpoint.example.request && (
                      <div>
                        <div className="flex items-center justify-between mb-2">
                          <h3 className="text-sm font-semibold text-gray-400 uppercase tracking-wide">
                            Example Request
                          </h3>
                          <button
                            onClick={() =>
                              copyToClipboard(
                                endpoint.example!.request!,
                                endpoint.path + '-request'
                              )
                            }
                            className="p-1 text-gray-400 hover:text-white transition-colors"
                            title="Copy request example"
                          >
                            {copiedEndpoint === endpoint.path + '-request' ? (
                              <Check size={14} className="text-green-400" />
                            ) : (
                              <Copy size={14} />
                            )}
                          </button>
                        </div>
                        <div className="bg-gray-800 rounded p-3">
                          <pre className="text-xs text-gray-300 overflow-x-auto">
                            {endpoint.example.request}
                          </pre>
                        </div>
                      </div>
                    )}
                    {endpoint.example.response && (
                      <div>
                        <div className="flex items-center justify-between mb-2">
                          <h3 className="text-sm font-semibold text-gray-400 uppercase tracking-wide">
                            Example Response
                          </h3>
                          <button
                            onClick={() =>
                              copyToClipboard(
                                endpoint.example!.response!,
                                endpoint.path + '-response'
                              )
                            }
                            className="p-1 text-gray-400 hover:text-white transition-colors"
                            title="Copy response example"
                          >
                            {copiedEndpoint === endpoint.path + '-response' ? (
                              <Check size={14} className="text-green-400" />
                            ) : (
                              <Copy size={14} />
                            )}
                          </button>
                        </div>
                        <div className="bg-gray-800 rounded p-3">
                          <pre className="text-xs text-gray-300 overflow-x-auto">
                            {endpoint.example.response}
                          </pre>
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>

        {/* Footer Info */}
        <div className="mt-12 p-6 bg-gray-900 border border-gray-800 rounded-lg">
          <h2 className="text-lg font-semibold mb-4 flex items-center space-x-2">
            <Code size={20} className="text-blue-400" />
            <span>Authentication</span>
          </h2>
          <p className="text-gray-400 text-sm">
            Currently, the API does not require authentication. All endpoints are publicly
            accessible. In production, you should implement proper authentication and
            authorization.
          </p>
        </div>

        <div className="mt-6 p-6 bg-gray-900 border border-gray-800 rounded-lg">
          <h2 className="text-lg font-semibold mb-4">Error Responses</h2>
          <p className="text-gray-400 text-sm mb-3">
            All endpoints may return the following error responses:
          </p>
          <div className="space-y-2 text-sm">
            <div className="bg-gray-800 rounded p-3">
              <div className="font-mono text-red-400">400 Bad Request</div>
              <div className="text-gray-300 mt-1">
                Invalid request parameters or missing required fields
              </div>
            </div>
            <div className="bg-gray-800 rounded p-3">
              <div className="font-mono text-red-400">404 Not Found</div>
              <div className="text-gray-300 mt-1">Agent or resource not found</div>
            </div>
            <div className="bg-gray-800 rounded p-3">
              <div className="font-mono text-red-400">500 Internal Server Error</div>
              <div className="text-gray-300 mt-1">Server error occurred</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

