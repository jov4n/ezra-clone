'use client';

import { useState } from 'react';
import { ArrowLeft, Info, ExternalLink } from 'lucide-react';
import { useRouter } from 'next/navigation';
import Modal from '@/components/ui/Modal';
import Input from '@/components/ui/Input';
import Button from '@/components/ui/Button';
import Tooltip from '@/components/ui/Tooltip';

interface Integration {
  id: string;
  name: string;
  description: string;
  icon: string;
  configured: boolean;
}

const integrations: Integration[] = [
  {
    id: 'openai',
    name: 'OpenAI',
    description: 'Connect to the OpenAI API to use GPT models in the ADE',
    icon: '🧠',
    configured: false,
  },
  {
    id: 'anthropic',
    name: 'Anthropic',
    description: 'Connect to the Anthropic API to use Anthropic models in the ADE',
    icon: '🤖',
    configured: false,
  },
  {
    id: 'ollama',
    name: 'Ollama',
    description: 'Connect to the Ollama API to use Ollama models in the ADE',
    icon: '🦙',
    configured: false,
  },
  {
    id: 'azure',
    name: 'Azure',
    description: 'Connect to the Azure API to use Azure models in the ADE',
    icon: '☁️',
    configured: false,
  },
  {
    id: 'composio',
    name: 'Composio',
    description: 'Connect to the Composio API to use Composio tools in the ADE',
    icon: '⚡',
    configured: false,
  },
];

export default function SettingsPage() {
  const router = useRouter();
  const [selectedIntegration, setSelectedIntegration] = useState<string | null>(null);
  const [apiKey, setApiKey] = useState('');
  const [saving, setSaving] = useState(false);

  const handleConfigure = (integrationId: string) => {
    setSelectedIntegration(integrationId);
    setApiKey('');
  };

  const handleSave = async () => {
    if (!selectedIntegration || !apiKey.trim()) return;
    setSaving(true);
    // In a real implementation, this would save to backend
    // For now, just save to localStorage
    localStorage.setItem(`api_key_${selectedIntegration}`, apiKey);
    setSaving(false);
    setSelectedIntegration(null);
    setApiKey('');
    alert('API key saved (stored locally)');
  };

  return (
    <div className="min-h-screen bg-gray-950 dark:bg-gray-950">
      {/* Header */}
      <header className="bg-gray-900 dark:bg-gray-900 border-b border-gray-800 dark:border-gray-800 px-6 py-4">
        <div className="flex items-center space-x-4">
          <button
            onClick={() => router.back()}
            className="p-2 text-gray-400 hover:text-gray-300 transition-colors"
          >
            <ArrowLeft size={20} />
          </button>
          <div>
            <h1 className="text-2xl font-semibold text-white">Integrations</h1>
            <p className="text-sm text-gray-400 mt-1">
              Below you can update the environment settings for the ADE.
            </p>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <div className="p-6 max-w-4xl">
        <div className="space-y-4">
          {integrations.map((integration) => (
            <div
              key={integration.id}
              className="bg-gray-900 rounded-lg border border-gray-800 p-6 hover:border-gray-700 transition-colors"
            >
              <div className="flex items-start justify-between">
                <div className="flex items-start space-x-4 flex-1">
                  <div className="text-4xl">{integration.icon}</div>
                  <div className="flex-1">
                    <h3 className="text-lg font-semibold text-white mb-1">
                      {integration.name}
                    </h3>
                    <p className="text-sm text-gray-400 mb-3">
                      {integration.description}
                    </p>
                    <a
                      href="#"
                      className="text-sm text-blue-400 hover:text-blue-300 inline-flex items-center space-x-1"
                    >
                      <span>Learn more</span>
                      <ExternalLink size={14} />
                    </a>
                  </div>
                </div>
                <div className="flex items-center space-x-3">
                  {integration.configured && (
                    <span className="text-xs text-green-400">Configured</span>
                  )}
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => handleConfigure(integration.id)}
                  >
                    Configure
                  </Button>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Configure Modal */}
      <Modal
        isOpen={selectedIntegration !== null}
        onClose={() => {
          setSelectedIntegration(null);
          setApiKey('');
        }}
        title={`Update connection with ${integrations.find(i => i.id === selectedIntegration)?.name || ''}`}
        size="md"
      >
        <div className="space-y-4">
          <div className="flex items-start space-x-2 p-3 bg-blue-900/20 border border-blue-800 rounded-lg">
            <Info size={16} className="text-blue-400 mt-0.5 flex-shrink-0" />
            <p className="text-sm text-gray-300">
              Learn about how to connect and more about this integration{' '}
              <a href="#" className="text-blue-400 hover:text-blue-300 underline">
                here
              </a>
            </p>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              {integrations.find(i => i.id === selectedIntegration)?.name.toUpperCase()} API KEY
            </label>
            <Input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder="Enter your API key..."
              className="font-mono"
            />
          </div>
          <div className="flex space-x-3 pt-4">
            <Button
              onClick={handleSave}
              disabled={saving || !apiKey.trim()}
              className="flex-1"
            >
              {saving ? 'Saving...' : 'Confirm'}
            </Button>
            <Button
              variant="secondary"
              onClick={() => {
                setSelectedIntegration(null);
                setApiKey('');
              }}
              disabled={saving}
            >
              Cancel
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}

