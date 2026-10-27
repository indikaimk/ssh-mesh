import React, { useState, useEffect } from 'react';
import { X, Copy, Check, ShieldCheck, Server, Key, Plus, Trash2, Clock, Download } from 'lucide-react';
import { EnrollmentToken } from '../types';

interface EnrollModalProps {
  isOpen: boolean;
  onClose: () => void;
  defaultToken: string;
  authToken: string;
}

export const EnrollModal: React.FC<EnrollModalProps> = ({
  isOpen,
  onClose,
  defaultToken,
  authToken,
}) => {
  const [activeTab, setActiveTab] = useState<'install' | 'tokens'>('install');
  const [tokens, setTokens] = useState<EnrollmentToken[]>([]);
  const [selectedToken, setSelectedToken] = useState(defaultToken);
  const [copiedCurl, setCopiedCurl] = useState(false);
  const [copiedBinary, setCopiedBinary] = useState(false);

  // New token form state
  const [newTokenName, setNewTokenName] = useState('');
  const [newTokenHours, setNewTokenHours] = useState('168'); // 7 days default
  const [isCreatingToken, setIsCreatingToken] = useState(false);

  const fetchTokens = async () => {
    if (!authToken) return;
    try {
      const res = await fetch('/api/v1/tokens', {
        headers: { Authorization: `Bearer ${authToken}` },
      });
      if (res.ok) {
        const data = await res.json();
        setTokens(data.tokens || []);
        if (data.tokens && data.tokens.length > 0 && !selectedToken) {
          const active = data.tokens.find((t: EnrollmentToken) => !t.revoked);
          if (active) setSelectedToken(active.token);
        }
      }
    } catch (err) {
      console.error('Failed to fetch tokens:', err);
    }
  };

  useEffect(() => {
    if (isOpen) {
      fetchTokens();
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const host = window.location.host;
  const isHttps = window.location.protocol === 'https:';
  const httpScheme = isHttps ? 'https' : 'http';
  const wsScheme = isHttps ? 'wss' : 'ws';

  const tokenParam = selectedToken ? `?token=${encodeURIComponent(selectedToken)}` : '';
  const curlCmd = `curl -sSL ${httpScheme}://${host}/api/v1/install-script${tokenParam} | sudo bash`;
  const binaryCmd = `chmod +x ssh-mesh-agent-* && ./ssh-mesh-agent-* -server ${wsScheme}://${host}/api/v1/agent/tunnel -token ${selectedToken || defaultToken}`;

  const handleCopy = (text: string, type: 'curl' | 'binary') => {
    navigator.clipboard.writeText(text);
    if (type === 'curl') {
      setCopiedCurl(true);
      setTimeout(() => setCopiedCurl(false), 2000);
    } else {
      setCopiedBinary(true);
      setTimeout(() => setCopiedBinary(false), 2000);
    }
  };

  const handleCreateToken = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newTokenName.trim()) return;
    setIsCreatingToken(true);
    try {
      const res = await fetch('/api/v1/tokens', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${authToken}`,
        },
        body: JSON.stringify({
          name: newTokenName.trim(),
          duration_hours: parseInt(newTokenHours, 10),
        }),
      });
      if (res.ok) {
        const created = await res.json();
        setSelectedToken(created.token);
        setNewTokenName('');
        await fetchTokens();
      }
    } catch (err) {
      console.error('Error creating token:', err);
    } finally {
      setIsCreatingToken(false);
    }
  };

  const handleRevokeToken = async (id: string) => {
    try {
      const res = await fetch(`/api/v1/tokens?id=${encodeURIComponent(id)}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${authToken}` },
      });
      if (res.ok) {
        await fetchTokens();
      }
    } catch (err) {
      console.error('Error revoking token:', err);
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: '680px' }}
      >
        <div className="modal-header">
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div
              style={{
                width: '34px',
                height: '34px',
                borderRadius: '8px',
                background: 'rgba(56, 189, 248, 0.15)',
                color: 'var(--accent-cyan)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <Server size={18} />
            </div>
            <div>
              <h3 style={{ fontSize: '1.15rem', fontWeight: 600 }}>Enrollment & Security</h3>
              <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>
                Connect private cloud VMs and manage agent authorization tokens
              </p>
            </div>
          </div>
          <button className="tab-close" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        {/* Modal Nav Tabs */}
        <div
          style={{
            display: 'flex',
            gap: '8px',
            borderBottom: '1px solid var(--border-subtle)',
            marginTop: '12px',
            marginBottom: '16px',
          }}
        >
          <button
            className={`btn btn-sm ${activeTab === 'install' ? 'btn-primary' : 'btn-secondary'}`}
            style={{ borderRadius: '6px 6px 0 0' }}
            onClick={() => setActiveTab('install')}
          >
            <Server size={14} />
            <span>Quick Install</span>
          </button>
          <button
            className={`btn btn-sm ${activeTab === 'tokens' ? 'btn-primary' : 'btn-secondary'}`}
            style={{ borderRadius: '6px 6px 0 0' }}
            onClick={() => setActiveTab('tokens')}
          >
            <Key size={14} />
            <span>Manage Tokens ({tokens.length})</span>
          </button>
        </div>

        {activeTab === 'install' ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '18px' }}>
            {tokens.length > 0 && (
              <div>
                <label style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '6px' }}>
                  Use Token for Installation:
                </label>
                <select
                  value={selectedToken}
                  onChange={(e) => setSelectedToken(e.target.value)}
                  style={{
                    background: 'var(--bg-surface)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '8px 12px',
                    color: 'var(--text-main)',
                    fontSize: '0.85rem',
                    width: '100%',
                    outline: 'none',
                  }}
                >
                  {tokens.map((t) => (
                    <option key={t.id} value={t.token} disabled={t.revoked}>
                      {t.name} {t.revoked ? '(REVOKED)' : ''} - {t.token.slice(0, 14)}...
                    </option>
                  ))}
                </select>
              </div>
            )}

            <div>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'var(--text-main)' }}>
                  Option 1: One-Line Auto-Installer (Recommended)
                </div>
                <span className="badge-tag" style={{ color: '#10b981', borderColor: 'rgba(16, 185, 129, 0.3)' }}>
                  Auto-Detects OS & Arch
                </span>
              </div>
              <p style={{ fontSize: '0.8rem', color: 'var(--text-dim)', marginBottom: '8px' }}>
                Detects architecture, downloads static agent to <code style={{ color: 'var(--accent-cyan)' }}>/usr/local/bin</code>, and configures a systemd background service:
              </p>
              <div className="code-block">
                <code>{curlCmd}</code>
                <button
                  className="btn btn-secondary btn-sm"
                  onClick={() => handleCopy(curlCmd, 'curl')}
                  style={{ marginLeft: '12px', flexShrink: 0 }}
                >
                  {copiedCurl ? <Check size={14} color="#10b981" /> : <Copy size={14} />}
                  <span>{copiedCurl ? 'Copied' : 'Copy'}</span>
                </button>
              </div>
            </div>

            <div>
              <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'var(--text-main)', marginBottom: '6px' }}>
                Option 2: Pre-Compiled Static Binaries (Self-Contained)
              </div>
              <p style={{ fontSize: '0.8rem', color: 'var(--text-dim)', marginBottom: '10px' }}>
                Download pure static binaries compiled with <code style={{ color: 'var(--accent-cyan)' }}>CGO_ENABLED=0</code> without external libc dependencies:
              </p>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: '10px', marginBottom: '12px' }}>
                {[
                  {
                    name: 'Linux x86_64',
                    os: 'linux',
                    arch: 'amd64',
                    badge: 'amd64',
                    desc: 'Ubuntu, Debian, RHEL, Alpine, Cloud VPS',
                    size: '6.8 MB',
                  },
                  {
                    name: 'Linux ARM64',
                    os: 'linux',
                    arch: 'arm64',
                    badge: 'arm64',
                    desc: 'AWS Graviton, Raspberry Pi 4+, Oracle ARM',
                    size: '6.2 MB',
                  },
                  {
                    name: 'macOS Apple Silicon',
                    os: 'darwin',
                    arch: 'arm64',
                    badge: 'arm64',
                    desc: 'Apple Silicon M1 / M2 / M3 / M4 Macs',
                    size: '6.4 MB',
                  },
                  {
                    name: 'macOS Intel',
                    os: 'darwin',
                    arch: 'amd64',
                    badge: 'amd64',
                    desc: 'Intel-based Mac hardware',
                    size: '6.9 MB',
                  },
                ].map((b) => (
                  <div
                    key={`${b.os}-${b.arch}`}
                    style={{
                      background: 'var(--bg-primary)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '8px',
                      padding: '10px 12px',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      gap: '8px',
                    }}
                  >
                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <span style={{ fontSize: '0.82rem', fontWeight: 600, color: 'var(--text-main)' }}>{b.name}</span>
                        <span className="badge-tag" style={{ fontSize: '0.65rem', padding: '1px 5px' }}>{b.badge}</span>
                      </div>
                      <div style={{ fontSize: '0.72rem', color: 'var(--text-dim)', marginTop: '2px' }}>
                        {b.desc} &bull; {b.size}
                      </div>
                    </div>
                    <a
                      href={`/api/v1/download/agent?os=${b.os}&arch=${b.arch}`}
                      download={`ssh-mesh-agent-${b.os}-${b.arch}`}
                      className="btn btn-secondary btn-sm"
                      style={{ padding: '6px 10px', flexShrink: 0 }}
                      title={`Download ${b.name} binary`}
                    >
                      <Download size={13} />
                      <span style={{ fontSize: '0.75rem' }}>Download</span>
                    </a>
                  </div>
                ))}
              </div>

              <div style={{ fontSize: '0.78rem', color: 'var(--text-dim)', marginBottom: '6px' }}>
                Run manually after downloading:
              </div>
              <div className="code-block">
                <code>{binaryCmd}</code>
                <button
                  className="btn btn-secondary btn-sm"
                  onClick={() => handleCopy(binaryCmd, 'binary')}
                  style={{ marginLeft: '12px', flexShrink: 0 }}
                >
                  {copiedBinary ? <Check size={14} color="#10b981" /> : <Copy size={14} />}
                  <span>{copiedBinary ? 'Copied' : 'Copy'}</span>
                </button>
              </div>
            </div>

            <div
              style={{
                padding: '12px 14px',
                borderRadius: '8px',
                background: 'rgba(16, 185, 129, 0.08)',
                border: '1px solid rgba(16, 185, 129, 0.2)',
                display: 'flex',
                gap: '10px',
                fontSize: '0.8rem',
                color: '#34d399',
              }}
            >
              <ShieldCheck size={18} style={{ flexShrink: 0, marginTop: '2px' }} />
              <div>
                <strong>Zero Inbound Firewall Exposure</strong>: The agent initiates an outbound reverse tunnel. No open port 22 or public IP required on the host.
              </div>
            </div>
          </div>
        ) : (
          /* Tokens Management Tab */
          <div style={{ display: 'flex', flexDirection: 'column', gap: '18px' }}>
            {/* Create Token Box */}
            <form
              onSubmit={handleCreateToken}
              style={{
                background: 'var(--bg-primary)',
                padding: '14px',
                borderRadius: '8px',
                border: '1px solid var(--border-subtle)',
                display: 'flex',
                gap: '10px',
                alignItems: 'flex-end',
                flexWrap: 'wrap',
              }}
            >
              <div style={{ flex: 1, minWidth: '180px' }}>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '4px' }}>
                  Token Label / Name
                </label>
                <input
                  type="text"
                  placeholder="e.g. AWS Staging VPC"
                  value={newTokenName}
                  onChange={(e) => setNewTokenName(e.target.value)}
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

              <div style={{ width: '130px' }}>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '4px' }}>
                  Expiration
                </label>
                <select
                  value={newTokenHours}
                  onChange={(e) => setNewTokenHours(e.target.value)}
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
                >
                  <option value="24">24 Hours</option>
                  <option value="168">7 Days</option>
                  <option value="720">30 Days</option>
                  <option value="0">Never</option>
                </select>
              </div>

              <button
                type="submit"
                className="btn btn-primary btn-sm"
                disabled={isCreatingToken || !newTokenName.trim()}
                style={{ height: '36px' }}
              >
                <Plus size={15} />
                <span>Generate Token</span>
              </button>
            </form>

            {/* Token List */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', maxHeight: '280px', overflowY: 'auto' }}>
              {tokens.length === 0 ? (
                <div style={{ textAlign: 'center', padding: '20px', color: 'var(--text-dim)', fontSize: '0.85rem' }}>
                  No tokens created yet.
                </div>
              ) : (
                tokens.map((t) => (
                  <div
                    key={t.id}
                    style={{
                      background: 'var(--bg-surface)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '8px',
                      padding: '12px 14px',
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      opacity: t.revoked ? 0.5 : 1,
                    }}
                  >
                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                        <span style={{ fontWeight: 600, fontSize: '0.9rem' }}>{t.name}</span>
                        {t.revoked ? (
                          <span className="badge-tag" style={{ color: '#ef4444', borderColor: '#ef4444' }}>
                            Revoked
                          </span>
                        ) : (
                          <span className="badge-tag" style={{ color: '#10b981', borderColor: '#10b981' }}>
                            Active
                          </span>
                        )}
                      </div>
                      <div style={{ fontFamily: 'var(--font-mono)', fontSize: '0.8rem', color: 'var(--accent-cyan)', marginTop: '4px' }}>
                        {t.token}
                      </div>
                      {t.expires_at && !t.revoked && (
                        <div style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '0.72rem', color: 'var(--text-dim)', marginTop: '4px' }}>
                          <Clock size={12} />
                          <span>Expires: {new Date(t.expires_at).toLocaleDateString()}</span>
                        </div>
                      )}
                    </div>

                    <div style={{ display: 'flex', gap: '8px' }}>
                      <button
                        className="btn btn-secondary btn-sm"
                        onClick={() => {
                          navigator.clipboard.writeText(t.token);
                        }}
                        title="Copy Token"
                      >
                        <Copy size={13} />
                      </button>
                      {!t.revoked && t.id !== 'default' && (
                        <button
                          className="btn btn-secondary btn-sm"
                          style={{ color: '#ef4444' }}
                          onClick={() => handleRevokeToken(t.id)}
                          title="Revoke Token"
                        >
                          <Trash2 size={13} />
                        </button>
                      )}
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        )}

        <div style={{ marginTop: '20px', display: 'flex', justifyContent: 'flex-end' }}>
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
