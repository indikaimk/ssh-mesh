export interface AgentInfo {
  id: string;
  hostname: string;
  os: string;
  arch: string;
  private_ip: string;
  tags?: Record<string, string>;
  version: string;
  connected_at: string;
  last_seen_at: string;
  status: 'online' | 'offline';
}

export interface TerminalTab {
  id: string;
  agentId: string;
  hostname: string;
  privateIp: string;
  status: 'connecting' | 'connected' | 'disconnected' | 'error';
  errorMsg?: string;
}
