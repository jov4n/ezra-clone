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

// New types for ADE features
export interface Agent {
  id: string;
  name: string;
  created_at: string;
}

export interface AgentConfig {
  model: string;
  system_instructions: string;
}

export interface ContextStats {
  used_tokens: number;
  total_tokens: number;
}

export interface ArchivalMemory {
  id: string;
  summary: string;
  content: string;
  timestamp: string;
  relevance_score: number;
}

export interface Tool {
  type: string;
  function: {
    name: string;
    description: string;
    parameters: Record<string, any>;
  };
}

export interface Fact {
  id: string;
  content: string;
  source?: string;
  confidence: number;
  created_at: string;
}

export interface Topic {
  id: string;
  name: string;
  description?: string;
}

export interface Message {
  id: string;
  content: string;
  role: string;
  platform: string;
  timestamp: string;
}

export interface Conversation {
  id: string;
  channel_id?: string;
  platform: string;
  started_at: string;
}

export interface User {
  id: string;
  discord_id?: string;
  discord_username?: string;
  web_id?: string;
  preferred_language?: string;
  first_seen: string;
  last_seen: string;
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

// New API functions for ADE features
export async function listAgents(): Promise<Agent[]> {
  const response = await apiClient.get<Agent[]>('/api/agents');
  return response.data;
}

export async function getAgentConfig(agentID: string): Promise<AgentConfig> {
  const response = await apiClient.get<AgentConfig>(`/api/agent/${agentID}/config`);
  return response.data;
}

export async function updateAgentConfig(
  agentID: string,
  config: AgentConfig
): Promise<void> {
  await apiClient.put(`/api/agent/${agentID}/config`, config);
}

export async function getAgentTools(agentID: string): Promise<Tool[]> {
  const response = await apiClient.get<Tool[]>(`/api/agent/${agentID}/tools`);
  return response.data;
}

export async function getContextStats(agentID: string): Promise<ContextStats> {
  const response = await apiClient.get<ContextStats>(`/api/agent/${agentID}/context`);
  return response.data;
}

export async function getArchivalMemories(agentID: string): Promise<ArchivalMemory[]> {
  const response = await apiClient.get<ArchivalMemory[]>(`/api/agent/${agentID}/archival-memories`);
  return response.data;
}

export async function createArchivalMemory(
  agentID: string,
  memory: Omit<ArchivalMemory, 'timestamp' | 'id'>
): Promise<void> {
  await apiClient.post(`/api/agent/${agentID}/archival-memories`, {
    ...memory,
    timestamp: new Date().toISOString(),
  });
}

export async function deleteArchivalMemory(
  agentID: string,
  memoryID: string
): Promise<void> {
  await apiClient.delete(`/api/agent/${agentID}/archival-memories/${memoryID}`);
}

export async function createAgent(
  name: string,
  model?: string,
  systemInstructions?: string
): Promise<Agent> {
  const response = await apiClient.post<Agent>('/api/agents', {
    name,
    model,
    system_instructions: systemInstructions,
  });
  return response.data;
}

export async function deleteMemoryBlock(
  agentID: string,
  blockName: string
): Promise<void> {
  await apiClient.delete(`/api/memory/${agentID}/block/${encodeURIComponent(blockName)}`);
}

export async function fetchAllFacts(agentID: string): Promise<Fact[]> {
  const response = await apiClient.get<Fact[]>(`/api/agent/${agentID}/facts`);
  return response.data;
}

export async function fetchAllTopics(agentID: string): Promise<Topic[]> {
  const response = await apiClient.get<Topic[]>(`/api/agent/${agentID}/topics`);
  return response.data;
}

export async function fetchAllMessages(agentID: string, limit: number = 100): Promise<Message[]> {
  const response = await apiClient.get<Message[]>(`/api/agent/${agentID}/messages`, {
    params: { limit },
  });
  return response.data;
}

export async function fetchAllConversations(agentID: string, limit: number = 50): Promise<Conversation[]> {
  const response = await apiClient.get<Conversation[]>(`/api/agent/${agentID}/conversations`, {
    params: { limit },
  });
  return response.data;
}

export async function fetchAllUsers(agentID: string): Promise<User[]> {
  const response = await apiClient.get<User[]>(`/api/agent/${agentID}/users`);
  return response.data;
}

