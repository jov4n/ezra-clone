'use client';

import { useState, useEffect, Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import { Settings, Users, ChevronLeft, ChevronRight, X } from 'lucide-react';
import { useRouter } from 'next/navigation';
import AgentSettingsPanel from '@/components/AgentSettingsPanel';
import ChatArea from '@/components/ChatArea';
import ContextWindowPanel from '@/components/ContextWindowPanel';
import { listAgents, Agent } from '@/lib/api';

const DEFAULT_AGENT_ID = 'Ezra';
const DEFAULT_USER_ID = 'developer';

function HomeContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [agentID, setAgentID] = useState(searchParams?.get('agent') || DEFAULT_AGENT_ID);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentsLoading, setAgentsLoading] = useState(true);
  const [refreshKey, setRefreshKey] = useState(0);
  const [leftSidebarOpen, setLeftSidebarOpen] = useState(true);
  const [rightSidebarOpen, setRightSidebarOpen] = useState(true);

  useEffect(() => {
    loadAgents();
  }, []);

  useEffect(() => {
    const agentParam = searchParams?.get('agent');
    if (agentParam) {
      setAgentID(agentParam);
    }
  }, [searchParams]);

  const loadAgents = async () => {
    setAgentsLoading(true);
    try {
      const agentList = await listAgents();
      setAgents(agentList);
      if (agentList.length > 0 && !agentList.find(a => a.id === agentID)) {
        const newAgentID = agentList[0].id;
        setAgentID(newAgentID);
        router.replace(`/?agent=${newAgentID}`);
      }
    } catch (error) {
      console.error('Failed to load agents:', error);
    } finally {
      setAgentsLoading(false);
    }
  };

  const handleMessageSent = () => {
    setRefreshKey(prev => prev + 1);
  };

  return (
    <div className="h-screen flex flex-col bg-gray-950 dark:bg-gray-950">
      {/* Top Navigation Bar */}
      <header className="bg-gray-900 dark:bg-gray-900 border-b border-gray-800 dark:border-gray-800 px-6 py-3 flex-shrink-0">
        <div className="flex justify-between items-center">
          <div className="flex items-center space-x-4">
            {/* Mobile sidebar toggles */}
            <div className="lg:hidden flex items-center space-x-2">
              <button
                onClick={() => setLeftSidebarOpen(!leftSidebarOpen)}
                className="p-2 text-gray-400 hover:text-white transition-colors"
                aria-label="Toggle left sidebar"
              >
                <ChevronRight size={20} className={leftSidebarOpen ? 'rotate-180' : ''} />
              </button>
              <button
                onClick={() => setRightSidebarOpen(!rightSidebarOpen)}
                className="p-2 text-gray-400 hover:text-white transition-colors"
                aria-label="Toggle right sidebar"
              >
                <ChevronLeft size={20} className={rightSidebarOpen ? 'rotate-180' : ''} />
              </button>
            </div>
            <h1 className="text-lg font-semibold text-white">
              Ezra Clone / {agentID}
            </h1>
            {agentsLoading ? (
              <div className="px-3 py-1.5 text-gray-400 text-sm">Loading agents...</div>
            ) : agents.length > 0 ? (
              <select
                value={agentID}
                onChange={(e) => {
                  setAgentID(e.target.value);
                  router.replace(`/?agent=${e.target.value}`);
                }}
                className="px-3 py-1.5 bg-gray-800 border border-gray-700 rounded-lg text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                {agents.map((agent) => (
                  <option key={agent.id} value={agent.id}>
                    {agent.name}
                  </option>
                ))}
              </select>
            ) : null}
          </div>
          <div className="flex items-center space-x-2 lg:space-x-4">
            <button
              onClick={() => router.push('/agents')}
              className="text-sm text-gray-400 hover:text-gray-300 transition-colors flex items-center space-x-1"
            >
              <Users size={16} />
              <span className="hidden sm:inline">Agents</span>
            </button>
            <button
              onClick={() => router.push('/docs')}
              className="hidden md:inline text-sm text-gray-400 hover:text-gray-300 transition-colors"
            >
              Docs
            </button>
            <button
              onClick={() => router.push('/api-reference')}
              className="hidden lg:inline text-sm text-gray-400 hover:text-gray-300 transition-colors"
            >
              API reference
            </button>
            <a
              href="#"
              className="hidden lg:inline text-sm text-gray-400 hover:text-gray-300 transition-colors"
            >
              Support
            </a>
            <button
              onClick={() => router.push('/settings')}
              className="p-2 text-gray-400 hover:text-gray-300 transition-colors"
            >
              <Settings size={18} />
            </button>
            <button className="hidden sm:inline px-3 py-1.5 text-sm bg-purple-600 text-white rounded-lg hover:bg-purple-700 transition-colors">
              Deploy
            </button>
            <div className="flex items-center space-x-2">
              <div className="w-2 h-2 bg-green-500 rounded-full"></div>
              <span className="hidden sm:inline text-sm text-green-400">Connected</span>
            </div>
          </div>
        </div>
      </header>

      {/* Main Layout */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left Sidebar - Agent Settings */}
        {leftSidebarOpen ? (
          <div className="w-80 flex-shrink-0 border-r border-gray-800 dark:border-gray-800 hidden lg:block">
            <AgentSettingsPanel
              agentID={agentID}
              onAgentChange={setAgentID}
              onUpdate={handleMessageSent}
            />
          </div>
        ) : null}

        {/* Mobile Left Sidebar Overlay */}
        {leftSidebarOpen && (
          <div className="lg:hidden fixed inset-0 z-40 flex">
            <div className="w-80 max-w-[90vw] bg-gray-900 border-r border-gray-800 relative">
              <button
                onClick={() => setLeftSidebarOpen(false)}
                className="absolute top-2 right-2 z-50 p-2 text-gray-400 hover:text-white bg-gray-800 rounded"
              >
                <X size={18} />
              </button>
              <AgentSettingsPanel
                agentID={agentID}
                onAgentChange={setAgentID}
                onUpdate={handleMessageSent}
              />
            </div>
            <div
              className="flex-1 bg-black bg-opacity-50"
              onClick={() => setLeftSidebarOpen(false)}
            />
          </div>
        )}

        {/* Toggle button for left sidebar */}
        {!leftSidebarOpen && (
          <button
            onClick={() => setLeftSidebarOpen(true)}
            className="fixed left-0 top-1/2 -translate-y-1/2 z-50 p-2 bg-gray-800 border-r border-t border-b border-gray-700 rounded-r-lg text-gray-400 hover:text-white hover:bg-gray-700 transition-colors"
            aria-label="Open left sidebar"
          >
            <ChevronRight size={20} />
          </button>
        )}

        {/* Center - Chat Area */}
        <div className="flex-1 flex-shrink-0 min-w-0">
          <ChatArea
            agentID={agentID}
            userID={DEFAULT_USER_ID}
            onMessageSent={handleMessageSent}
          />
        </div>

        {/* Toggle button for right sidebar */}
        {!rightSidebarOpen && (
          <button
            onClick={() => setRightSidebarOpen(true)}
            className="fixed right-0 top-1/2 -translate-y-1/2 z-50 p-2 bg-gray-800 border-l border-t border-b border-gray-700 rounded-l-lg text-gray-400 hover:text-white hover:bg-gray-700 transition-colors"
            aria-label="Open right sidebar"
          >
            <ChevronLeft size={20} />
          </button>
        )}

        {/* Right Sidebar - Context Window & Memory */}
        {rightSidebarOpen ? (
          <div className="w-96 flex-shrink-0 border-l border-gray-800 dark:border-gray-800 hidden lg:block">
            <ContextWindowPanel
              key={refreshKey}
              agentID={agentID}
              onUpdate={handleMessageSent}
            />
          </div>
        ) : null}

        {/* Mobile Right Sidebar Overlay */}
        {rightSidebarOpen && (
          <div className="lg:hidden fixed inset-0 z-40 flex">
            <div
              className="flex-1 bg-black bg-opacity-50"
              onClick={() => setRightSidebarOpen(false)}
            />
            <div className="w-96 max-w-[90vw] bg-gray-900 border-l border-gray-800 relative">
              <button
                onClick={() => setRightSidebarOpen(false)}
                className="absolute top-2 left-2 z-50 p-2 text-gray-400 hover:text-white bg-gray-800 rounded"
              >
                <X size={18} />
              </button>
              <ContextWindowPanel
                key={refreshKey}
                agentID={agentID}
                onUpdate={handleMessageSent}
              />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export default function Home() {
  return (
    <Suspense fallback={<div className="h-screen flex items-center justify-center bg-gray-950 text-white">Loading...</div>}>
      <HomeContent />
    </Suspense>
  );
}
