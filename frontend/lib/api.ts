import axios from 'axios';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Types matching backend models
export interface AgentIdentity {
  name: string;
  personality: string;
  capabilities: string[];
}

export interface MemoryBlock {
  name: string;
  content: string;
  updated_at: string;
}

export interface ArchivalPointer {
  summary: string;
  timestamp: string;
  relevance_score: number;
}

export interface ContextWindow {
  identity: AgentIdentity;
  core_memory: MemoryBlock[];
  archival_refs: ArchivalPointer[];
  user_context: Record<string, any>;
}

export interface ChatResponse {
  content: string;
  tool_calls: ToolCall[];
  ignored: boolean;
}

export interface ToolCall {
  id: string;
  name: string;
  arguments: Record<string, any>;
}

export interface ChatRequest {
  message: string;
  user_id: string;
}

export interface MemoryUpdateRequest {
  block_name: string;
  content: string;
}

// API Functions
export async function fetchAgentState(agentID: string): Promise<ContextWindow> {
  const response = await apiClient.get<ContextWindow>(`/api/agent/${agentID}/state`);
  return response.data;
}

export async function sendChatMessage(
  agentID: string,
  message: string,
  userID: string
): Promise<ChatResponse> {
  const response = await apiClient.post<ChatResponse>(`/api/agent/${agentID}/chat`, {
    message,
    user_id: userID,
  } as ChatRequest);
  return response.data;
}

export async function updateMemoryBlock(
  agentID: string,
  blockName: string,
  content: string
): Promise<void> {
  await apiClient.post(`/api/memory/${agentID}/update`, {
    block_name: blockName,
    content,
  } as MemoryUpdateRequest);
}

