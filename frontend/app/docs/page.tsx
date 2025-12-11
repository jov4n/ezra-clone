'use client';

import { useState } from 'react';
import { BookOpen, ChevronRight, ChevronDown, Code, Settings, MessageSquare, Database, Users, Zap, Search } from 'lucide-react';
import { useRouter } from 'next/navigation';

interface DocSection {
  id: string;
  title: string;
  icon: any;
  content: React.ReactNode;
}

const docSections: DocSection[] = [
  {
    id: 'getting-started',
    title: 'Getting Started',
    icon: Zap,
    content: (
      <div className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">What is the ADE?</h3>
          <p className="text-gray-300">
            The Agent Development Environment (ADE) is a powerful interface for creating, managing, and interacting with AI agents. 
            Each agent has its own memory, personality, and capabilities that you can customize.
          </p>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Quick Start</h3>
          <ol className="list-decimal list-inside space-y-2 text-gray-300">
            <li>Navigate to the <strong className="text-white">Agents</strong> page to see all available agents</li>
            <li>Create a new agent or select an existing one</li>
            <li>Configure the agent's model and system instructions in the left sidebar</li>
            <li>Start chatting with your agent in the center panel</li>
            <li>Monitor context window usage and manage memories in the right sidebar</li>
          </ol>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Key Concepts</h3>
          <div className="space-y-2 text-gray-300">
            <p><strong className="text-white">Agents:</strong> Individual AI assistants with their own memory and configuration</p>
            <p><strong className="text-white">Core Memory:</strong> Persistent memory blocks (PERSONA, HUMAN) that are always in context</p>
            <p><strong className="text-white">Archival Memory:</strong> Long-term memories that can be searched and retrieved</p>
            <p><strong className="text-white">Context Window:</strong> The token budget for each conversation</p>
            <p><strong className="text-white">Tools:</strong> Functions the agent can call to perform actions</p>
          </div>
        </div>
      </div>
    ),
  },
  {
    id: 'creating-agents',
    title: 'Creating Agents',
    icon: Users,
    content: (
      <div className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Creating a New Agent</h3>
          <ol className="list-decimal list-inside space-y-2 text-gray-300">
            <li>Go to the <strong className="text-white">Agents</strong> page</li>
            <li>Click the <strong className="text-white">"Create a new agent"</strong> button</li>
            <li>Enter the agent's name</li>
            <li>Select a model (e.g., Claude 3.5 Sonnet, GPT-4 Turbo)</li>
            <li>Optionally set initial system instructions</li>
            <li>Click <strong className="text-white">"Create"</strong></li>
          </ol>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Agent Configuration</h3>
          <p className="text-gray-300 mb-2">
            After creating an agent, you can configure it in the ADE:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300">
            <li><strong className="text-white">Model:</strong> Choose which LLM the agent uses</li>
            <li><strong className="text-white">System Instructions:</strong> Define the agent's behavior and personality</li>
            <li><strong className="text-white">Tools:</strong> View and manage available tools</li>
          </ul>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Available Models</h3>
          <div className="bg-gray-800 rounded p-3 text-sm text-gray-300">
            <ul className="space-y-1">
              <li>• Claude 3.5 Sonnet</li>
              <li>• Claude 3 Opus</li>
              <li>• Claude 3 Haiku</li>
              <li>• Gemini 2.5 Flash</li>
              <li>• Gemini Pro</li>
              <li>• GPT-4 Turbo</li>
              <li>• GPT-3.5 Turbo</li>
              <li>• Llama 3.1 70B</li>
            </ul>
          </div>
        </div>
      </div>
    ),
  },
  {
    id: 'memory-management',
    title: 'Memory Management',
    icon: Database,
    content: (
      <div className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Core Memory</h3>
          <p className="text-gray-300 mb-2">
            Core memory consists of two main blocks that are always included in the agent's context:
          </p>
          <div className="space-y-2">
            <div className="bg-gray-800 rounded p-3">
              <p className="text-sm font-semibold text-white mb-1">PERSONA</p>
              <p className="text-xs text-gray-400">
                Defines the agent's personality, behavior, and characteristics. This is how the agent sees itself.
              </p>
            </div>
            <div className="bg-gray-800 rounded p-3">
              <p className="text-sm font-semibold text-white mb-1">HUMAN</p>
              <p className="text-xs text-gray-400">
                Contains information about the human user(s) the agent interacts with. Preferences, facts, and context about users.
              </p>
            </div>
          </div>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Custom Memory Blocks</h3>
          <p className="text-gray-300 mb-2">
            You can create additional memory blocks beyond PERSONA and HUMAN:
          </p>
          <ol className="list-decimal list-inside space-y-1 text-gray-300 text-sm">
            <li>Click the <strong className="text-white">+</strong> button in the Core Memory section</li>
            <li>Enter a block name (e.g., "instructions", "preferences")</li>
            <li>Add content to the block</li>
            <li>The block will be included in the agent's context</li>
          </ol>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Archival Memory</h3>
          <p className="text-gray-300 mb-2">
            Archival memories are long-term storage that can be searched and retrieved when relevant:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li>Add new memories with a summary and content</li>
            <li>Search through memories using the search bar</li>
            <li>Delete memories that are no longer needed</li>
            <li>Memories are automatically retrieved based on relevance</li>
          </ul>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Neo4j Memory Manager</h3>
          <p className="text-gray-300 mb-2">
            The Neo4j Memory Manager provides a comprehensive view of all data stored in the database:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li>View all memory blocks, facts, topics, and conversations</li>
            <li>See Discord-specific memories and messages</li>
            <li>Search across all data types</li>
            <li>Access via the "Neo4j Manager" button in the Context Window panel</li>
          </ul>
        </div>
      </div>
    ),
  },
  {
    id: 'chatting',
    title: 'Chatting with Agents',
    icon: MessageSquare,
    content: (
      <div className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Basic Chat</h3>
          <p className="text-gray-300 mb-2">
            To chat with an agent:
          </p>
          <ol className="list-decimal list-inside space-y-1 text-gray-300 text-sm">
            <li>Select an agent from the dropdown in the header</li>
            <li>Type your message in the input box at the bottom</li>
            <li>Press Enter or click Send</li>
            <li>The agent will respond using its configured model and memory</li>
          </ol>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Tool Calls</h3>
          <p className="text-gray-300 mb-2">
            Agents can use tools to perform actions. When a tool is called, you'll see:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li>The tool name and status</li>
            <li>Expandable JSON showing the request arguments</li>
            <li>Tool execution results</li>
          </ul>
          <p className="text-gray-400 text-xs mt-2">
            Click on tool calls to expand and see detailed information.
          </p>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Variables Panel</h3>
          <p className="text-gray-300 mb-2">
            Click the "Variables" button in the token usage bar to see:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li>Current agent ID</li>
            <li>User ID</li>
            <li>Message count</li>
            <li>Other conversation metadata</li>
          </ul>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Token Usage</h3>
          <p className="text-gray-300 mb-2">
            Monitor your context window usage:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li>View used vs. total tokens in the Context Window panel</li>
            <li>Color-coded progress bar (green = safe, yellow = warning, red = critical)</li>
            <li>Token usage includes: system prompt, memory blocks, conversation history, and tool results</li>
          </ul>
        </div>
      </div>
    ),
  },
  {
    id: 'tools',
    title: 'Tools & Capabilities',
    icon: Zap,
    content: (
      <div className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Available Tools</h3>
          <p className="text-gray-300 mb-2">
            Agents have access to various tools organized by category:
          </p>
          <div className="space-y-2">
            <div className="bg-gray-800 rounded p-3">
              <p className="text-sm font-semibold text-white mb-1">Core Tools</p>
              <ul className="text-xs text-gray-400 space-y-1">
                <li>• update_core_memory - Update memory blocks</li>
                <li>• search_memories - Search through memories</li>
                <li>• create_archival_memory - Create long-term memories</li>
              </ul>
            </div>
            <div className="bg-gray-800 rounded p-3">
              <p className="text-sm font-semibold text-white mb-1">Discord Tools</p>
              <ul className="text-xs text-gray-400 space-y-1">
                <li>• send_message - Send messages to Discord channels</li>
                <li>• get_channel_info - Get channel information</li>
                <li>• get_user_info - Get user information</li>
              </ul>
            </div>
          </div>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Viewing Tools</h3>
          <p className="text-gray-300 mb-2">
            To see all available tools:
          </p>
          <ol className="list-decimal list-inside space-y-1 text-gray-300 text-sm">
            <li>Open the Agent Settings panel (left sidebar)</li>
            <li>Scroll to the "Tools" section</li>
            <li>Use the search bar to filter tools</li>
            <li>Expand categories to see tool details</li>
          </ol>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Tool Execution</h3>
          <p className="text-gray-300 mb-2">
            When an agent uses a tool:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li>The tool is executed automatically</li>
            <li>Results are returned to the agent</li>
            <li>The agent can use results to form its response</li>
            <li>Tool calls are visible in the chat interface</li>
          </ul>
        </div>
      </div>
    ),
  },
  {
    id: 'context-window',
    title: 'Context Window',
    icon: Settings,
    content: (
      <div className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Understanding Context Windows</h3>
          <p className="text-gray-300 mb-2">
            The context window is the token budget for each conversation. It includes:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li>System prompt and instructions</li>
            <li>Core memory blocks (PERSONA, HUMAN, custom blocks)</li>
            <li>Relevant archival memories</li>
            <li>Conversation history</li>
            <li>Tool call results</li>
          </ul>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Token Usage Display</h3>
          <p className="text-gray-300 mb-2">
            The Context Window panel shows:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li><strong className="text-white">Used Tokens:</strong> Current token count</li>
            <li><strong className="text-white">Total Tokens:</strong> Maximum context window size</li>
            <li><strong className="text-white">Progress Bar:</strong> Visual indicator with color coding</li>
          </ul>
          <div className="bg-gray-800 rounded p-3 mt-2 text-xs text-gray-400">
            <p><strong className="text-green-400">Green:</strong> &lt; 85% usage (safe)</p>
            <p><strong className="text-yellow-400">Yellow:</strong> 85-95% usage (warning)</p>
            <p><strong className="text-red-400">Red:</strong> &gt; 95% usage (critical)</p>
          </div>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Optimizing Context Usage</h3>
          <p className="text-gray-300 mb-2">
            To keep token usage manageable:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li>Keep core memory blocks concise</li>
            <li>Use archival memory for less critical information</li>
            <li>Archive old conversations when needed</li>
            <li>Monitor the token usage bar regularly</li>
          </ul>
        </div>
      </div>
    ),
  },
  {
    id: 'api-integration',
    title: 'API Integration',
    icon: Code,
    content: (
      <div className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">REST API</h3>
          <p className="text-gray-300 mb-2">
            The ADE provides a comprehensive REST API for programmatic access:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-300 text-sm">
            <li>All endpoints are documented in the <strong className="text-white">API Reference</strong> page</li>
            <li>Base URL: <code className="bg-gray-800 px-1 rounded">http://localhost:8080</code></li>
            <li>All responses are in JSON format</li>
            <li>No authentication required (for development)</li>
          </ul>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Common Endpoints</h3>
          <div className="bg-gray-800 rounded p-3 space-y-2 text-sm">
            <div>
              <code className="text-blue-400">GET /api/agents</code>
              <p className="text-gray-400 text-xs">List all agents</p>
            </div>
            <div>
              <code className="text-green-400">POST /api/agent/:id/chat</code>
              <p className="text-gray-400 text-xs">Send a message to an agent</p>
            </div>
            <div>
              <code className="text-blue-400">GET /api/agent/:id/state</code>
              <p className="text-gray-400 text-xs">Get agent's current state</p>
            </div>
            <div>
              <code className="text-yellow-400">PUT /api/agent/:id/config</code>
              <p className="text-gray-400 text-xs">Update agent configuration</p>
            </div>
          </div>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">Example: Chat with Agent</h3>
          <div className="bg-gray-800 rounded p-3">
            <pre className="text-xs text-gray-300 overflow-x-auto">
{`curl -X POST http://localhost:8080/api/agent/Ezra/chat \\
  -H "Content-Type: application/json" \\
  -d '{
    "message": "Hello!",
    "user_id": "developer"
  }'`}
            </pre>
          </div>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white mb-2">For More Information</h3>
          <p className="text-gray-300 text-sm">
            See the <strong className="text-white">API Reference</strong> page for complete documentation, 
            including all endpoints, parameters, request/response examples, and error codes.
          </p>
        </div>
      </div>
    ),
  },
];

export default function DocsPage() {
  const router = useRouter();
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['getting-started']));
  const [searchQuery, setSearchQuery] = useState('');

  const toggleSection = (sectionId: string) => {
    setExpandedSections(prev => {
      const next = new Set(prev);
      if (next.has(sectionId)) {
        next.delete(sectionId);
      } else {
        next.add(sectionId);
      }
      return next;
    });
  };

  const filteredSections = docSections.filter(section =>
    section.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
    section.id.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="min-h-screen bg-gray-950 text-white">
      {/* Header */}
      <header className="bg-gray-900 border-b border-gray-800 px-6 py-4">
        <div className="max-w-7xl mx-auto flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <BookOpen size={24} className="text-blue-400" />
            <div>
              <h1 className="text-2xl font-bold">Documentation</h1>
              <p className="text-sm text-gray-400 mt-1">
                Learn how to use the Agent Development Environment
              </p>
            </div>
          </div>
          <button
            onClick={() => router.push('/api-reference')}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-lg text-sm font-medium transition-colors flex items-center space-x-2"
          >
            <Code size={16} />
            <span>API Reference</span>
          </button>
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
              placeholder="Search documentation..."
              className="w-full pl-10 pr-4 py-3 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
        </div>

        {/* Documentation Sections */}
        <div className="space-y-3">
          {filteredSections.map((section) => {
            const Icon = section.icon;
            const isExpanded = expandedSections.has(section.id);
            
            return (
              <div
                key={section.id}
                className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden"
              >
                <button
                  onClick={() => toggleSection(section.id)}
                  className="w-full px-6 py-4 flex items-center justify-between hover:bg-gray-800 transition-colors text-left"
                >
                  <div className="flex items-center space-x-3">
                    <Icon size={20} className="text-blue-400" />
                    <h2 className="text-lg font-semibold text-white">{section.title}</h2>
                  </div>
                  {isExpanded ? (
                    <ChevronDown size={20} className="text-gray-400" />
                  ) : (
                    <ChevronRight size={20} className="text-gray-400" />
                  )}
                </button>
                {isExpanded && (
                  <div className="px-6 py-4 border-t border-gray-800">
                    {section.content}
                  </div>
                )}
              </div>
            );
          })}
        </div>

        {/* Quick Links */}
        <div className="mt-12 grid grid-cols-1 md:grid-cols-3 gap-4">
          <div
            onClick={() => router.push('/agents')}
            className="bg-gray-900 border border-gray-800 rounded-lg p-6 hover:border-blue-600 cursor-pointer transition-colors"
          >
            <Users size={24} className="text-blue-400 mb-3" />
            <h3 className="text-lg font-semibold text-white mb-2">Manage Agents</h3>
            <p className="text-sm text-gray-400">
              Create, view, and manage your AI agents
            </p>
          </div>
          <div
            onClick={() => router.push('/api-reference')}
            className="bg-gray-900 border border-gray-800 rounded-lg p-6 hover:border-blue-600 cursor-pointer transition-colors"
          >
            <Code size={24} className="text-blue-400 mb-3" />
            <h3 className="text-lg font-semibold text-white mb-2">API Reference</h3>
            <p className="text-sm text-gray-400">
              Complete API documentation with examples
            </p>
          </div>
          <div
            onClick={() => router.push('/settings')}
            className="bg-gray-900 border border-gray-800 rounded-lg p-6 hover:border-blue-600 cursor-pointer transition-colors"
          >
            <Settings size={24} className="text-blue-400 mb-3" />
            <h3 className="text-lg font-semibold text-white mb-2">Settings</h3>
            <p className="text-sm text-gray-400">
              Configure integrations and API keys
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}

