'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Search, Plus, ExternalLink } from 'lucide-react';
import { listAgents, createAgent, Agent } from '@/lib/api';
import Modal from '@/components/ui/Modal';
import Input from '@/components/ui/Input';
import Button from '@/components/ui/Button';

export default function AgentsPage() {
  const router = useRouter();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newAgent, setNewAgent] = useState({
    name: '',
    model: 'openrouter/anthropic/claude-3.5-sonnet',
    systemInstructions: '',
  });

  useEffect(() => {
    loadAgents();
  }, []);

  const loadAgents = async () => {
    setLoading(true);
    try {
      const agentList = await listAgents();
      setAgents(agentList);
    } catch (error) {
      console.error('Failed to load agents:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleCreateAgent = async () => {
    if (!newAgent.name.trim()) return;
    setCreating(true);
    try {
      await createAgent(newAgent.name, newAgent.model, newAgent.systemInstructions);
      setShowCreateModal(false);
      setNewAgent({ name: '', model: 'openrouter/anthropic/claude-3.5-sonnet', systemInstructions: '' });
      await loadAgents();
    } catch (error) {
      console.error('Failed to create agent:', error);
      alert('Failed to create agent');
    } finally {
      setCreating(false);
    }
  };

  const filteredAgents = agents.filter(agent =>
    agent.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    agent.id.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const formatDate = (dateString: string) => {
    try {
      return new Date(dateString).toLocaleString('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
      });
    } catch {
      return dateString;
    }
  };

  return (
    <div className="min-h-screen bg-gray-950 dark:bg-gray-950">
      {/* Header */}
      <header className="bg-gray-900 dark:bg-gray-900 border-b border-gray-800 dark:border-gray-800 px-6 py-4">
        <div className="flex justify-between items-center">
          <div>
            <h1 className="text-2xl font-semibold text-white mb-1">Local agents</h1>
            <p className="text-sm text-gray-400">
              View your available agents running inside the Ezra server
            </p>
          </div>
          <Button
            onClick={() => setShowCreateModal(true)}
            className="flex items-center space-x-2"
          >
            <Plus size={16} />
            <span>Create a new agent</span>
          </Button>
        </div>
      </header>

      {/* Main Content */}
      <div className="p-6">
        {/* Search Bar */}
        <div className="mb-6">
          <div className="relative max-w-md">
            <Search size={18} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search"
              className="w-full pl-10 pr-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
        </div>

        {/* Agents Table */}
        {loading ? (
          <div className="text-center text-gray-400 py-12">Loading agents...</div>
        ) : (
          <div className="bg-gray-900 rounded-lg border border-gray-800 overflow-hidden">
            <table className="w-full">
              <thead className="bg-gray-800 border-b border-gray-700">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-400 uppercase tracking-wide">
                    NAME
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-400 uppercase tracking-wide">
                    ID
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-400 uppercase tracking-wide">
                    CREATED AT
                  </th>
                  <th className="px-6 py-3 text-right text-xs font-semibold text-gray-400 uppercase tracking-wide">
                    ACTIONS
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800">
                {filteredAgents.map((agent) => (
                  <tr key={agent.id} className="hover:bg-gray-800 transition-colors">
                    <td className="px-6 py-4 text-sm text-white font-medium">
                      {agent.name}
                    </td>
                    <td className="px-6 py-4 text-sm text-gray-400 font-mono">
                      {agent.id}
                    </td>
                    <td className="px-6 py-4 text-sm text-gray-400">
                      {formatDate(agent.created_at)}
                    </td>
                    <td className="px-6 py-4 text-right">
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => router.push(`/?agent=${agent.id}`)}
                        className="flex items-center space-x-1"
                      >
                        <ExternalLink size={14} />
                        <span>Open in ADE</span>
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {filteredAgents.length === 0 && (
              <div className="text-center text-gray-400 py-12">
                {searchQuery ? 'No agents found matching your search' : 'No agents yet. Create one to get started!'}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Create Agent Modal */}
      <Modal
        isOpen={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        title="Create a new agent"
        size="md"
      >
        <div className="space-y-4">
          <Input
            label="Agent Name"
            value={newAgent.name}
            onChange={(e) => setNewAgent({ ...newAgent, name: e.target.value })}
            placeholder="e.g., customer-service-agent"
            required
          />
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">
              Model
            </label>
            <select
              value={newAgent.model}
              onChange={(e) => setNewAgent({ ...newAgent, model: e.target.value })}
              className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              <option value="openrouter/anthropic/claude-3.5-sonnet">Claude 3.5 Sonnet</option>
              <option value="openrouter/anthropic/claude-3-opus">Claude 3 Opus</option>
              <option value="openrouter/anthropic/claude-3-haiku">Claude 3 Haiku</option>
              <option value="openrouter/google/gemini-2.5-flash">Gemini 2.5 Flash</option>
              <option value="openrouter/openai/gpt-4-turbo">GPT-4 Turbo</option>
              <option value="openrouter/openai/gpt-3.5-turbo">GPT-3.5 Turbo</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">
              System Instructions (Optional)
            </label>
            <textarea
              value={newAgent.systemInstructions}
              onChange={(e) => setNewAgent({ ...newAgent, systemInstructions: e.target.value })}
              placeholder="Enter system instructions for the agent..."
              className="w-full h-32 px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
            />
          </div>
          <div className="flex space-x-3 pt-4">
            <Button
              onClick={handleCreateAgent}
              disabled={creating || !newAgent.name.trim()}
              className="flex-1"
            >
              {creating ? 'Creating...' : 'Create Agent'}
            </Button>
            <Button
              variant="secondary"
              onClick={() => setShowCreateModal(false)}
              disabled={creating}
            >
              Cancel
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}

