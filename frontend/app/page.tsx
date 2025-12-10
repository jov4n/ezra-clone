'use client';

import { useState } from 'react';
import ChatArea from '@/components/ChatArea';
import ToolsSidebar from '@/components/ToolsSidebar';
import CoreMemorySidebar from '@/components/CoreMemorySidebar';
import { Play } from 'lucide-react';
import { sendChatMessage } from '@/lib/api';

const DEFAULT_AGENT_ID = 'Ezra';
const DEFAULT_USER_ID = 'developer';

export default function Home() {
  const [agentID, setAgentID] = useState(DEFAULT_AGENT_ID);
  const [runningEval, setRunningEval] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);

  const runEval = async () => {
    setRunningEval(true);
    try {
      const scenarios = [
        "Hello! What's your name?",
        "Change your name to Jarvis",
        "What's your name now?",
        "Update your personality to be more friendly",
      ];

      for (const scenario of scenarios) {
        await sendChatMessage(agentID, scenario, DEFAULT_USER_ID);
        await new Promise((resolve) => setTimeout(resolve, 1000));
      }

      alert('Eval completed! Check the chat and memory panels.');
      setRefreshKey(prev => prev + 1);
    } catch (error) {
      console.error('Eval failed:', error);
      alert('Eval failed. Check console for details.');
    } finally {
      setRunningEval(false);
    }
  };

  const handleMessageSent = () => {
    // Only refresh the CoreMemorySidebar, not the ChatArea
    // ChatArea manages its own message state
    setRefreshKey(prev => prev + 1);
  };

  return (
    <div className="h-screen flex flex-col bg-gray-100 dark:bg-gray-950">
      {/* Top Header */}
      <header className="bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 px-6 py-3">
        <div className="flex justify-between items-center">
          <div>
            <h1 className="text-lg font-semibold text-gray-900 dark:text-white">
              Ezra Clone - Agent Development Environment
            </h1>
          </div>
          <div className="flex items-center space-x-4">
            <div className="flex items-center space-x-2">
              <label className="text-sm text-gray-700 dark:text-gray-300">Agent:</label>
              <input
                type="text"
                value={agentID}
                onChange={(e) => setAgentID(e.target.value)}
                className="px-3 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-800 dark:text-white"
              />
            </div>
            <button
              onClick={runEval}
              disabled={runningEval}
              className="px-3 py-1.5 text-sm bg-green-500 text-white rounded-lg hover:bg-green-600 disabled:opacity-50 disabled:cursor-not-allowed flex items-center space-x-2"
            >
              <Play size={16} />
              <span>{runningEval ? 'Running...' : 'Run Eval'}</span>
            </button>
          </div>
        </div>
      </header>

      {/* Main Layout */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left Sidebar - Tools */}
        <div className="w-64 flex-shrink-0">
          <ToolsSidebar />
        </div>

        {/* Center - Chat Area */}
        <div className="flex-1 flex-shrink-0 min-w-0">
          <ChatArea
            agentID={agentID}
            userID={DEFAULT_USER_ID}
            onMessageSent={handleMessageSent}
          />
        </div>

        {/* Right Sidebar - Core Memory */}
        <div className="w-80 flex-shrink-0">
          <CoreMemorySidebar key={refreshKey} agentID={agentID} />
        </div>
      </div>
    </div>
  );
}
