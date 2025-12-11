'use client';

import { useState, useEffect } from 'react';
import { Info, Search, Plus, Copy, Trash2, Edit2, Save, X } from 'lucide-react';
import { getContextStats, getArchivalMemories, createArchivalMemory, deleteArchivalMemory, fetchAgentState, updateMemoryBlock, deleteMemoryBlock, ContextWindow, ArchivalMemory, MemoryBlock } from '@/lib/api';
import Tooltip from '@/components/ui/Tooltip';
import Neo4jMemoryManager from './Neo4jMemoryManager';

interface ContextWindowPanelProps {
  agentID: string;
  onUpdate?: () => void;
}

export default function ContextWindowPanel({
  agentID,
  onUpdate,
}: ContextWindowPanelProps) {
  const [contextStats, setContextStats] = useState({ used_tokens: 0, total_tokens: 8192 });
  const [state, setState] = useState<ContextWindow | null>(null);
  const [archivalMemories, setArchivalMemories] = useState<ArchivalMemory[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [archivalSearch, setArchivalSearch] = useState('');
  const [showAddMemory, setShowAddMemory] = useState(false);
  const [newMemoryContent, setNewMemoryContent] = useState('');
  const [showAllBlocks, setShowAllBlocks] = useState(true);
  const [editingBlock, setEditingBlock] = useState<string | null>(null);
  const [editingContent, setEditingContent] = useState('');
  const [showAddBlock, setShowAddBlock] = useState(false);
  const [newBlockName, setNewBlockName] = useState('');
  const [newBlockContent, setNewBlockContent] = useState('');
  const [showNeo4jManager, setShowNeo4jManager] = useState(false);

  useEffect(() => {
    loadData();
    // Only auto-refresh when not editing to avoid clearing user input
    if (!editingBlock && !showAddBlock) {
      const interval = setInterval(loadData, 5000); // Increased to 5 seconds
      return () => clearInterval(interval);
    }
  }, [agentID, editingBlock, showAddBlock]);

  const loadData = async () => {
    try {
      const [stats, stateData, memories] = await Promise.all([
        getContextStats(agentID).catch(() => ({ used_tokens: 0, total_tokens: 8192 })),
        fetchAgentState(agentID).catch(() => null),
        getArchivalMemories(agentID).catch(() => []),
      ]);
      setContextStats(stats);
      setState(stateData);
      setArchivalMemories(memories);
    } catch (error) {
      console.error('Failed to load context data:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleSaveMemory = async (blockName: string, content: string) => {
    setSaving(true);
    try {
      await updateMemoryBlock(agentID, blockName, content);
      // Only refresh on success
      await loadData();
      if (onUpdate) {
        onUpdate();
      }
    } catch (error: any) {
      console.error('Failed to save memory:', error);
      const errorMsg = error?.response?.data?.error || error?.message || 'Failed to save memory block';
      alert(`Failed to save memory block: ${errorMsg}`);
      // Don't refresh on error - preserve user's input
    } finally {
      setSaving(false);
    }
  };

  const handleAddArchivalMemory = async () => {
    if (!newMemoryContent.trim()) return;
    setSaving(true);
    try {
      await createArchivalMemory(agentID, {
        summary: newMemoryContent.substring(0, 100),
        content: newMemoryContent,
        relevance_score: 0.5,
      });
      setNewMemoryContent('');
      setShowAddMemory(false);
      await loadData();
      if (onUpdate) {
        onUpdate();
      }
    } catch (error) {
      console.error('Failed to create archival memory:', error);
      alert('Failed to create archival memory');
    } finally {
      setSaving(false);
    }
  };

  const getPersonaBlock = () => {
    return state?.core_memory.find(b => b.name.toLowerCase() === 'persona') || { name: 'persona', content: '', updated_at: '' };
  };

  const getHumanBlock = () => {
    return state?.core_memory.find(b => b.name.toLowerCase() === 'human') || { name: 'human', content: '', updated_at: '' };
  };

  const getAllOtherBlocks = () => {
    if (!state) return [];
    return state.core_memory.filter(b => 
      b.name.toLowerCase() !== 'persona' && b.name.toLowerCase() !== 'human'
    );
  };

  const handleDeleteBlock = async (blockName: string) => {
    if (!confirm(`Delete memory block "${blockName}"?`)) return;
    setSaving(true);
    try {
      await deleteMemoryBlock(agentID, blockName);
      await loadData();
      if (onUpdate) {
        onUpdate();
      }
    } catch (error) {
      console.error('Failed to delete memory block:', error);
      alert('Failed to delete memory block');
    } finally {
      setSaving(false);
    }
  };

  const handleStartEdit = (block: MemoryBlock) => {
    setEditingBlock(block.name);
    setEditingContent(block.content);
  };

  const handleSaveEdit = async () => {
    if (!editingBlock) return;
    setSaving(true);
    try {
      await updateMemoryBlock(agentID, editingBlock, editingContent);
      setEditingBlock(null);
      setEditingContent('');
      await loadData();
      if (onUpdate) {
        onUpdate();
      }
    } catch (error: any) {
      console.error('Failed to save memory block:', error);
      const errorMsg = error?.response?.data?.error || error?.message || 'Failed to save memory block';
      alert(`Failed to save memory block: ${errorMsg}`);
      // Don't clear editing state on error - let user try again
    } finally {
      setSaving(false);
    }
  };

  const handleCreateBlock = async () => {
    if (!newBlockName.trim()) return;
    setSaving(true);
    try {
      await updateMemoryBlock(agentID, newBlockName.trim(), newBlockContent);
      setNewBlockName('');
      setNewBlockContent('');
      setShowAddBlock(false);
      await loadData();
      if (onUpdate) {
        onUpdate();
      }
    } catch (error: any) {
      console.error('Failed to create memory block:', error);
      const errorMsg = error?.response?.data?.error || error?.message || 'Failed to create memory block';
      alert(`Failed to create memory block: ${errorMsg}`);
      // Don't clear form on error - let user try again
    } finally {
      setSaving(false);
    }
  };

  const filteredMemories = (archivalMemories || []).filter(m =>
    (m.summary || '').toLowerCase().includes(archivalSearch.toLowerCase()) ||
    (m.content || '').toLowerCase().includes(archivalSearch.toLowerCase())
  );

  const tokenPercentage = (contextStats.used_tokens / contextStats.total_tokens) * 100;
  const getTokenColor = () => {
    if (tokenPercentage < 50) return 'bg-blue-500';
    if (tokenPercentage < 70) return 'bg-teal-500';
    if (tokenPercentage < 85) return 'bg-green-500';
    if (tokenPercentage < 95) return 'bg-yellow-500';
    if (tokenPercentage < 99) return 'bg-orange-500';
    return 'bg-red-500';
  };

  if (loading) {
    return (
      <div className="h-full bg-gray-900 dark:bg-gray-900 flex items-center justify-center">
        <div className="text-gray-400">Loading...</div>
      </div>
    );
  }

  const personaBlock = getPersonaBlock();
  const humanBlock = getHumanBlock();

  if (showNeo4jManager) {
    return <Neo4jMemoryManager agentID={agentID} />;
  }

  return (
    <div className="h-full bg-gray-900 dark:bg-gray-900 flex flex-col overflow-y-auto overscroll-contain">
      {/* Context Window Section */}
      <div className="p-4 border-b border-gray-800 dark:border-gray-800">
        <div className="flex items-center space-x-2 mb-2">
          <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
            CONTEXT WINDOW
          </h3>
          <Tooltip content="Token usage in the current context window">
            <Info size={12} className="text-gray-500 cursor-help" />
          </Tooltip>
        </div>
        <div className="text-sm text-white mb-2">
          {contextStats.used_tokens.toLocaleString()} / {contextStats.total_tokens.toLocaleString()} TOKENS
        </div>
        <div className="w-full h-2 bg-gray-800 rounded-full overflow-hidden">
          <div
            className={`h-full ${getTokenColor()} transition-all duration-300`}
            style={{ width: `${Math.min(tokenPercentage, 100)}%` }}
          />
        </div>
      </div>

      {/* Core Memory Section */}
      <div className="p-4 border-b border-gray-800 dark:border-gray-800">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center space-x-2">
            <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
              CORE MEMORY ({state?.core_memory.length || 0})
            </h3>
            <Tooltip content="Editable memory blocks that are always in context">
              <Info size={12} className="text-gray-500 cursor-help" />
            </Tooltip>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={() => setShowNeo4jManager(!showNeo4jManager)}
              className="text-xs px-2 py-1 bg-blue-600 hover:bg-blue-700 text-white rounded"
              title="View all Neo4j data"
            >
              Neo4j Manager
            </button>
            <button
              onClick={() => setShowAllBlocks(!showAllBlocks)}
              className="text-xs text-gray-400 hover:text-gray-300"
            >
              {showAllBlocks ? 'Show Standard' : 'Show All Blocks'}
            </button>
            <button
              onClick={() => setShowAddBlock(true)}
              className="p-1 text-gray-400 hover:text-gray-300"
              title="Add new memory block"
            >
              <Plus size={14} />
            </button>
          </div>
        </div>

        {/* PERSONA Block */}
        <div className="mb-4">
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs font-semibold text-gray-300 uppercase">PERSONA</span>
            <span className="text-xs text-gray-500">
              {personaBlock.content.length} / 5000 CHARS
            </span>
          </div>
          <textarea
            value={personaBlock.content}
            onChange={(e) => {
              const newState = { ...state } as ContextWindow;
              if (!newState) return;
              const block = newState.core_memory.find(b => b.name.toLowerCase() === 'persona');
              if (block) {
                block.content = e.target.value;
              } else {
                newState.core_memory.push({ name: 'persona', content: e.target.value, updated_at: new Date().toISOString() });
              }
              setState(newState);
            }}
            onBlur={() => handleSaveMemory('persona', personaBlock.content)}
            className="w-full h-32 px-3 py-2 bg-gray-800 border border-gray-700 rounded text-white text-xs focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
            placeholder="Persona description..."
            maxLength={5000}
          />
        </div>

        {/* HUMAN Block */}
        <div>
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs font-semibold text-gray-300 uppercase">HUMAN</span>
            <span className="text-xs text-gray-500">
              {humanBlock.content.length} / 5000 CHARS
            </span>
          </div>
          <textarea
            value={humanBlock.content}
            onChange={(e) => {
              const newState = { ...state } as ContextWindow;
              if (!newState) return;
              const block = newState.core_memory.find(b => b.name.toLowerCase() === 'human');
              if (block) {
                block.content = e.target.value;
              } else {
                newState.core_memory.push({ name: 'human', content: e.target.value, updated_at: new Date().toISOString() });
              }
              setState(newState);
            }}
            onBlur={() => handleSaveMemory('human', humanBlock.content)}
            className="w-full h-32 px-3 py-2 bg-gray-800 border border-gray-700 rounded text-white text-xs focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
            placeholder="Information about the human user..."
            maxLength={5000}
          />
        </div>

        {/* Add New Block Form */}
        {showAddBlock && (
          <div className="mt-4 p-3 bg-gray-800 rounded border border-gray-700">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-semibold text-gray-300">New Memory Block</span>
              <button
                onClick={() => {
                  setShowAddBlock(false);
                  setNewBlockName('');
                  setNewBlockContent('');
                }}
                className="text-gray-400 hover:text-gray-300"
              >
                <X size={14} />
              </button>
            </div>
            <input
              type="text"
              value={newBlockName}
              onChange={(e) => setNewBlockName(e.target.value)}
              placeholder="Block name (e.g., 'instructions', 'preferences')"
              className="w-full mb-2 px-2 py-1 bg-gray-900 border border-gray-700 rounded text-white text-xs focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <textarea
              value={newBlockContent}
              onChange={(e) => setNewBlockContent(e.target.value)}
              placeholder="Block content..."
              className="w-full h-24 px-2 py-1 bg-gray-900 border border-gray-700 rounded text-white text-xs focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
              maxLength={5000}
            />
            <div className="flex items-center justify-between mt-2">
              <span className="text-xs text-gray-500">
                {newBlockContent.length} / 5000 CHARS
              </span>
              <div className="flex space-x-2">
                <button
                  onClick={handleCreateBlock}
                  disabled={saving || !newBlockName.trim()}
                  className="px-2 py-1 bg-blue-600 text-white rounded text-xs hover:bg-blue-700 disabled:opacity-50"
                >
                  Create
                </button>
                <button
                  onClick={() => {
                    setShowAddBlock(false);
                    setNewBlockName('');
                    setNewBlockContent('');
                  }}
                  className="px-2 py-1 bg-gray-700 text-white rounded text-xs hover:bg-gray-600"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>
        )}

        {/* All Other Memory Blocks - Neo4j Memory Blocks */}
        {showAllBlocks && (
          <div className="mt-4 space-y-3 border-t border-gray-700 pt-4">
            <div className="flex items-center justify-between mb-2">
              <div className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                Neo4j Memory Blocks ({getAllOtherBlocks().length})
              </div>
              <Tooltip content="All memory blocks stored in Neo4j database">
                <Info size={12} className="text-gray-500 cursor-help" />
              </Tooltip>
            </div>
            {getAllOtherBlocks().length > 0 ? (
              getAllOtherBlocks().map((block) => (
              <div key={block.name} className="bg-gray-800 rounded border border-gray-700 p-3">
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center space-x-2">
                    <span className="text-xs font-semibold text-gray-300">{block.name}</span>
                    <span className="text-xs text-gray-500">
                      {block.content.length} / 5000 CHARS
                    </span>
                  </div>
                  <div className="flex items-center space-x-1">
                    {editingBlock === block.name ? (
                      <>
                        <button
                          onClick={handleSaveEdit}
                          disabled={saving}
                          className="p-1 text-green-400 hover:text-green-300"
                          title="Save"
                        >
                          <Save size={12} />
                        </button>
                        <button
                          onClick={() => {
                            setEditingBlock(null);
                            setEditingContent('');
                          }}
                          className="p-1 text-gray-400 hover:text-gray-300"
                          title="Cancel"
                        >
                          <X size={12} />
                        </button>
                      </>
                    ) : (
                      <>
                        <button
                          onClick={() => handleStartEdit(block)}
                          className="p-1 text-gray-400 hover:text-gray-300"
                          title="Edit"
                        >
                          <Edit2 size={12} />
                        </button>
                        <button
                          onClick={() => handleDeleteBlock(block.name)}
                          className="p-1 text-gray-400 hover:text-red-400"
                          title="Delete"
                        >
                          <Trash2 size={12} />
                        </button>
                      </>
                    )}
                  </div>
                </div>
                {editingBlock === block.name ? (
                  <textarea
                    value={editingContent}
                    onChange={(e) => setEditingContent(e.target.value)}
                    className="w-full h-32 px-2 py-1 bg-gray-900 border border-gray-700 rounded text-white text-xs focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
                    maxLength={5000}
                  />
                ) : (
                  <div className="text-xs text-gray-300 bg-gray-900 p-2 rounded max-h-32 overflow-y-auto whitespace-pre-wrap">
                    {block.content || '(empty)'}
                  </div>
                )}
              </div>
            ))
            ) : (
              <div className="text-xs text-gray-500 text-center py-4 bg-gray-800 rounded border border-gray-700">
                No additional memory blocks. Click the + button to create one.
              </div>
            )}
          </div>
        )}
      </div>

      {/* Archival Memories Section */}
      <div className="p-4 flex-1 flex flex-col min-h-0">
        <div className="flex items-center space-x-2 mb-2">
          <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
            ARCHIVAL MEMORIES ({(archivalMemories || []).length})
          </h3>
          <Tooltip content="Long-term memories that can be searched">
            <Info size={12} className="text-gray-500 cursor-help" />
          </Tooltip>
        </div>

        {/* Search Bar */}
        <div className="relative mb-3">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
          <input
            type="text"
            value={archivalSearch}
            onChange={(e) => setArchivalSearch(e.target.value)}
            placeholder="Search memories..."
            className="w-full pl-9 pr-9 py-2 bg-gray-800 border border-gray-700 rounded text-white text-xs focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          <button
            onClick={() => setShowAddMemory(!showAddMemory)}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 hover:text-gray-400"
          >
            <Plus size={14} />
          </button>
        </div>

        {/* Add Memory Form */}
        {showAddMemory && (
          <div className="mb-3 p-3 bg-gray-800 rounded border border-gray-700">
            <textarea
              value={newMemoryContent}
              onChange={(e) => setNewMemoryContent(e.target.value)}
              placeholder="Enter memory content..."
              className="w-full h-20 px-2 py-1 bg-gray-900 border border-gray-700 rounded text-white text-xs focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none mb-2"
            />
            <div className="flex space-x-2">
              <button
                onClick={handleAddArchivalMemory}
                disabled={saving || !newMemoryContent.trim()}
                className="flex-1 px-2 py-1 bg-blue-600 text-white rounded text-xs hover:bg-blue-700 disabled:opacity-50"
              >
                Add
              </button>
              <button
                onClick={() => {
                  setShowAddMemory(false);
                  setNewMemoryContent('');
                }}
                className="px-2 py-1 bg-gray-700 text-white rounded text-xs hover:bg-gray-600"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Memories List */}
        <div className="flex-1 overflow-y-auto space-y-2">
          {filteredMemories.map((memory, idx) => (
            <div
              key={idx}
              className="p-3 bg-gray-800 rounded border border-gray-700 hover:border-gray-600 transition-colors"
            >
              <div className="flex items-start justify-between mb-1">
                <div className="flex-1 min-w-0">
                  <div className="text-xs text-gray-400 mb-1">
                    {new Date(memory.timestamp).toLocaleString()}
                  </div>
                  <div className="text-xs text-gray-300 line-clamp-2">
                    {memory.summary || memory.content.substring(0, 100)}
                  </div>
                </div>
                <div className="flex items-center space-x-1 ml-2">
                  <button
                    onClick={() => {
                      navigator.clipboard.writeText(memory.content || memory.summary);
                    }}
                    className="text-gray-500 hover:text-gray-400"
                    title="Copy memory"
                  >
                    <Copy size={12} />
                  </button>
                  <button
                    onClick={async () => {
                      if (confirm('Delete this archival memory?')) {
                        try {
                          await deleteArchivalMemory(agentID, memory.id);
                          await loadData();
                          if (onUpdate) {
                            onUpdate();
                          }
                        } catch (error) {
                          console.error('Failed to delete memory:', error);
                          alert('Failed to delete memory');
                        }
                      }
                    }}
                    className="text-gray-500 hover:text-red-400"
                    title="Delete memory"
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              </div>
            </div>
          ))}
          {filteredMemories.length === 0 && (
            <div className="text-center text-gray-500 text-xs py-8">
              No archival memories found
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

