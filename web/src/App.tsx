import React, { useState, useEffect } from 'react';
import { AgentInfo, TerminalTab } from './types';
import { TerminalView } from './components/TerminalView';
import { EnrollModal } from './components/EnrollModal';
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
} from 'lucide-react';

export const App: React.FC = () => {
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [tabs, setTabs] = useState<TerminalTab[]>([]);
  const [activeTabId, setActiveTabId] = useState<string | null>(null);
  const [isEnrollModalOpen, setIsEnrollModalOpen] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const fetchAgents = async () => {
    setIsRefreshing(true);
    try {
      const res = await fetch('/api/v1/agents');
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
    fetchAgents();
    const interval = setInterval(fetchAgents, 3000);
    return () => clearInterval(interval);
  }, []);

  const openTerminal = (agent: AgentInfo) => {
    // Check if a tab already exists for this agent
    const existing = tabs.find((t) => t.agentId === agent.id);
    if (existing) {
      setActiveTabId(existing.id);
      return;
    }

    const newTab: TerminalTab = {
      id: `tab-${agent.id}-${Date.now()}`,
      agentId: agent.id,
      hostname: agent.hostname,
      privateIp: agent.private_ip,
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

  const filteredAgents = agents.filter((a) => {
    const q = searchQuery.toLowerCase();
    return (
      a.hostname.toLowerCase().includes(q) ||
      a.private_ip.toLowerCase().includes(q) ||
      a.id.toLowerCase().includes(q) ||
      a.os.toLowerCase().includes(q)
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

          <button className="btn btn-primary btn-sm" onClick={() => setIsEnrollModalOpen(true)}>
            <Plus size={15} />
            <span>Enroll Node</span>
          </button>
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
                    <button
                      className="btn btn-secondary btn-sm"
                      style={{ marginTop: '12px' }}
                      onClick={() => setIsEnrollModalOpen(true)}
                    >
                      Connect your first VM
                    </button>
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
                    onClick={() => openTerminal(agent)}
                  >
                    <div className="node-header">
                      <div className="node-hostname">
                        <span className={`status-dot ${agent.status}`} />
                        <span>{agent.hostname}</span>
                      </div>
                      <ChevronRight size={15} color="var(--text-dim)" />
                    </div>

                    <div className="node-ip-row">
                      <span>{agent.private_ip}</span>
                    </div>

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
                <TerminalView key={activeTab.id} tab={activeTab} onClose={closeTab} />
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
                <button
                  className="btn btn-primary"
                  onClick={() => {
                    if (agents.length > 0) {
                      openTerminal(agents[0]);
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
              </div>
            </div>
          )}
        </main>
      </div>

      {/* Enroll Modal */}
      <EnrollModal
        isOpen={isEnrollModalOpen}
        onClose={() => setIsEnrollModalOpen(false)}
        token="mesh-secret-token"
      />
    </div>
  );
};
