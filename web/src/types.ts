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
  user?: string;
  status: 'connecting' | 'connected' | 'disconnected' | 'error';
  errorMsg?: string;
}

export interface EnrollmentToken {
  id: string;
  name: string;
  token: string;
  created_at: string;
  expires_at?: string;
  revoked: boolean;
}

export interface UserAccount {
  id: string;
  username: string;
  role: 'admin' | 'operator' | 'viewer';
  created_at: string;
  last_login_at?: string;
}
