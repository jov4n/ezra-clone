'use client';

import { useState, useRef, useEffect } from 'react';
import { Send } from 'lucide-react';
import { sendChatMessage, ChatResponse, ToolCall } from '@/lib/api';

interface Message {
  role: 'user' | 'agent';
  content: string;
  timestamp: Date;
  duration?: number;
  toolCalls?: ToolCall[];
  thinking?: string;
}

interface ExtendedToolCall extends ToolCall {
  result?: string;
  status?: 'pending' | 'executing' | 'completed' | 'error';
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
  const [tokenUsage, setTokenUsage] = useState({ used: 0, total: 90000 });
  const messagesEndRef = useRef<HTMLDivElement>(null);

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
        };
        setMessages((prev) => [...prev, agentMessage]);
      }

      if (onMessageSent) {
        onMessageSent();
      }
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

  return (
    <div className="flex flex-col h-full bg-white dark:bg-gray-950">
      {/* Token Usage Bar */}
      <div className="px-6 py-2.5 bg-yellow-50 dark:bg-yellow-900/20 border-b border-yellow-200 dark:border-yellow-800">
        <div className="flex items-center justify-between text-sm">
          <div className="flex items-center space-x-3">
            <span className="text-yellow-800 dark:text-yellow-200 font-semibold">
              ▲ Deployed
            </span>
            {loading && (
              <span className="text-gray-600 dark:text-gray-400">
                Processing...
              </span>
            )}
            {!loading && (
              <>
                <span className="text-gray-600 dark:text-gray-400">
                  {tokenUsage.used > 0 
                    ? `${(tokenUsage.used / 1000).toFixed(1)}k of ${(tokenUsage.total / 1000).toFixed(0)}k tokens`
                    : 'Ready'}
                </span>
                {tokenUsage.used > 0 && (
                  <span className="text-gray-500 dark:text-gray-500">
                    {Math.round((1 - tokenUsage.used / tokenUsage.total) * 100)}% left
                  </span>
                )}
              </>
            )}
          </div>
        </div>
      </div>

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
              {msg.role === 'agent' && msg.toolCalls && msg.toolCalls.length > 0 && (
                <div className="mb-2 space-y-1.5">
                  {msg.toolCalls.map((toolCall) => {
                    const extended = toolCall as ExtendedToolCall;
                    return (
                      <div
                        key={toolCall.id}
                        className="inline-flex items-center space-x-2 px-3 py-1.5 bg-blue-50 dark:bg-blue-900/30 border border-blue-200 dark:border-blue-800 rounded-md text-xs"
                      >
                        <span className="font-mono font-semibold text-blue-700 dark:text-blue-300">
                          {toolCall.name}
                        </span>
                        {extended.status === 'executing' && (
                          <span className="text-blue-600 dark:text-blue-400 animate-pulse">executing...</span>
                        )}
                        {extended.status === 'completed' && (
                          <span className="text-green-600 dark:text-green-400">✓</span>
                        )}
                        {extended.status === 'error' && (
                          <span className="text-red-600 dark:text-red-400">✗</span>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
              
              {msg.content && (
                <div
                  className={`rounded-lg p-4 ${
                    msg.role === 'user'
                      ? 'bg-blue-500 text-white'
                      : 'bg-gray-100 dark:bg-gray-800 text-gray-900 dark:text-gray-100'
                  }`}
                >
                  <div className="whitespace-pre-wrap break-words prose prose-sm max-w-none dark:prose-invert">
                    {msg.content.split('\n').map((line, i) => {
                      // Simple markdown-like formatting
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
              
              <div className="flex items-center space-x-2 mt-2 text-xs text-gray-500 dark:text-gray-400">
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
                    <button className="text-blue-500 hover:text-blue-600 dark:hover:text-blue-400 font-mono">
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
            <div className="bg-gray-100 dark:bg-gray-800 rounded-lg p-4">
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
      <div className="border-t border-gray-200 dark:border-gray-700 p-4 bg-gray-50 dark:bg-gray-900">
        <div className="flex items-end space-x-3">
          <div className="flex-1 relative">
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyPress={handleKeyPress}
              placeholder="Type your message here..."
              rows={1}
              className="w-full px-4 py-3 pr-12 border border-gray-300 dark:border-gray-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-800 dark:text-white resize-none"
              disabled={loading}
              style={{ minHeight: '44px', maxHeight: '120px' }}
            />
            <button className="absolute right-2 bottom-2 p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
              <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
                <path d="M10 2C6.48 2 3.5 4.98 3.5 8.5c0 1.29.41 2.49 1.11 3.47L2 18l6.03-2.61C9.51 16.09 9.75 16.5 10 16.5c3.52 0 6.5-2.98 6.5-6.5S13.52 2 10 2zm0 11.5c-.83 0-1.5-.67-1.5-1.5S9.17 10.5 10 10.5s1.5.67 1.5 1.5-.67 1.5-1.5 1.5zm0-5c-.83 0-1.5-.67-1.5-1.5S9.17 3.5 10 3.5s1.5.67 1.5 1.5-.67 1.5-1.5 1.5z"/>
              </svg>
            </button>
          </div>
          <select className="px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-800 dark:text-white text-sm">
            <option>User</option>
          </select>
          <button
            onClick={handleSend}
            disabled={loading || !input.trim()}
            className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed flex items-center space-x-2"
          >
            <Send size={18} />
          </button>
        </div>
      </div>
    </div>
  );
}

