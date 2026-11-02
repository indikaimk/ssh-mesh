import React, { useState, useEffect } from 'react';
import { AgentInfo, TerminalTab } from './types';
import { TerminalView } from './components/TerminalView';
import { EnrollModal } from './components/EnrollModal';
import { LoginView } from './components/LoginView';
import { UsersModal } from './components/UsersModal';
import { AuditModal } from './components/AuditModal';
import {
  Terminal as TerminalIcon,
  Server,
  Plus,
  RefreshCw,
  Search,
  X,
  Shield,
  Activity,
  ChevronRight,
  LogOut,
  User,
  Users,
  Tag,
  FileText,
} from 'lucide-react';

export const App: React.FC = () => {
  const [authToken, setAuthToken] = useState<string | null>(() => localStorage.getItem('ssh_mesh_jwt'));
  const [currentUser, setCurrentUser] = useState<string>(() => localStorage.getItem('ssh_mesh_user') || 'admin');
  const [userRole, setUserRole] = useState<string>(() => localStorage.getItem('ssh_mesh_role') || 'operator');
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [tabs, setTabs] = useState<TerminalTab[]>([]);
  const [activeTabId, setActiveTabId] = useState<string | null>(null);
  const [isEnrollModalOpen, setIsEnrollModalOpen] = useState(false);
  const [isUsersModalOpen, setIsUsersModalOpen] = useState(false);
  const [isAuditModalOpen, setIsAuditModalOpen] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);

  // Target User prompt modal state
  const [targetUserModalAgent, setTargetUserModalAgent] = useState<AgentInfo | null>(null);
  const [selectedUser, setSelectedUser] = useState<string>('default');
  const [customUser, setCustomUser] = useState<string>('');

  // Node Tag management modal state (Admin)
  const [tagModalAgent, setTagModalAgent] = useState<AgentInfo | null>(null);
  const [editingTags, setEditingTags] = useState<Record<string, string>>({});
  const [newTagKey, setNewTagKey] = useState('');
  const [newTagValue, setNewTagValue] = useState('');
  const [isSavingTags, setIsSavingTags] = useState(false);

  const openTagModal = (e: React.MouseEvent, agent: AgentInfo) => {
    e.stopPropagation();
    setTagModalAgent(agent);
    setEditingTags({ ...(agent.tags || {}) });
    setNewTagKey('');
    setNewTagValue('');
  };

  const handleSaveNodeTags = async () => {
    if (!tagModalAgent || !authToken) return;
    setIsSavingTags(true);

    const updatedTags = { ...editingTags };
    if (newTagKey.trim() && newTagValue.trim()) {
      updatedTags[newTagKey.trim()] = newTagValue.trim();
    }

    try {
      const res = await fetch('/api/v1/agents/tags', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${authToken}`,
        },
        body: JSON.stringify({
          agent_id: tagModalAgent.id,
          tags: updatedTags,
        }),
      });
      if (res.ok) {
        setTagModalAgent(null);
        await fetchAgents();
      } else {
        const text = await res.text();
        alert(text || 'Failed to update node tags');
      }
    } catch (err: any) {
      alert(err.message || 'Failed to save tags');
    } finally {
      setIsSavingTags(false);
    }
  };

  const fetchAgents = async () => {
    if (!authToken) return;
    setIsRefreshing(true);
    try {
      const res = await fetch('/api/v1/agents', {
        headers: { Authorization: `Bearer ${authToken}` },
      });
      if (res.status === 401) {
        handleLogout();
        return;
      }
      if (res.ok) {
        const data = await res.json();
        setAgents(data.agents || []);
      }
    } catch (err) {
      console.error('Failed to fetch agents:', err);
    } finally {
      setIsRefreshing(false);
    }
  };

  useEffect(() => {
    if (authToken) {
      fetchAgents();
      const interval = setInterval(fetchAgents, 3000);
      return () => clearInterval(interval);
    }
  }, [authToken]);

  const handleLoginSuccess = (token: string, user: string, role: string) => {
    setAuthToken(token);
    setCurrentUser(user);
    setUserRole(role);
  };

  const handleLogout = () => {
    localStorage.removeItem('ssh_mesh_jwt');
    localStorage.removeItem('ssh_mesh_user');
    localStorage.removeItem('ssh_mesh_role');
    setAuthToken(null);
    setTabs([]);
    setActiveTabId(null);
  };

  const handleNodeClick = (agent: AgentInfo) => {
    if (userRole === 'viewer') {
      alert('Viewer accounts have read-only access and cannot open terminal sessions.');
      return;
    }
    setTargetUserModalAgent(agent);
  };

  const openTerminal = (agent: AgentInfo, user?: string) => {
    const finalUser = user && user !== 'default' ? user : undefined;
    const tabKey = finalUser ? `${agent.id}-${finalUser}` : agent.id;

    // Check if an identical tab already exists
    const existing = tabs.find((t) => t.agentId === agent.id && t.user === finalUser);
    if (existing) {
      setActiveTabId(existing.id);
      return;
    }

    const newTab: TerminalTab = {
      id: `tab-${tabKey}-${Date.now()}`,
      agentId: agent.id,
      hostname: agent.hostname,
      privateIp: agent.private_ip,
      user: finalUser,
      status: 'connecting',
    };

    setTabs((prev) => [...prev, newTab]);
    setActiveTabId(newTab.id);
  };

  const closeTab = (tabId: string) => {
    setTabs((prev) => {
      const nextTabs = prev.filter((t) => t.id !== tabId);
      if (activeTabId === tabId) {
        setActiveTabId(nextTabs.length > 0 ? nextTabs[nextTabs.length - 1].id : null);
      }
      return nextTabs;
    });
  };

  if (!authToken) {
    return <LoginView onLoginSuccess={handleLoginSuccess} />;
  }

  const filteredAgents = agents.filter((a) => {
    const q = searchQuery.toLowerCase();
    const tagMatch =
      a.tags &&
      Object.entries(a.tags).some(
        ([k, v]) => k.toLowerCase().includes(q) || v.toLowerCase().includes(q) || `${k}:${v}`.toLowerCase().includes(q) || `${k}=${v}`.toLowerCase().includes(q)
      );
    return (
      a.hostname.toLowerCase().includes(q) ||
      a.private_ip.toLowerCase().includes(q) ||
      a.id.toLowerCase().includes(q) ||
      a.os.toLowerCase().includes(q) ||
      Boolean(tagMatch)
    );
  });

  const onlineCount = agents.filter((a) => a.status === 'online').length;
  const activeTab = tabs.find((t) => t.id === activeTabId);

  return (
    <div className="app-container">
      {/* Top Navbar */}
      <header className="navbar">
        <div className="brand">
          <div className="brand-icon">
            <TerminalIcon size={19} />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span>SSH-MESH</span>
            <span className="badge-tag">REVERSE SSH</span>
          </div>
        </div>

        <div className="navbar-actions">
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              fontSize: '0.82rem',
              color: 'var(--text-muted)',
              marginRight: '8px',
            }}
          >
            <Activity size={15} color="var(--status-online)" />
            <span>
              <strong>{onlineCount}</strong> of <strong>{agents.length}</strong> Nodes Online
            </span>
          </div>

          <button
            className="btn btn-secondary btn-sm"
            onClick={fetchAgents}
            disabled={isRefreshing}
            title="Refresh Node List"
          >
            <RefreshCw size={14} className={isRefreshing ? 'animate-spin' : ''} />
            <span>Refresh</span>
          </button>

          {/* Admin: Users & Audit Management */}
          {userRole === 'admin' && (
            <>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => setIsAuditModalOpen(true)}
                title="Audit Logs, Command History & Session Recordings"
              >
                <FileText size={14} />
                <span>Audit & Logs</span>
              </button>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => setIsUsersModalOpen(true)}
                title="Manage Users & Roles"
              >
                <Users size={14} />
                <span>Team & Users</span>
              </button>
            </>
          )}

          {userRole !== 'viewer' && (
            <button className="btn btn-primary btn-sm" onClick={() => setIsEnrollModalOpen(true)}>
              <Plus size={15} />
              <span>Enroll Node</span>
            </button>
          )}

          <div style={{ width: '1px', height: '24px', background: 'var(--border-subtle)', margin: '0 4px' }} />

          {/* User profile & Logout */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span style={{ fontSize: '0.8rem', color: 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: '6px' }}>
              <User size={14} color="var(--accent-cyan)" />
              <span>{currentUser}</span>
              <span
                className="badge-tag"
                style={{
                  fontSize: '0.68rem',
                  textTransform: 'uppercase',
                  background:
                    userRole === 'admin'
                      ? 'rgba(168, 85, 247, 0.15)'
                      : userRole === 'operator'
                      ? 'rgba(56, 189, 248, 0.15)'
                      : 'rgba(148, 163, 184, 0.15)',
                  color:
                    userRole === 'admin'
                      ? '#c084fc'
                      : userRole === 'operator'
                      ? 'var(--accent-cyan)'
                      : '#94a3b8',
                }}
              >
                {userRole}
              </span>
            </span>
            <button
              className="btn btn-secondary btn-sm"
              onClick={handleLogout}
              title="Sign Out"
              style={{ padding: '6px 8px' }}
            >
              <LogOut size={14} />
            </button>
          </div>
        </div>
      </header>

      {/* Main Workspace */}
      <div className="main-content">
        {/* Left Sidebar: Inventory */}
        <aside className="sidebar">
          <div className="sidebar-header">
            <div className="sidebar-title-row">
              <span className="sidebar-title">
                <Server size={16} />
                <span>Cloud VM Inventory</span>
              </span>
              <span style={{ fontSize: '0.75rem', color: 'var(--text-dim)' }}>
                {filteredAgents.length} nodes
              </span>
            </div>

            <div className="search-input-wrapper">
              <Search size={14} className="search-icon" />
              <input
                type="text"
                className="search-input"
                placeholder="Filter by hostname, IP, OS..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
          </div>

          <div className="node-list">
            {filteredAgents.length === 0 ? (
              <div
                style={{
                  padding: '30px 16px',
                  textAlign: 'center',
                  color: 'var(--text-dim)',
                  fontSize: '0.85rem',
                }}
              >
                {searchQuery ? (
                  'No matching nodes found.'
                ) : (
                  <div>
                    <Server size={28} style={{ opacity: 0.3, marginBottom: '8px' }} />
                    <p>No agents connected yet.</p>
                    {userRole !== 'viewer' && (
                      <button
                        className="btn btn-secondary btn-sm"
                        style={{ marginTop: '12px' }}
                        onClick={() => setIsEnrollModalOpen(true)}
                      >
                        Connect your first VM
                      </button>
                    )}
                  </div>
                )}
              </div>
            ) : (
              filteredAgents.map((agent) => {
                const isCurrentActive = activeTab?.agentId === agent.id;
                return (
                  <div
                    key={agent.id}
                    className={`node-card ${isCurrentActive ? 'active' : ''}`}
                    onClick={() => handleNodeClick(agent)}
                  >
                    <div className="node-header">
                      <div className="node-hostname">
                        <span className={`status-dot ${agent.status}`} />
                        <span>{agent.hostname}</span>
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                        {userRole === 'admin' && (
                          <button
                            type="button"
                            className="tab-close"
                            title="Manage VM Tags"
                            onClick={(e) => openTagModal(e, agent)}
                            style={{ padding: '3px', color: 'var(--text-dim)' }}
                          >
                            <Tag size={13} />
                          </button>
                        )}
                        <ChevronRight size={15} color="var(--text-dim)" />
                      </div>
                    </div>

                    <div className="node-ip-row">
                      <span>{agent.private_ip}</span>
                    </div>

                    {agent.tags && Object.keys(agent.tags).length > 0 && (
                      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap', marginTop: '6px' }}>
                        {Object.entries(agent.tags).map(([k, v]) => (
                          <span
                            key={k}
                            className="meta-chip"
                            style={{
                              color: '#c084fc',
                              borderColor: 'rgba(168, 85, 247, 0.35)',
                              background: 'rgba(168, 85, 247, 0.1)',
                              fontSize: '0.68rem',
                            }}
                          >
                            {k}:{v}
                          </span>
                        ))}
                      </div>
                    )}

                    <div className="node-details">
                      <div className="node-meta-tags">
                        <span className="meta-chip">{agent.os}/{agent.arch}</span>
                        <span className="meta-chip">v{agent.version}</span>
                      </div>
                      <span style={{ textTransform: 'capitalize', color: agent.status === 'online' ? 'var(--status-online)' : 'var(--text-dim)' }}>
                        {agent.status}
                      </span>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </aside>

        {/* Right Area: Terminal Workspace */}
        <main className="terminal-area">
          {tabs.length > 0 ? (
            <>
              {/* Tab Header Bar */}
              <div className="tab-bar">
                {tabs.map((tab) => (
                  <div
                    key={tab.id}
                    className={`terminal-tab ${tab.id === activeTabId ? 'active' : ''}`}
                    onClick={() => setActiveTabId(tab.id)}
                  >
                    <TerminalIcon size={13} color="var(--accent-cyan)" />
                    <span>{tab.hostname}</span>
                    {tab.user && (
                      <span style={{ fontSize: '0.72rem', opacity: 0.8, color: '#c084fc' }}>
                        ({tab.user})
                      </span>
                    )}
                    <span
                      className="tab-close"
                      onClick={(e) => {
                        e.stopPropagation();
                        closeTab(tab.id);
                      }}
                    >
                      <X size={12} />
                    </span>
                  </div>
                ))}
              </div>

              {/* Render Active Terminal */}
              {activeTab && (
                <TerminalView
                  key={activeTab.id}
                  tab={activeTab}
                  authToken={authToken}
                  onClose={closeTab}
                />
              )}
            </>
          ) : (
            /* Welcome / Empty Workspace State */
            <div className="empty-state">
              <div className="empty-icon-wrap">
                <TerminalIcon size={34} />
              </div>
              <h2 style={{ fontSize: '1.4rem', fontWeight: 600 }}>No Terminal Session Active</h2>
              <p style={{ color: 'var(--text-muted)', maxWidth: '440px', fontSize: '0.9rem', lineHeight: '1.5' }}>
                Select a connected node from the inventory on the left to launch an encrypted reverse SSH terminal, or enroll a new VM with private IP.
              </p>
              <div style={{ display: 'flex', gap: '12px', marginTop: '6px' }}>
                {userRole !== 'viewer' ? (
                  <>
                    <button
                      className="btn btn-primary"
                      onClick={() => {
                        if (agents.length > 0) {
                          setTargetUserModalAgent(agents[0]);
                        } else {
                          setIsEnrollModalOpen(true);
                        }
                      }}
                    >
                      <TerminalIcon size={16} />
                      <span>{agents.length > 0 ? 'Connect to ' + agents[0].hostname : 'Enroll Your First VM'}</span>
                    </button>
                    <button className="btn btn-secondary" onClick={() => setIsEnrollModalOpen(true)}>
                      <Shield size={16} />
                      <span>Enroll Instructions</span>
                    </button>
                  </>
                ) : (
                  <div style={{ fontSize: '0.85rem', color: 'var(--text-dim)' }}>
                    Your account has <strong>Viewer (Read-Only)</strong> permissions.
                  </div>
                )}
              </div>
            </div>
          )}
        </main>
      </div>

      {/* Target User Selection Modal */}
      {targetUserModalAgent && (
        <div className="modal-overlay" onClick={() => setTargetUserModalAgent(null)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()} style={{ maxWidth: '460px' }}>
            <div className="modal-header">
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                <div
                  style={{
                    width: '32px',
                    height: '32px',
                    borderRadius: '8px',
                    background: 'rgba(56, 189, 248, 0.15)',
                    color: 'var(--accent-cyan)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <TerminalIcon size={16} />
                </div>
                <div>
                  <h3 style={{ fontSize: '1.05rem', fontWeight: 600 }}>Launch Terminal</h3>
                  <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>
                    {targetUserModalAgent.hostname} ({targetUserModalAgent.private_ip})
                  </p>
                </div>
              </div>
              <button className="tab-close" onClick={() => setTargetUserModalAgent(null)}>
                <X size={18} />
              </button>
            </div>

            <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-muted)', marginBottom: '8px' }}>
                  Connect as Target Linux User:
                </label>
                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                  {['default', 'root', 'ubuntu', 'ec2-user'].map((u) => (
                    <button
                      key={u}
                      type="button"
                      className={`btn btn-sm ${selectedUser === u ? 'btn-primary' : 'btn-secondary'}`}
                      onClick={() => setSelectedUser(u)}
                    >
                      {u === 'default' ? 'Default (Process User)' : u}
                    </button>
                  ))}
                  <button
                    type="button"
                    className={`btn btn-sm ${selectedUser === 'custom' ? 'btn-primary' : 'btn-secondary'}`}
                    onClick={() => setSelectedUser('custom')}
                  >
                    Custom User...
                  </button>
                </div>
              </div>

              {selectedUser === 'custom' && (
                <div>
                  <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-dim)', marginBottom: '4px' }}>
                    Linux Username on VM:
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. debian, centos, deploy"
                    value={customUser}
                    onChange={(e) => setCustomUser(e.target.value)}
                    style={{
                      width: '100%',
                      background: 'var(--bg-surface)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '6px',
                      padding: '8px 10px',
                      color: 'var(--text-main)',
                      fontSize: '0.85rem',
                      outline: 'none',
                    }}
                  />
                </div>
              )}
            </div>

            <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button className="btn btn-secondary btn-sm" onClick={() => setTargetUserModalAgent(null)}>
                Cancel
              </button>
              <button
                className="btn btn-primary btn-sm"
                onClick={() => {
                  const userToConnect = selectedUser === 'custom' ? customUser.trim() : selectedUser;
                  openTerminal(targetUserModalAgent, userToConnect);
                  setTargetUserModalAgent(null);
                }}
              >
                <TerminalIcon size={14} />
                <span>Open Terminal</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Node Tag Management Modal (Admin only) */}
      {tagModalAgent && (
        <div className="modal-overlay" onClick={() => setTagModalAgent(null)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()} style={{ maxWidth: '480px' }}>
            <div className="modal-header">
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                <div
                  style={{
                    width: '32px',
                    height: '32px',
                    borderRadius: '8px',
                    background: 'rgba(168, 85, 247, 0.15)',
                    color: '#c084fc',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <Tag size={16} />
                </div>
                <div>
                  <h3 style={{ fontSize: '1.05rem', fontWeight: 600 }}>Manage VM Tags</h3>
                  <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>
                    {tagModalAgent.hostname} ({tagModalAgent.private_ip})
                  </p>
                </div>
              </div>
              <button className="tab-close" onClick={() => setTagModalAgent(null)}>
                <X size={18} />
              </button>
            </div>

            <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '0.78rem', fontWeight: 600, color: 'var(--text-muted)', marginBottom: '6px' }}>
                  Assigned Tags:
                </label>
                {Object.keys(editingTags).length === 0 ? (
                  <div style={{ fontSize: '0.8rem', color: 'var(--text-dim)', fontStyle: 'italic' }}>
                    No tags assigned to this VM yet.
                  </div>
                ) : (
                  <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
                    {Object.entries(editingTags).map(([k, v]) => (
                      <span
                        key={k}
                        className="meta-chip"
                        style={{
                          display: 'inline-flex',
                          alignItems: 'center',
                          gap: '5px',
                          color: '#e9d5ff',
                          borderColor: 'rgba(168, 85, 247, 0.4)',
                          background: 'rgba(168, 85, 247, 0.15)',
                          padding: '3px 8px',
                          fontSize: '0.75rem',
                        }}
                      >
                        <span>
                          {k}:{v}
                        </span>
                        <X
                          size={12}
                          style={{ cursor: 'pointer' }}
                          onClick={() => {
                            const next = { ...editingTags };
                            delete next[k];
                            setEditingTags(next);
                          }}
                        />
                      </span>
                    ))}
                  </div>
                )}
              </div>

              <div
                style={{
                  display: 'flex',
                  gap: '8px',
                  alignItems: 'flex-end',
                  background: 'var(--bg-primary)',
                  padding: '10px',
                  borderRadius: '8px',
                  border: '1px solid var(--border-subtle)',
                }}
              >
                <div style={{ flex: 1 }}>
                  <label style={{ display: 'block', fontSize: '0.72rem', color: 'var(--text-dim)', marginBottom: '4px' }}>
                    Tag Key
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. env"
                    value={newTagKey}
                    onChange={(e) => setNewTagKey(e.target.value)}
                    style={{
                      width: '100%',
                      background: 'var(--bg-surface)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '6px',
                      padding: '6px 9px',
                      color: 'var(--text-main)',
                      fontSize: '0.8rem',
                      outline: 'none',
                    }}
                  />
                </div>
                <div style={{ flex: 1 }}>
                  <label style={{ display: 'block', fontSize: '0.72rem', color: 'var(--text-dim)', marginBottom: '4px' }}>
                    Tag Value
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. staging"
                    value={newTagValue}
                    onChange={(e) => setNewTagValue(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && newTagKey.trim() && newTagValue.trim()) {
                        e.preventDefault();
                        setEditingTags((prev) => ({ ...prev, [newTagKey.trim()]: newTagValue.trim() }));
                        setNewTagKey('');
                        setNewTagValue('');
                      }
                    }}
                    style={{
                      width: '100%',
                      background: 'var(--bg-surface)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '6px',
                      padding: '6px 9px',
                      color: 'var(--text-main)',
                      fontSize: '0.8rem',
                      outline: 'none',
                    }}
                  />
                </div>
                <button
                  type="button"
                  className="btn btn-secondary btn-sm"
                  style={{ height: '32px' }}
                  onClick={() => {
                    if (newTagKey.trim() && newTagValue.trim()) {
                      setEditingTags((prev) => ({ ...prev, [newTagKey.trim()]: newTagValue.trim() }));
                      setNewTagKey('');
                      setNewTagValue('');
                    }
                  }}
                >
                  <Plus size={13} />
                  <span>Add</span>
                </button>
              </div>
            </div>

            <div style={{ marginTop: '20px', display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button className="btn btn-secondary btn-sm" onClick={() => setTagModalAgent(null)}>
                Cancel
              </button>
              <button
                className="btn btn-primary btn-sm"
                disabled={isSavingTags}
                onClick={handleSaveNodeTags}
              >
                <span>Save Tags</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Users & Team Modal (Admin only) */}
      <UsersModal
        isOpen={isUsersModalOpen}
        onClose={() => setIsUsersModalOpen(false)}
        authToken={authToken}
        currentUsername={currentUser}
        agents={agents}
      />

      {/* Security Audit & Session Recordings Modal (Admin only) */}
      <AuditModal
        isOpen={isAuditModalOpen}
        onClose={() => setIsAuditModalOpen(false)}
        authToken={authToken}
      />

      {/* Enroll Modal */}
      <EnrollModal
        isOpen={isEnrollModalOpen}
        onClose={() => setIsEnrollModalOpen(false)}
        defaultToken="mesh-secret-token"
        authToken={authToken}
      />
    </div>
  );
};
