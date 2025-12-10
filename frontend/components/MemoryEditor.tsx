'use client';

import { useState, useEffect } from 'react';
import { Save, Plus } from 'lucide-react';
import { updateMemoryBlock, fetchAgentState, ContextWindow } from '@/lib/api';

interface MemoryEditorProps {
  agentID: string;
  onUpdate?: () => void;
}

export default function MemoryEditor({ agentID, onUpdate }: MemoryEditorProps) {
  const [state, setState] = useState<ContextWindow | null>(null);
  const [selectedBlock, setSelectedBlock] = useState<string>('');
  const [content, setContent] = useState('');
  const [newBlockName, setNewBlockName] = useState('');
  const [isCreating, setIsCreating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [success, setSuccess] = useState(false);

  useEffect(() => {
    loadState();
  }, [agentID]);

  const loadState = async () => {
    try {
      const data = await fetchAgentState(agentID);
      setState(data);
      if (data.core_memory.length > 0 && !selectedBlock) {
        setSelectedBlock(data.core_memory[0].name);
        setContent(data.core_memory[0].content);
      }
    } catch (error) {
      console.error('Failed to load state:', error);
    }
  };

  useEffect(() => {
    if (state && selectedBlock) {
      const block = state.core_memory.find((m) => m.name === selectedBlock);
      if (block) {
        setContent(block.content);
      } else {
        setContent('');
      }
    }
  }, [selectedBlock, state]);

  const handleSave = async () => {
    if (!selectedBlock || saving) return;

    setSaving(true);
    setSuccess(false);

    try {
      await updateMemoryBlock(agentID, selectedBlock, content);
      setSuccess(true);
      setTimeout(() => setSuccess(false), 2000);
      if (onUpdate) {
        onUpdate();
      }
      await loadState();
    } catch (error) {
      console.error('Failed to update memory:', error);
      alert('Failed to update memory block');
    } finally {
      setSaving(false);
    }
  };

  const handleCreate = async () => {
    if (!newBlockName.trim() || saving) return;

    setSaving(true);
    try {
      await updateMemoryBlock(agentID, newBlockName.trim(), '');
      setSelectedBlock(newBlockName.trim());
      setNewBlockName('');
      setIsCreating(false);
      if (onUpdate) {
        onUpdate();
      }
      await loadState();
    } catch (error) {
      console.error('Failed to create memory block:', error);
      alert('Failed to create memory block');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="h-full flex flex-col bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg">
      <div className="p-4 border-b border-gray-200 dark:border-gray-700">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
          Memory Editor
        </h2>
        <div className="flex space-x-2">
          <select
            value={selectedBlock}
            onChange={(e) => setSelectedBlock(e.target.value)}
            className="flex-1 px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-800 dark:text-white"
          >
            <option value="">Select a memory block...</option>
            {state?.core_memory.map((block) => (
              <option key={block.name} value={block.name}>
                {block.name}
              </option>
            ))}
          </select>
          <button
            onClick={() => setIsCreating(true)}
            className="px-3 py-2 bg-green-500 text-white rounded-lg hover:bg-green-600 flex items-center space-x-2"
          >
            <Plus size={20} />
            <span>New</span>
          </button>
        </div>
        {isCreating && (
          <div className="mt-4 flex space-x-2">
            <input
              type="text"
              value={newBlockName}
              onChange={(e) => setNewBlockName(e.target.value)}
              placeholder="New block name..."
              className="flex-1 px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-800 dark:text-white"
            />
            <button
              onClick={handleCreate}
              disabled={saving}
              className="px-3 py-2 bg-green-500 text-white rounded-lg hover:bg-green-600 disabled:opacity-50"
            >
              Create
            </button>
            <button
              onClick={() => {
                setIsCreating(false);
                setNewBlockName('');
              }}
              className="px-3 py-2 bg-gray-500 text-white rounded-lg hover:bg-gray-600"
            >
              Cancel
            </button>
          </div>
        )}
      </div>
      <div className="flex-1 p-4">
        <textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          placeholder="Memory block content..."
          className="w-full h-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-800 dark:text-white font-mono text-sm"
          disabled={!selectedBlock || saving}
        />
      </div>
      <div className="p-4 border-t border-gray-200 dark:border-gray-700 flex justify-between items-center">
        <div>
          {success && (
            <span className="text-green-500 text-sm">Saved successfully!</span>
          )}
        </div>
        <button
          onClick={handleSave}
          disabled={!selectedBlock || saving || isCreating}
          className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed flex items-center space-x-2"
        >
          <Save size={20} />
          <span>{saving ? 'Saving...' : 'Save'}</span>
        </button>
      </div>
    </div>
  );
}

