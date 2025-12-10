'use client';

import { useState, useEffect } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { fetchAgentState, ContextWindow, MemoryBlock } from '@/lib/api';

interface CoreMemorySidebarProps {
  agentID: string;
  onUpdate?: () => void;
}

export default function CoreMemorySidebar({ agentID, onUpdate }: CoreMemorySidebarProps) {
  const [state, setState] = useState<ContextWindow | null>(null);
  const [loading, setLoading] = useState(true);
  const [visibleBlocks, setVisibleBlocks] = useState<Set<string>>(new Set());

  useEffect(() => {
    loadState();
    const interval = setInterval(loadState, 2000);
    return () => clearInterval(interval);
  }, [agentID]);

  const loadState = async () => {
    try {
      const data = await fetchAgentState(agentID);
      setState(data);
      // Initialize all blocks as visible
      if (data && visibleBlocks.size === 0) {
        setVisibleBlocks(new Set(data.core_memory.map(b => b.name)));
      }
    } catch (error) {
      console.error('Failed to load state:', error);
    } finally {
      setLoading(false);
    }
  };

  const toggleVisibility = (blockName: string) => {
    setVisibleBlocks(prev => {
      const next = new Set(prev);
      if (next.has(blockName)) {
        next.delete(blockName);
      } else {
        next.add(blockName);
      }
      return next;
    });
  };

  if (loading) {
    return (
      <div className="h-full bg-gray-50 dark:bg-gray-900 border-l border-gray-200 dark:border-gray-700 p-4">
        <div className="text-sm text-gray-500 dark:text-gray-400">Loading...</div>
      </div>
    );
  }

  return (
    <div className="h-full bg-gray-50 dark:bg-gray-900 border-l border-gray-200 dark:border-gray-700 flex flex-col">
      {/* Header */}
      <div className="p-4 border-b border-gray-200 dark:border-gray-700">
        <h2 className="text-sm font-semibold text-gray-900 dark:text-white uppercase tracking-wide">
          Core Memory
        </h2>
      </div>

      {/* Blocks Section */}
      <div className="flex-1 overflow-y-auto p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-xs font-medium text-gray-700 dark:text-gray-300 uppercase tracking-wide">
            Blocks
          </h3>
          <div className="text-xs text-gray-500 dark:text-gray-400">
            {state?.core_memory.length || 0} blocks
          </div>
        </div>

        <div className="space-y-3">
          {state?.core_memory.map((block: MemoryBlock) => {
            const isVisible = visibleBlocks.has(block.name);
            const preview = block.content.substring(0, 100).replace(/\n/g, ' ');
            
            return (
              <div
                key={block.name}
                className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-3 hover:border-gray-300 dark:hover:border-gray-600 transition-colors"
              >
                <div className="flex items-start justify-between mb-2">
                  <div className="flex-1">
                    <div className="flex items-center space-x-2 mb-1">
                      <h4 className="text-sm font-semibold text-gray-900 dark:text-white">
                        {block.name}
                      </h4>
                      <button
                        onClick={() => toggleVisibility(block.name)}
                        className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
                        title={isVisible ? 'Hide' : 'Show'}
                      >
                        {isVisible ? (
                          <Eye size={14} />
                        ) : (
                          <EyeOff size={14} />
                        )}
                      </button>
                    </div>
                    <div className="text-xs text-gray-500 dark:text-gray-400 mb-2">
                      {block.content.length} Chars
                    </div>
                  </div>
                </div>
                
                {isVisible && (
                  <div className="text-xs text-gray-700 dark:text-gray-300 bg-gray-50 dark:bg-gray-900 rounded p-3 border border-gray-200 dark:border-gray-700 max-h-64 overflow-y-auto">
                    <div className="whitespace-pre-wrap break-words prose prose-xs max-w-none dark:prose-invert">
                      {block.content.split('\n').map((line, i) => {
                        // Format markdown-like content
                        if (line.startsWith('# ')) {
                          return <h1 key={i} className="text-sm font-bold mt-2 mb-1">{line.slice(2)}</h1>;
                        }
                        if (line.startsWith('## ')) {
                          return <h2 key={i} className="text-xs font-semibold mt-1.5 mb-1">{line.slice(3)}</h2>;
                        }
                        if (line.startsWith('**') && line.endsWith('**')) {
                          return <strong key={i} className="font-semibold">{line.slice(2, -2)}</strong>;
                        }
                        if (line.startsWith('- ') || line.startsWith('* ')) {
                          return <div key={i} className="ml-2">• {line.slice(2)}</div>;
                        }
                        return <div key={i}>{line || '\u00A0'}</div>;
                      })}
                    </div>
                  </div>
                )}
                
                {!isVisible && (
                  <div className="text-xs text-gray-500 dark:text-gray-400 line-clamp-2">
                    {preview}...
                  </div>
                )}
              </div>
            );
          })}
          
          {(!state?.core_memory || state.core_memory.length === 0) && (
            <div className="text-sm text-gray-500 dark:text-gray-400 text-center py-8">
              No memory blocks yet
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

