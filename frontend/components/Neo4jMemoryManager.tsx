'use client';

import { useState, useEffect } from 'react';
import { Database, MessageSquare, Users, Lightbulb, Hash, Archive, Search, ChevronDown, ChevronUp } from 'lucide-react';
import { fetchAllFacts, fetchAllTopics, fetchAllMessages, fetchAllConversations, fetchAllUsers, Fact, Topic, Message, Conversation, User, getArchivalMemories, ArchivalMemory, fetchAgentState, MemoryBlock } from '@/lib/api';
import Tooltip from '@/components/ui/Tooltip';
import Badge from '@/components/ui/Badge';

interface Neo4jMemoryManagerProps {
  agentID: string;
}

export default function Neo4jMemoryManager({ agentID }: Neo4jMemoryManagerProps) {
  const [activeTab, setActiveTab] = useState<'overview' | 'memories' | 'facts' | 'topics' | 'messages' | 'conversations' | 'users'>('overview');
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  
  // Data states
  const [facts, setFacts] = useState<Fact[]>([]);
  const [topics, setTopics] = useState<Topic[]>([]);
  const [messages, setMessages] = useState<Message[]>([]);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [archivalMemories, setArchivalMemories] = useState<ArchivalMemory[]>([]);
  const [coreMemory, setCoreMemory] = useState<MemoryBlock[]>([]);
  
  // Expanded sections
  const [expandedSections, setExpandedSections] = useState<Record<string, boolean>>({
    memories: true,
    facts: true,
    topics: true,
    messages: false,
    conversations: false,
    users: false,
  });

  useEffect(() => {
    loadAllData();
  }, [agentID]);

  const loadAllData = async () => {
    setLoading(true);
    try {
      const [factsData, topicsData, messagesData, conversationsData, usersData, archivalData, stateData] = await Promise.all([
        fetchAllFacts(agentID).catch(() => []),
        fetchAllTopics(agentID).catch(() => []),
        fetchAllMessages(agentID, 200).catch(() => []),
        fetchAllConversations(agentID, 100).catch(() => []),
        fetchAllUsers(agentID).catch(() => []),
        getArchivalMemories(agentID).catch(() => []),
        fetchAgentState(agentID).catch(() => null),
      ]);
      
      setFacts(factsData);
      setTopics(topicsData);
      setMessages(messagesData);
      setConversations(conversationsData);
      setUsers(usersData);
      setArchivalMemories(archivalData);
      if (stateData) {
        setCoreMemory(stateData.core_memory || []);
      }
    } catch (error) {
      console.error('Failed to load Neo4j data:', error);
    } finally {
      setLoading(false);
    }
  };

  const toggleSection = (section: string) => {
    setExpandedSections(prev => ({
      ...prev,
      [section]: !prev[section],
    }));
  };

  const filterData = <T extends { content?: string; name?: string; summary?: string; channel_id?: string; platform?: string; discord_username?: string }>(data: T[]): T[] => {
    if (!searchQuery.trim()) return data;
    const query = searchQuery.toLowerCase();
    return data.filter(item => 
      (item.content?.toLowerCase().includes(query)) ||
      (item.name?.toLowerCase().includes(query)) ||
      (item.summary?.toLowerCase().includes(query)) ||
      (item.channel_id?.toLowerCase().includes(query)) ||
      (item.platform?.toLowerCase().includes(query)) ||
      (item.discord_username?.toLowerCase().includes(query))
    );
  };

  const discordMessages = (messages || []).filter(m => m.platform === 'discord');
  const discordConversations = (conversations || []).filter(c => c.platform === 'discord');

  if (loading) {
    return (
      <div className="h-full bg-gray-900 flex items-center justify-center">
        <div className="text-gray-400">Loading Neo4j data...</div>
      </div>
    );
  }

  return (
    <div className="h-full bg-gray-900 flex flex-col">
      {/* Header */}
      <div className="p-4 border-b border-gray-800">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center space-x-2">
            <Database size={18} className="text-blue-400" />
            <h2 className="text-lg font-semibold text-white">Neo4j Memory Manager</h2>
          </div>
          <div className="flex items-center space-x-2">
            <div className="relative">
              <Search size={16} className="absolute left-2 top-1/2 transform -translate-y-1/2 text-gray-400" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search all data..."
                className="pl-8 pr-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 w-64"
              />
            </div>
          </div>
        </div>

        {/* Tabs */}
        <div className="flex space-x-1">
          {[
            { id: 'overview', label: 'Overview', icon: Database },
            { id: 'memories', label: 'Memories', icon: Archive },
            { id: 'facts', label: 'Facts', icon: Lightbulb },
            { id: 'topics', label: 'Topics', icon: Hash },
            { id: 'messages', label: 'Messages', icon: MessageSquare },
            { id: 'conversations', label: 'Conversations', icon: MessageSquare },
            { id: 'users', label: 'Users', icon: Users },
          ].map(tab => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id as any)}
                className={`px-3 py-1.5 text-xs font-medium rounded transition-colors ${
                  activeTab === tab.id
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-800 text-gray-400 hover:bg-gray-700 hover:text-gray-300'
                }`}
              >
                <div className="flex items-center space-x-1">
                  <Icon size={14} />
                  <span>{tab.label}</span>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-4">
        {activeTab === 'overview' && (
          <div className="space-y-4">
            {/* Statistics Cards */}
            <div className="grid grid-cols-4 gap-4">
              <div className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="text-xs text-gray-400 mb-1">Core Memory Blocks</div>
                <div className="text-2xl font-bold text-white">{coreMemory.length}</div>
              </div>
              <div className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="text-xs text-gray-400 mb-1">Archival Memories</div>
                <div className="text-2xl font-bold text-white">{archivalMemories?.length || 0}</div>
              </div>
              <div className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="text-xs text-gray-400 mb-1">Facts</div>
                <div className="text-2xl font-bold text-white">{facts?.length || 0}</div>
              </div>
              <div className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="text-xs text-gray-400 mb-1">Topics</div>
                <div className="text-2xl font-bold text-white">{topics?.length || 0}</div>
              </div>
            </div>

            <div className="grid grid-cols-3 gap-4">
              <div className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="text-xs text-gray-400 mb-1">Total Messages</div>
                <div className="text-2xl font-bold text-white">{messages?.length || 0}</div>
                <div className="text-xs text-gray-500 mt-1">
                  {discordMessages?.length || 0} from Discord
                </div>
              </div>
              <div className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="text-xs text-gray-400 mb-1">Conversations</div>
                <div className="text-2xl font-bold text-white">{conversations?.length || 0}</div>
                <div className="text-xs text-gray-500 mt-1">
                  {discordConversations?.length || 0} Discord
                </div>
              </div>
              <div className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="text-xs text-gray-400 mb-1">Users</div>
                <div className="text-2xl font-bold text-white">{users?.length || 0}</div>
                <div className="text-xs text-gray-500 mt-1">
                  {users?.filter(u => u.discord_id).length || 0} Discord users
                </div>
              </div>
            </div>

            {/* Quick Sections */}
            <div className="space-y-3">
              {/* Core Memory */}
              <div className="bg-gray-800 rounded border border-gray-700">
                <button
                  onClick={() => toggleSection('memories')}
                  className="w-full flex items-center justify-between p-3 text-left"
                >
                  <div className="flex items-center space-x-2">
                    <Archive size={16} className="text-gray-400" />
                    <span className="text-sm font-semibold text-white">Core Memory Blocks ({coreMemory?.length || 0})</span>
                  </div>
                  {expandedSections.memories ? <ChevronUp size={16} className="text-gray-400" /> : <ChevronDown size={16} className="text-gray-400" />}
                </button>
                {expandedSections.memories && (
                  <div className="px-3 pb-3 space-y-2">
                    {(coreMemory || []).map((block, idx) => (
                      <div key={idx} className="bg-gray-900 p-2 rounded text-xs">
                        <div className="font-semibold text-gray-300 mb-1">{block.name}</div>
                        <div className="text-gray-400 line-clamp-2">{block.content}</div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Facts */}
              <div className="bg-gray-800 rounded border border-gray-700">
                <button
                  onClick={() => toggleSection('facts')}
                  className="w-full flex items-center justify-between p-3 text-left"
                >
                  <div className="flex items-center space-x-2">
                    <Lightbulb size={16} className="text-gray-400" />
                    <span className="text-sm font-semibold text-white">Facts ({facts?.length || 0})</span>
                  </div>
                  {expandedSections.facts ? <ChevronUp size={16} className="text-gray-400" /> : <ChevronDown size={16} className="text-gray-400" />}
                </button>
                {expandedSections.facts && (
                  <div className="px-3 pb-3 space-y-2 max-h-64 overflow-y-auto">
                    {filterData(facts || []).slice(0, 10).map((fact) => (
                      <div key={fact.id} className="bg-gray-900 p-2 rounded text-xs">
                        <div className="text-gray-400 mb-1">{fact.content}</div>
                        {fact.source && (
                          <div className="text-gray-500 text-xs">Source: {fact.source}</div>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Discord Messages */}
              <div className="bg-gray-800 rounded border border-gray-700">
                <button
                  onClick={() => toggleSection('messages')}
                  className="w-full flex items-center justify-between p-3 text-left"
                >
                  <div className="flex items-center space-x-2">
                    <MessageSquare size={16} className="text-gray-400" />
                    <span className="text-sm font-semibold text-white">Discord Messages ({discordMessages?.length || 0})</span>
                  </div>
                  {expandedSections.messages ? <ChevronUp size={16} className="text-gray-400" /> : <ChevronDown size={16} className="text-gray-400" />}
                </button>
                {expandedSections.messages && (
                  <div className="px-3 pb-3 space-y-2 max-h-64 overflow-y-auto">
                    {filterData(discordMessages || []).slice(0, 10).map((msg) => (
                      <div key={msg.id} className="bg-gray-900 p-2 rounded text-xs">
                        <div className="flex items-center justify-between mb-1">
                          <Badge variant="default" className="text-xs">
                            {msg.role}
                          </Badge>
                          <span className="text-gray-500 text-xs">
                            {new Date(msg.timestamp).toLocaleString()}
                          </span>
                        </div>
                        <div className="text-gray-300">{msg.content}</div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        )}

        {activeTab === 'memories' && (
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-gray-300 mb-3">Core Memory Blocks ({coreMemory?.length || 0})</h3>
              <div className="space-y-2">
                {coreMemory.map((block, idx) => (
                  <div key={idx} className="bg-gray-800 p-3 rounded border border-gray-700">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-xs font-semibold text-gray-300 uppercase">{block.name}</span>
                      <span className="text-xs text-gray-500">{block.content.length} chars</span>
                    </div>
                    <div className="text-sm text-gray-400 whitespace-pre-wrap">{block.content}</div>
                  </div>
                ))}
              </div>
            </div>

            <div>
              <h3 className="text-sm font-semibold text-gray-300 mb-3">Archival Memories ({archivalMemories?.length || 0})</h3>
              <div className="space-y-2">
                {filterData(archivalMemories || []).map((memory) => (
                  <div key={memory.id} className="bg-gray-800 p-3 rounded border border-gray-700">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-xs font-semibold text-gray-300">{memory.summary}</span>
                      <span className="text-xs text-gray-500">
                        {new Date(memory.timestamp).toLocaleDateString()}
                      </span>
                    </div>
                    <div className="text-sm text-gray-400">{memory.content}</div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        {activeTab === 'facts' && (
          <div className="space-y-2">
            {filterData(facts || []).map((fact) => (
              <div key={fact.id} className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="flex items-start justify-between mb-2">
                  <div className="flex-1">
                    <div className="text-sm text-white mb-1">{fact.content}</div>
                    {fact.source && (
                      <div className="text-xs text-gray-500">Source: {fact.source}</div>
                    )}
                  </div>
                  <Badge variant="default" className="text-xs">
                    {Math.round(fact.confidence * 100)}%
                  </Badge>
                </div>
                <div className="text-xs text-gray-500">
                  {new Date(fact.created_at).toLocaleString()}
                </div>
              </div>
            ))}
          </div>
        )}

        {activeTab === 'topics' && (
          <div className="grid grid-cols-2 gap-3">
            {filterData(topics || []).map((topic) => (
              <div key={topic.id} className="bg-gray-800 p-3 rounded border border-gray-700">
                <div className="text-sm font-semibold text-white mb-1">{topic.name}</div>
                {topic.description && (
                  <div className="text-xs text-gray-400">{topic.description}</div>
                )}
              </div>
            ))}
          </div>
        )}

        {activeTab === 'messages' && (
          <div className="space-y-2">
            {filterData(messages || []).map((msg) => (
              <div key={msg.id} className="bg-gray-800 p-3 rounded border border-gray-700">
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center space-x-2">
                    <Badge variant={msg.platform === 'discord' ? 'primary' : 'default'} className="text-xs">
                      {msg.platform}
                    </Badge>
                    <Badge variant="default" className="text-xs">
                      {msg.role}
                    </Badge>
                  </div>
                  <span className="text-xs text-gray-500">
                    {new Date(msg.timestamp).toLocaleString()}
                  </span>
                </div>
                <div className="text-sm text-gray-300">{msg.content}</div>
              </div>
            ))}
          </div>
        )}

        {activeTab === 'conversations' && (
          <div className="space-y-2">
            {filterData(conversations || []).map((conv) => (
              <div key={conv.id} className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center space-x-2">
                    <Badge variant={conv.platform === 'discord' ? 'primary' : 'default'} className="text-xs">
                      {conv.platform}
                    </Badge>
                    {conv.channel_id && (
                      <span className="text-xs text-gray-400">Channel: {conv.channel_id}</span>
                    )}
                  </div>
                  <span className="text-xs text-gray-500">
                    {new Date(conv.started_at).toLocaleString()}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}

        {activeTab === 'users' && (
          <div className="space-y-2">
            {(users || []).map((user) => (
              <div key={user.id} className="bg-gray-800 p-4 rounded border border-gray-700">
                <div className="flex items-center justify-between mb-2">
                  <div>
                    <div className="text-sm font-semibold text-white">
                      {user.discord_username || user.id}
                    </div>
                    {user.discord_id && (
                      <div className="text-xs text-gray-400">Discord ID: {user.discord_id}</div>
                    )}
                  </div>
                  <Badge variant={user.discord_id ? 'primary' : 'default'} className="text-xs">
                    {user.discord_id ? 'Discord' : 'Web'}
                  </Badge>
                </div>
                <div className="text-xs text-gray-500">
                  First seen: {new Date(user.first_seen).toLocaleDateString()} | 
                  Last seen: {new Date(user.last_seen).toLocaleDateString()}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

