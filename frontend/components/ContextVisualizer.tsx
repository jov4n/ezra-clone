'use client';

import { useState, useEffect } from 'react';
import ChatInterface from './ChatInterface';
import StateInspector from './StateInspector';
import MemoryEditor from './MemoryEditor';

interface ContextVisualizerProps {
  agentID: string;
  userID: string;
}

export default function ContextVisualizer({ agentID, userID }: ContextVisualizerProps) {
  const [refreshKey, setRefreshKey] = useState(0);
  const [activeTab, setActiveTab] = useState<'state' | 'editor'>('state');

  const handleMessageSent = () => {
    // Trigger state refresh
    setRefreshKey((prev) => prev + 1);
  };

  const handleMemoryUpdate = () => {
    // Trigger state refresh
    setRefreshKey((prev) => prev + 1);
  };

  return (
    <div className="h-screen flex flex-col bg-gray-100 dark:bg-gray-950">
      <div className="flex-1 flex overflow-hidden">
        {/* Left Panel - Chat */}
        <div className="w-1/2 border-r border-gray-200 dark:border-gray-700">
          <ChatInterface
            agentID={agentID}
            userID={userID}
            onMessageSent={handleMessageSent}
          />
        </div>

        {/* Right Panel - State or Editor */}
        <div className="w-1/2 flex flex-col">
          {/* Tab Selector */}
          <div className="flex border-b border-gray-200 dark:border-gray-700">
            <button
              onClick={() => setActiveTab('state')}
              className={`flex-1 px-4 py-2 text-sm font-medium ${
                activeTab === 'state'
                  ? 'bg-blue-500 text-white'
                  : 'bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700'
              }`}
            >
              State Inspector
            </button>
            <button
              onClick={() => setActiveTab('editor')}
              className={`flex-1 px-4 py-2 text-sm font-medium ${
                activeTab === 'editor'
                  ? 'bg-blue-500 text-white'
                  : 'bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700'
              }`}
            >
              Memory Editor
            </button>
          </div>

          {/* Content */}
          <div className="flex-1 overflow-hidden">
            {activeTab === 'state' ? (
              <StateInspector
                key={refreshKey}
                agentID={agentID}
                highlightChanges={refreshKey > 0}
              />
            ) : (
              <MemoryEditor
                key={refreshKey}
                agentID={agentID}
                onUpdate={handleMemoryUpdate}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

