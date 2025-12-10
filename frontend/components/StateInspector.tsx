'use client';

import { useEffect, useState } from 'react';
import { ContextWindow } from '@/lib/api';
import { fetchAgentState } from '@/lib/api';

interface StateInspectorProps {
  agentID: string;
  highlightChanges?: boolean;
}

export default function StateInspector({ agentID, highlightChanges }: StateInspectorProps) {
  const [state, setState] = useState<ContextWindow | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdate, setLastUpdate] = useState<Date>(new Date());

  const loadState = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await fetchAgentState(agentID);
      setState(data);
      setLastUpdate(new Date());
    } catch (err) {
      setError('Failed to load agent state');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadState();
    // Poll for updates every 2 seconds
    const interval = setInterval(loadState, 2000);
    return () => clearInterval(interval);
  }, [agentID]);

  if (loading && !state) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-gray-500 dark:text-gray-400">Loading state...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-red-500">{error}</div>
      </div>
    );
  }

  if (!state) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-gray-500 dark:text-gray-400">No state available</div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-white dark:bg-gray-900">
      <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Agent State</h2>
        <div className="text-sm text-gray-500 dark:text-gray-400">
          Updated: {lastUpdate.toLocaleTimeString()}
        </div>
      </div>
      <div className="flex-1 overflow-auto p-4">
        <div
          className={`transition-all duration-300 rounded-lg ${
            highlightChanges ? 'bg-yellow-100 dark:bg-yellow-900' : ''
          }`}
        >
          <pre className="text-sm font-mono bg-gray-900 text-gray-100 p-4 rounded-lg overflow-auto">
            {JSON.stringify(state, null, 2)}
          </pre>
        </div>
      </div>
    </div>
  );
}

