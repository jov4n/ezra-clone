'use client';

import { useState, useEffect } from 'react';
import { Lock, Copy, Edit, Info, Search, Plus, ChevronDown, ChevronRight, MoreVertical } from 'lucide-react';
import { getAgentConfig, updateAgentConfig, getAgentTools, AgentConfig, Tool } from '@/lib/api';
import Tooltip from '@/components/ui/Tooltip';
import Badge from '@/components/ui/Badge';

interface AgentSettingsPanelProps {
  agentID: string;
  onAgentChange?: (agentID: string) => void;
  onUpdate?: () => void;
}

export default function AgentSettingsPanel({
  agentID,
  onAgentChange,
  onUpdate,
}: AgentSettingsPanelProps) {
  const [activeTab, setActiveTab] = useState<'settings' | 'advanced'>('settings');
  const [config, setConfig] = useState<AgentConfig | null>(null);
  const [tools, setTools] = useState<Tool[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [toolSearch, setToolSearch] = useState('');
  const [expandedCategories, setExpandedCategories] = useState<Set<string>>(new Set(['core']));
  const [editingName, setEditingName] = useState(false);
  const [agentName, setAgentName] = useState(agentID);

  useEffect(() => {
    loadData();
  }, [agentID]);

  const loadData = async () => {
    setLoading(true);
    try {
      const [configData, toolsData] = await Promise.all([
        getAgentConfig(agentID),
        getAgentTools(agentID),
      ]);
      setConfig(configData);
      setTools(toolsData);
      setAgentName(agentID);
    } catch (error) {
      console.error('Failed to load agent settings:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleSaveConfig = async () => {
    if (!config) return;
    setSaving(true);
    try {
      await updateAgentConfig(agentID, config);
      if (onUpdate) {
        onUpdate();
      }
    } catch (error) {
      console.error('Failed to save config:', error);
      alert('Failed to save configuration');
    } finally {
      setSaving(false);
    }
  };

  const toggleCategory = (category: string) => {
    setExpandedCategories(prev => {
      const next = new Set(prev);
      if (next.has(category)) {
        next.delete(category);
      } else {
        next.add(category);
      }
      return next;
    });
  };

  const categorizeTools = (tools: Tool[]) => {
    const categories: Record<string, Tool[]> = {
      core: [],
      other: [],
    };

    const coreToolNames = [
      'core_memory_insert',
      'core_memory_replace',
      'archival_memory_insert',
      'archival_memory_search',
      'memory_search',
      'create_fact',
      'search_facts',
    ];

    tools.forEach(tool => {
      if (coreToolNames.includes(tool.function.name)) {
        categories.core.push(tool);
      } else {
        categories.other.push(tool);
      }
    });

    return categories;
  };

  const filteredTools = tools.filter(tool =>
    tool.function.name.toLowerCase().includes(toolSearch.toLowerCase()) ||
    tool.function.description.toLowerCase().includes(toolSearch.toLowerCase())
  );

  const categorizedTools = categorizeTools(filteredTools);

  if (loading) {
    return (
      <div className="h-full bg-gray-900 dark:bg-gray-900 flex items-center justify-center">
        <div className="text-gray-400">Loading...</div>
      </div>
    );
  }

  return (
    <div className="h-full bg-gray-900 dark:bg-gray-900 flex flex-col overflow-y-auto">
      {/* Tabs */}
      <div className="flex border-b border-gray-800 dark:border-gray-800">
        <button
          onClick={() => setActiveTab('settings')}
          className={`flex-1 px-4 py-2 text-sm font-medium transition-colors ${
            activeTab === 'settings'
              ? 'text-white border-b-2 border-blue-500'
              : 'text-gray-400 hover:text-gray-300'
          }`}
        >
          AGENT SETTINGS
        </button>
        <button
          onClick={() => setActiveTab('advanced')}
          className={`flex-1 px-4 py-2 text-sm font-medium transition-colors ${
            activeTab === 'advanced'
              ? 'text-white border-b-2 border-blue-500'
              : 'text-gray-400 hover:text-gray-300'
          }`}
        >
          ADVANCED
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-6">
        {activeTab === 'settings' && config && (
          <>
            {/* Agent Name */}
            <div>
              <label className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-2 block">
                AGENT NAME
              </label>
              <div className="flex items-center space-x-2">
                {editingName ? (
                  <>
                    <input
                      type="text"
                      value={agentName}
                      onChange={(e) => setAgentName(e.target.value)}
                      className="flex-1 px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                      onBlur={() => setEditingName(false)}
                      onKeyPress={(e) => {
                        if (e.key === 'Enter') {
                          setEditingName(false);
                        }
                      }}
                      autoFocus
                    />
                  </>
                ) : (
                  <>
                    <div className="flex-1 px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-white text-sm flex items-center justify-between">
                      <span>{agentName}</span>
                      <div className="flex items-center space-x-1">
                        <Lock size={14} className="text-gray-500" />
                        <button
                          onClick={() => setEditingName(true)}
                          className="text-gray-500 hover:text-gray-400"
                        >
                          <Edit size={14} />
                        </button>
                      </div>
                    </div>
                  </>
                )}
              </div>
              <div className="mt-1 flex items-center space-x-2 text-xs text-gray-500">
                <span className="font-mono">{agentID}</span>
                <button className="text-gray-500 hover:text-gray-400">
                  <Copy size={12} />
                </button>
              </div>
            </div>

            {/* Model Selection */}
            <div>
              <div className="flex items-center space-x-2 mb-2">
                <label className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                  MODEL
                </label>
                <Tooltip content="Select the LLM model for this agent">
                  <Info size={12} className="text-gray-500 cursor-help" />
                </Tooltip>
              </div>
              <select
                value={config.model || ''}
                onChange={(e) => setConfig({ ...config, model: e.target.value })}
                className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                <option value="openrouter/anthropic/claude-3.5-sonnet">Claude 3.5 Sonnet</option>
                <option value="openrouter/anthropic/claude-3-opus">Claude 3 Opus</option>
                <option value="openrouter/anthropic/claude-3-haiku">Claude 3 Haiku</option>
                <option value="openrouter/google/gemini-2.5-flash">Gemini 2.5 Flash</option>
                <option value="openrouter/google/gemini-pro">Gemini Pro</option>
                <option value="openrouter/openai/gpt-4-turbo">GPT-4 Turbo</option>
                <option value="openrouter/openai/gpt-3.5-turbo">GPT-3.5 Turbo</option>
                <option value="openrouter/meta-llama/llama-3.1-70b-instruct">Llama 3.1 70B</option>
              </select>
            </div>

            {/* System Instructions */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center space-x-2">
                  <label className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                    SYSTEM INSTRUCTIONS
                  </label>
                  <Tooltip content="Define the agent's behavior and personality">
                    <Info size={12} className="text-gray-500 cursor-help" />
                  </Tooltip>
                </div>
                <span className="text-xs text-gray-500">
                  {config.system_instructions.length} CHARS
                </span>
              </div>
              <textarea
                value={config.system_instructions}
                onChange={(e) => setConfig({ ...config, system_instructions: e.target.value })}
                className="w-full h-48 px-3 py-2 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none font-mono"
                placeholder="Enter system instructions..."
              />
            </div>

            {/* Tools Section */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center space-x-2">
                  <span className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                    TOOLS ({tools.length})
                  </span>
                </div>
                <span className="text-xs text-gray-400 uppercase">SOURCES (0)</span>
              </div>
              
              {/* Search Bar */}
              <div className="relative mb-3">
                <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                <input
                  type="text"
                  value={toolSearch}
                  onChange={(e) => setToolSearch(e.target.value)}
                  placeholder="Search tools..."
                  className="w-full pl-9 pr-9 py-2 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
                <button className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 hover:text-gray-400">
                  <Plus size={14} />
                </button>
              </div>

              {/* Tools List */}
              <div className="space-y-1">
                {categorizedTools.core.length > 0 && (
                  <div>
                    <button
                      onClick={() => toggleCategory('core')}
                      className="w-full flex items-center justify-between px-2 py-1.5 text-sm text-gray-300 hover:bg-gray-800 rounded transition-colors"
                    >
                      <span>Letta core tools ({categorizedTools.core.length})</span>
                      {expandedCategories.has('core') ? (
                        <ChevronDown size={14} />
                      ) : (
                        <ChevronRight size={14} />
                      )}
                    </button>
                    {expandedCategories.has('core') && (
                      <div className="ml-4 space-y-0.5">
                        {categorizedTools.core.map((tool) => (
                          <div
                            key={tool.function.name}
                            className="flex items-center justify-between px-2 py-1.5 text-xs text-gray-400 hover:bg-gray-800 rounded group"
                          >
                            <span className="font-mono">{tool.function.name}</span>
                            <button className="opacity-0 group-hover:opacity-100 text-gray-500 hover:text-gray-400">
                              <MoreVertical size={12} />
                            </button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}

                {categorizedTools.other.length > 0 && (
                  <div>
                    <button
                      onClick={() => toggleCategory('other')}
                      className="w-full flex items-center justify-between px-2 py-1.5 text-sm text-gray-300 hover:bg-gray-800 rounded transition-colors"
                    >
                      <span>Other tools ({categorizedTools.other.length})</span>
                      {expandedCategories.has('other') ? (
                        <ChevronDown size={14} />
                      ) : (
                        <ChevronRight size={14} />
                      )}
                    </button>
                    {expandedCategories.has('other') && (
                      <div className="ml-4 space-y-0.5">
                        {categorizedTools.other.map((tool) => (
                          <div
                            key={tool.function.name}
                            className="flex items-center justify-between px-2 py-1.5 text-xs text-gray-400 hover:bg-gray-800 rounded group"
                          >
                            <span className="font-mono">{tool.function.name}</span>
                            <button className="opacity-0 group-hover:opacity-100 text-gray-500 hover:text-gray-400">
                              <MoreVertical size={12} />
                            </button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          </>
        )}

        {activeTab === 'advanced' && config && (
          <div className="space-y-6">
            {/* Model Parameters */}
            <div>
              <div className="flex items-center space-x-2 mb-2">
                <label className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                  MODEL PARAMETERS
                </label>
                <Tooltip content="Advanced model configuration">
                  <Info size={12} className="text-gray-500 cursor-help" />
                </Tooltip>
              </div>
              <div className="space-y-3">
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Temperature (0.0 - 2.0)</label>
                  <input
                    type="number"
                    min="0"
                    max="2"
                    step="0.1"
                    defaultValue="0.7"
                    className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    placeholder="0.7"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Max Tokens</label>
                  <input
                    type="number"
                    min="1"
                    max="100000"
                    defaultValue="4096"
                    className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    placeholder="4096"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Top P (0.0 - 1.0)</label>
                  <input
                    type="number"
                    min="0"
                    max="1"
                    step="0.01"
                    defaultValue="1.0"
                    className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    placeholder="1.0"
                  />
                </div>
              </div>
            </div>

            {/* Rate Limiting */}
            <div>
              <div className="flex items-center space-x-2 mb-2">
                <label className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                  RATE LIMITING
                </label>
                <Tooltip content="Configure request rate limits">
                  <Info size={12} className="text-gray-500 cursor-help" />
                </Tooltip>
              </div>
              <div className="space-y-3">
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Requests per minute</label>
                  <input
                    type="number"
                    min="1"
                    defaultValue="60"
                    className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    placeholder="60"
                  />
                </div>
              </div>
            </div>

            {/* Tool Configuration */}
            <div>
              <div className="flex items-center space-x-2 mb-2">
                <label className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                  TOOL CONFIGURATION
                </label>
                <Tooltip content="Configure which tools are available">
                  <Info size={12} className="text-gray-500 cursor-help" />
                </Tooltip>
              </div>
              <div className="text-xs text-gray-500">
                All tools are currently enabled. Tool-specific configuration coming soon.
              </div>
            </div>

            {/* Export/Import */}
            <div>
              <div className="flex items-center space-x-2 mb-2">
                <label className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                  EXPORT / IMPORT
                </label>
              </div>
              <div className="flex space-x-2">
                <button
                  onClick={() => {
                    const exportData = {
                      agent_id: agentID,
                      config: config,
                      timestamp: new Date().toISOString(),
                    };
                    const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = `agent-${agentID}-${Date.now()}.json`;
                    a.click();
                    URL.revokeObjectURL(url);
                  }}
                  className="px-3 py-2 bg-blue-600 text-white rounded text-xs hover:bg-blue-700 transition-colors"
                >
                  Export Config
                </button>
                <label className="px-3 py-2 bg-gray-700 text-white rounded text-xs hover:bg-gray-600 transition-colors cursor-pointer">
                  Import Config
                  <input
                    type="file"
                    accept=".json"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        const reader = new FileReader();
                        reader.onload = (event) => {
                          try {
                            const data = JSON.parse(event.target?.result as string);
                            if (data.config) {
                              setConfig(data.config);
                              alert('Config loaded. Click Save to apply.');
                            }
                          } catch (error) {
                            alert('Failed to parse config file');
                          }
                        };
                        reader.readAsText(file);
                      }
                    }}
                  />
                </label>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Save Button */}
      {activeTab === 'settings' && config && (
        <div className="p-4 border-t border-gray-800 dark:border-gray-800">
          <button
            onClick={handleSaveConfig}
            disabled={saving}
            className="w-full px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-sm font-medium transition-colors"
          >
            {saving ? 'Saving...' : 'Save Configuration'}
          </button>
        </div>
      )}
    </div>
  );
}

