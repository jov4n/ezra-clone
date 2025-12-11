'use client';

import { useState, useRef, useEffect } from 'react';
import { Send, User, Code2, List, ChevronDown, ChevronUp } from 'lucide-react';
import { sendChatMessage, ChatResponse, ToolCall, getContextStats } from '@/lib/api';
import Badge from '@/components/ui/Badge';

interface Message {
  role: 'user' | 'agent';
  content: string;
  timestamp: Date;
  duration?: number;
  toolCalls?: ToolCall[];
  thinking?: string;
  reasoning?: string;
}

interface ExtendedToolCall extends ToolCall {
  result?: string;
  status?: 'pending' | 'executing' | 'completed' | 'error';
  request?: Record<string, any>;
  response?: Record<string, any>;
}

interface ChatAreaProps {
  agentID: string;
  userID: string;
  onMessageSent?: () => void;
}

export default function ChatArea({ agentID, userID, onMessageSent }: ChatAreaProps) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [tokenUsage, setTokenUsage] = useState({ used: 0, total: 8192 });
  const [showVariables, setShowVariables] = useState(false);
  const [expandedToolCalls, setExpandedToolCalls] = useState<Set<number>>(new Set());
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    loadTokenUsage();
    const interval = setInterval(loadTokenUsage, 5000);
    return () => clearInterval(interval);
  }, [agentID]);

  const loadTokenUsage = async () => {
    try {
      const stats = await getContextStats(agentID);
      setTokenUsage({ used: stats.used_tokens, total: stats.total_tokens });
    } catch (error) {
      console.error('Failed to load token usage:', error);
    }
  };

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  const handleSend = async () => {
    if (!input.trim() || loading) return;

    const userMessage: Message = {
      role: 'user',
      content: input,
      timestamp: new Date(),
    };

    setMessages((prev) => [...prev, userMessage]);
    const messageToSend = input;
    setInput('');
    setLoading(true);

    const startTime = Date.now();

    try {
      const response: ChatResponse = await sendChatMessage(agentID, messageToSend, userID);
      const duration = ((Date.now() - startTime) / 1000).toFixed(1);
      
      if (!response.ignored) {
        const agentMessage: Message = {
          role: 'agent',
          content: response.content || '',
          timestamp: new Date(),
          duration: parseFloat(duration),
          toolCalls: response.tool_calls || [],
          reasoning: response.content?.includes('Reasoning:') ? 'Internal reasoning process' : undefined,
        };
        setMessages((prev) => [...prev, agentMessage]);
      }

      if (onMessageSent) {
        onMessageSent();
      }
      await loadTokenUsage();
    } catch (error) {
      console.error('Failed to send message:', error);
      const errorMessage: Message = {
        role: 'agent',
        content: 'Sorry, I encountered an error processing your message.',
        timestamp: new Date(),
      };
      setMessages((prev) => [...prev, errorMessage]);
    } finally {
      setLoading(false);
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const formatTime = (date: Date) => {
    return date.toLocaleTimeString('en-US', { 
      hour: 'numeric', 
      minute: '2-digit',
      second: '2-digit',
      hour12: true 
    });
  };

  const toggleToolCall = (index: number) => {
    setExpandedToolCalls(prev => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  };

  return (
    <div className="flex flex-col h-full bg-gray-950 dark:bg-gray-950">
      {/* Token Usage Bar */}
      <div className="px-6 py-2.5 bg-gray-900 dark:bg-gray-900 border-b border-gray-800 dark:border-gray-800">
        <div className="flex items-center justify-between text-sm">
          <div className="flex items-center space-x-3">
            <Badge variant="success">Deployed</Badge>
            {loading && (
              <span className="text-gray-400">Processing...</span>
            )}
            {!loading && (
              <>
                <span className="text-gray-400">
                  {tokenUsage.used > 0 
                    ? `${(tokenUsage.used / 1000).toFixed(1)}k / ${(tokenUsage.total / 1000).toFixed(0)}k tokens`
                    : 'Ready'}
                </span>
              </>
            )}
          </div>
          <button
            onClick={() => setShowVariables(!showVariables)}
            className="flex items-center space-x-2 px-3 py-1 text-xs text-gray-400 hover:text-gray-300 border border-gray-700 rounded hover:bg-gray-800 transition-colors"
          >
            {showVariables ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
            <Code2 size={14} />
            <span>Variables</span>
          </button>
        </div>
      </div>

      {/* Variables Panel */}
      {showVariables && (
        <div className="px-6 py-3 bg-gray-900 border-b border-gray-800">
          <div className="text-xs text-gray-400 mb-2">Conversation Variables</div>
          <div className="text-xs text-gray-500 font-mono bg-gray-800 p-2 rounded">
            {JSON.stringify({ agentID, userID, messageCount: messages.length }, null, 2)}
          </div>
        </div>
      )}

      {/* Messages Area */}
      <div className="flex-1 overflow-y-auto p-6 space-y-6">
        {messages.length === 0 && (
          <div className="text-center text-gray-500 dark:text-gray-400 mt-12">
            <p className="text-lg mb-2">Start a conversation</p>
            <p className="text-sm">Send a message to interact with the agent</p>
          </div>
        )}
        
        {messages.map((msg, idx) => (
          <div
            key={idx}
            className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
          >
            <div className={`max-w-[85%] ${msg.role === 'user' ? 'text-right' : ''}`}>
              {/* Reasoning Tag */}
              {msg.role === 'agent' && msg.reasoning && (
                <div className="mb-2">
                  <Badge variant="primary" className="bg-purple-600 text-white">
                    Reasoning
                  </Badge>
                </div>
              )}

              {/* Tool Calls */}
              {msg.role === 'agent' && msg.toolCalls && msg.toolCalls.length > 0 && (
                <div className="mb-2 space-y-1.5">
                  {msg.toolCalls.map((toolCall, toolIdx) => {
                    const isExpanded = expandedToolCalls.has(idx * 1000 + toolIdx);
                    const extended = toolCall as ExtendedToolCall;
                    return (
                      <div
                        key={toolCall.id || toolIdx}
                        className="bg-gray-800 dark:bg-gray-800 border border-gray-700 rounded-md overflow-hidden"
                      >
                        <button
                          onClick={() => toggleToolCall(idx * 1000 + toolIdx)}
                          className="w-full flex items-center justify-between px-3 py-2 hover:bg-gray-750 transition-colors"
                        >
                          <div className="flex items-center space-x-2">
                            <span className="font-mono font-semibold text-blue-400 text-xs">
                              {toolCall.name}
                            </span>
                            {extended.status === 'executing' && (
                              <span className="text-blue-400 animate-pulse text-xs">executing...</span>
                            )}
                            {extended.status === 'completed' && (
                              <span className="text-green-400 text-xs">✓</span>
                            )}
                            {extended.status === 'error' && (
                              <span className="text-red-400 text-xs">✗</span>
                            )}
                          </div>
                          {isExpanded ? (
                            <ChevronUp size={14} className="text-gray-400" />
                          ) : (
                            <ChevronDown size={14} className="text-gray-400" />
                          )}
                        </button>
                        {isExpanded && (
                          <div className="px-3 py-2 border-t border-gray-700 space-y-2">
                            <div>
                              <div className="text-xs text-gray-500 mb-1 font-semibold">Request:</div>
                              <pre className="text-xs bg-gray-900 p-2 rounded overflow-x-auto text-gray-300 font-mono">
                                {JSON.stringify(toolCall.arguments || {}, null, 2)}
                              </pre>
                            </div>
                            {extended.response && (
                              <div>
                                <div className="text-xs text-gray-500 mb-1 font-semibold">Response:</div>
                                <pre className="text-xs bg-gray-900 p-2 rounded overflow-x-auto text-gray-300 font-mono">
                                  {JSON.stringify(extended.response, null, 2)}
                                </pre>
                              </div>
                            )}
                            {extended.result && (
                              <div>
                                <div className="text-xs text-gray-500 mb-1 font-semibold">Result:</div>
                                <div className="text-xs bg-gray-900 p-2 rounded text-gray-300">
                                  {extended.result}
                                </div>
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
              
              {/* Message Content */}
              {msg.content && (
                <div
                  className={`rounded-lg p-4 ${
                    msg.role === 'user'
                      ? 'bg-blue-600 text-white'
                      : 'bg-gray-800 dark:bg-gray-800 text-gray-100'
                  }`}
                >
                  <div className="whitespace-pre-wrap break-words prose prose-sm max-w-none dark:prose-invert">
                    {msg.content.split('\n').map((line, i) => {
                      if (line.startsWith('**') && line.endsWith('**')) {
                        return <strong key={i}>{line.slice(2, -2)}</strong>;
                      }
                      if (line.startsWith('- ') || line.startsWith('* ')) {
                        return <div key={i} className="ml-4">• {line.slice(2)}</div>;
                      }
                      return <div key={i}>{line}</div>;
                    })}
                  </div>
                </div>
              )}
              
              {/* Timestamp and Metadata */}
              <div className={`flex items-center space-x-2 mt-2 text-xs text-gray-500 ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                <span>{formatTime(msg.timestamp)}</span>
                {msg.duration && (
                  <>
                    <span>•</span>
                    <span>{msg.duration}s</span>
                  </>
                )}
                {msg.role === 'agent' && idx === messages.length - 1 && !loading && (
                  <>
                    <span>•</span>
                    <button className="text-blue-400 hover:text-blue-300 font-mono">
                      end_turn
                    </button>
                  </>
                )}
              </div>
            </div>
          </div>
        ))}
        
        {loading && (
          <div className="flex justify-start">
            <div className="bg-gray-800 rounded-lg p-4">
              <div className="flex space-x-2">
                <div className="w-2 h-2 bg-gray-400 rounded-full animate-bounce"></div>
                <div className="w-2 h-2 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '0.2s' }}></div>
                <div className="w-2 h-2 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '0.4s' }}></div>
              </div>
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Input Area */}
      <div className="border-t border-gray-800 dark:border-gray-800 p-4 bg-gray-900 dark:bg-gray-900">
        <div className="flex items-end space-x-3">
          <div className="flex-1 relative">
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyPress={handleKeyPress}
              placeholder="Type a message..."
              rows={1}
              className="w-full px-4 py-3 pr-12 border border-gray-700 dark:border-gray-700 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-800 dark:text-white resize-none text-sm"
              disabled={loading}
              style={{ minHeight: '44px', maxHeight: '120px' }}
            />
          </div>
          <select className="px-3 py-2 border border-gray-700 dark:border-gray-700 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-800 dark:text-white text-sm">
            <option>User</option>
          </select>
          <button
            onClick={handleSend}
            disabled={loading || !input.trim()}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center space-x-2 transition-colors"
          >
            <Send size={18} />
          </button>
        </div>
      </div>
    </div>
  );
}
