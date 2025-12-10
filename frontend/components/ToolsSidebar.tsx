'use client';

import { useState } from 'react';
import { MoreVertical, ChevronDown, ChevronRight } from 'lucide-react';

interface Tool {
  name: string;
  description?: string;
}

interface ToolCategory {
  name: string;
  icon: string;
  color: string;
  tools: Tool[];
}

const toolCategories: ToolCategory[] = [
  {
    name: 'Memory',
    icon: '',
    color: 'from-purple-500 to-purple-600',
    tools: [
      { name: 'core_memory_insert', description: 'Create new memory block' },
      { name: 'core_memory_replace', description: 'Update existing memory' },
      { name: 'archival_memory_insert', description: 'Archive information' },
      { name: 'archival_memory_search', description: 'Search archives' },
      { name: 'memory_search', description: 'Search all memories' },
    ],
  },
  {
    name: 'Knowledge',
    icon: '',
    color: 'from-blue-500 to-blue-600',
    tools: [
      { name: 'create_fact', description: 'Store a new fact' },
      { name: 'search_facts', description: 'Search known facts' },
      { name: 'get_user_context', description: 'Get user information' },
    ],
  },
  {
    name: 'Topics',
    icon: '',
    color: 'from-green-500 to-green-600',
    tools: [
      { name: 'create_topic', description: 'Create a new topic' },
      { name: 'link_topics', description: 'Connect related topics' },
      { name: 'find_related_topics', description: 'Find related topics' },
      { name: 'link_user_to_topic', description: 'Track user interest' },
    ],
  },
  {
    name: 'Conversation',
    icon: '',
    color: 'from-yellow-500 to-yellow-600',
    tools: [
      { name: 'get_conversation_history', description: 'Get recent messages' },
      { name: 'send_message', description: 'Send response to user' },
    ],
  },
  {
    name: 'Web',
    icon: '',
    color: 'from-cyan-500 to-cyan-600',
    tools: [
      { name: 'web_search', description: 'Search the web' },
      { name: 'fetch_webpage', description: 'Read webpage content' },
    ],
  },
  {
    name: 'GitHub',
    icon: '',
    color: 'from-gray-600 to-gray-700',
    tools: [
      { name: 'github_repo_info', description: 'Get repository info' },
      { name: 'github_search', description: 'Search GitHub' },
      { name: 'github_read_file', description: 'Read file from repo' },
      { name: 'github_list_org_repos', description: 'List org repos by update' },
    ],
  },
  {
    name: 'Discord',
    icon: '',
    color: 'from-indigo-500 to-indigo-600',
    tools: [
      { name: 'discord_read_history', description: 'Read channel messages' },
      { name: 'discord_get_user_info', description: 'Get user info' },
      { name: 'discord_get_channel_info', description: 'Get channel info' },
    ],
  },
  {
    name: 'Personality',
    icon: '',
    color: 'from-pink-500 to-pink-600',
    tools: [
      { name: 'mimic_personality', description: 'Mimic user\'s style' },
      { name: 'revert_personality', description: 'Return to normal' },
      { name: 'analyze_user_style', description: 'Analyze communication style' },
    ],
  },
];

interface ToolsSidebarProps {
  tools?: Tool[];
}

export default function ToolsSidebar({ tools }: ToolsSidebarProps) {
  const [expandedCategories, setExpandedCategories] = useState<Set<string>>(
    new Set(['Memory', 'Knowledge'])
  );

  const toggleCategory = (category: string) => {
    setExpandedCategories((prev) => {
      const next = new Set(prev);
      if (next.has(category)) {
        next.delete(category);
      } else {
        next.add(category);
      }
      return next;
    });
  };

  return (
    <div className="h-full bg-gray-50 dark:bg-gray-900 border-r border-gray-200 dark:border-gray-700 flex flex-col">
      {/* Header */}
      <div className="p-4 border-b border-gray-200 dark:border-gray-700">
        <h2 className="text-sm font-semibold text-gray-900 dark:text-white uppercase tracking-wide">
          Tools ({toolCategories.reduce((acc, c) => acc + c.tools.length, 0)})
        </h2>
        <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
          Available agent capabilities
        </p>
      </div>

      {/* Tools List by Category */}
      <div className="flex-1 overflow-y-auto p-2">
        <div className="space-y-2">
          {toolCategories.map((category) => (
            <div key={category.name} className="rounded-lg overflow-hidden">
              {/* Category Header */}
              <button
                onClick={() => toggleCategory(category.name)}
                className="w-full flex items-center justify-between p-2 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors rounded-lg"
              >
                <div className="flex items-center space-x-2">
                  {category.icon && <span className="text-lg">{category.icon}</span>}
                  <span className="text-sm font-medium text-gray-900 dark:text-white">
                    {category.name}
                  </span>
                  <span className="text-xs text-gray-500 dark:text-gray-400 bg-gray-200 dark:bg-gray-700 px-1.5 py-0.5 rounded">
                    {category.tools.length}
                  </span>
                </div>
                {expandedCategories.has(category.name) ? (
                  <ChevronDown size={16} className="text-gray-400" />
                ) : (
                  <ChevronRight size={16} className="text-gray-400" />
                )}
              </button>

              {/* Tools in Category */}
              {expandedCategories.has(category.name) && (
                <div className="ml-2 space-y-0.5">
                  {category.tools.map((tool) => (
                    <div
                      key={tool.name}
                      className="flex items-center justify-between p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors cursor-pointer group"
                    >
                      <div className="flex items-center space-x-2 flex-1 min-w-0">
                        <div
                          className={`w-6 h-6 rounded bg-gradient-to-br ${category.color} flex items-center justify-center flex-shrink-0 shadow-sm`}
                        >
                          <span className="text-white text-xs font-bold">
                            {tool.name.charAt(0).toUpperCase()}
                          </span>
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="text-xs font-medium text-gray-900 dark:text-white truncate font-mono">
                            {tool.name}
                          </div>
                          {tool.description && (
                            <div className="text-xs text-gray-500 dark:text-gray-400 truncate">
                              {tool.description}
                            </div>
                          )}
                        </div>
                      </div>
                      <button className="opacity-0 group-hover:opacity-100 transition-opacity p-1 hover:bg-gray-200 dark:hover:bg-gray-700 rounded">
                        <MoreVertical size={14} className="text-gray-400" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Footer */}
      <div className="p-3 border-t border-gray-200 dark:border-gray-700">
        <div className="text-xs text-gray-500 dark:text-gray-400 text-center">
          Graph-powered memory system
        </div>
      </div>
    </div>
  );
}
