import React, { useState, useEffect, useRef } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import {
  X,
  FileText,
  RefreshCw,
  Trash2,
  Download,
  Play,
  Pause,
  RotateCcw,
  Search,
  Terminal as TerminalIcon,
  Shield,
  Video,
  ArrowLeft,
} from 'lucide-react';
import { AuditLogEntry, AuditSettings } from '../types';

interface AuditModalProps {
  isOpen: boolean;
  onClose: () => void;
  authToken: string;
}

interface CastFrame {
  time: number;
  data: string;
}

export const AuditModal: React.FC<AuditModalProps> = ({ isOpen, onClose, authToken }) => {
  const [settings, setSettings] = useState<AuditSettings>({
    enable_audit_log: false,
    enable_command_logging: false,
    enable_session_recording: false,
  });
  const [savingSettings, setSavingSettings] = useState(false);

  const [logs, setLogs] = useState<AuditLogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [category, setCategory] = useState<'all' | 'ssh.session' | 'ssh.command' | 'auth_user' | 'agent'>('all');
  const [searchQuery, setSearchQuery] = useState('');

  // Session Replay Player state
  const [replayId, setReplayId] = useState<string | null>(null);
  const [replayTitle, setReplayTitle] = useState<string>('');
  const [frames, setFrames] = useState<CastFrame[]>([]);
  const [isPlaying, setIsPlaying] = useState(false);
  const [playbackSpeed, setPlaybackSpeed] = useState<1 | 2 | 4>(1);
  const [currentFrameIdx, setCurrentFrameIdx] = useState(0);

  const playerContainerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const timerRef = useRef<number | null>(null);

  const fetchSettingsAndLogs = async () => {
    if (!authToken) return;
    setLoading(true);
    setError(null);
    try {
      const [settingsRes, logsRes] = await Promise.all([
        fetch('/api/v1/audit/settings', {
          headers: { Authorization: `Bearer ${authToken}` },
        }),
        fetch('/api/v1/audit/logs?limit=300', {
          headers: { Authorization: `Bearer ${authToken}` },
        }),
      ]);

      if (settingsRes.ok) {
        const sData = await settingsRes.json();
        setSettings(sData);
      }
      if (logsRes.ok) {
        const lData = await logsRes.json();
        setLogs(lData.logs || []);
      }
    } catch (err: any) {
      setError(err.message || 'Failed to load audit data');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      fetchSettingsAndLogs();
      setReplayId(null);
    }
    return () => {
      if (timerRef.current) window.clearTimeout(timerRef.current);
      if (termRef.current) {
        termRef.current.dispose();
        termRef.current = null;
      }
    };
  }, [isOpen]);

  const handleToggleSetting = async (key: keyof AuditSettings) => {
    const nextSettings: AuditSettings = {
      ...settings,
      [key]: !settings[key],
    };
    setSettings(nextSettings);
    setSavingSettings(true);
    try {
      const res = await fetch('/api/v1/audit/settings', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${authToken}`,
        },
        body: JSON.stringify(nextSettings),
      });
      if (res.ok) {
        await fetchSettingsAndLogs();
      }
    } catch (err: any) {
      setError(err.message || 'Failed to update settings');
    } finally {
      setSavingSettings(false);
    }
  };

  const handleClearLogs = async () => {
    if (!confirm('Clear all audit logs and recorded terminal sessions?')) return;
    try {
      const res = await fetch('/api/v1/audit/logs', {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${authToken}` },
      });
      if (res.ok) {
        setLogs([]);
      }
    } catch (err: any) {
      alert(err.message || 'Failed to clear logs');
    }
  };

  const handleExportJSON = () => {
    const blob = new Blob([JSON.stringify(logs, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `ssh-mesh-audit-${new Date().toISOString().slice(0, 10)}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const openReplay = async (recordingId: string, label: string) => {
    setError(null);
    try {
      const res = await fetch(`/api/v1/audit/recordings?id=${encodeURIComponent(recordingId)}`, {
        headers: { Authorization: `Bearer ${authToken}` },
      });
      if (!res.ok) {
        throw new Error('Recording file not found on server');
      }
      const text = await res.text();
      const lines = text.split('\n').filter((l) => l.trim().length > 0);
      const parsedFrames: CastFrame[] = [];
      let headerTitle = label;

      lines.forEach((line, idx) => {
        try {
          const parsed = JSON.parse(line);
          if (idx === 0 && parsed.version === 2) {
            if (parsed.title) headerTitle = parsed.title;
          } else if (Array.isArray(parsed) && parsed.length >= 3 && parsed[1] === 'o') {
            parsedFrames.push({
              time: Number(parsed[0]) || 0,
              data: String(parsed[2]),
            });
          }
        } catch {
          // Ignore malformed frame line
        }
      });

      setReplayTitle(headerTitle);
      setFrames(parsedFrames);
      setCurrentFrameIdx(0);
      setReplayId(recordingId);
      setIsPlaying(true);
    } catch (err: any) {
      setError(err.message || 'Could not load session recording');
    }
  };

  // Initialize xterm when entering replay view
  useEffect(() => {
    if (!replayId || !playerContainerRef.current) return;

    playerContainerRef.current.innerHTML = '';
    const term = new Terminal({
      disableStdin: true,
      cursorBlink: false,
      fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
      fontSize: 13,
      lineHeight: 1.22,
      theme: {
        background: '#020617',
        foreground: '#f8fafc',
        cursor: '#38bdf8',
      },
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(playerContainerRef.current);
    setTimeout(() => {
      try {
        fitAddon.fit();
      } catch {
        // Ignore fit error if hidden
      }
    }, 50);
    termRef.current = term;

    return () => {
      if (timerRef.current) window.clearTimeout(timerRef.current);
      term.dispose();
      termRef.current = null;
    };
  }, [replayId]);

  // Step through frames during playback
  useEffect(() => {
    if (!replayId || !isPlaying || !termRef.current) return;
    if (currentFrameIdx >= frames.length) {
      setIsPlaying(false);
      return;
    }

    const current = frames[currentFrameIdx];
    const prevTime = currentFrameIdx > 0 ? frames[currentFrameIdx - 1].time : 0;
    const rawDelayMs = Math.max(0, (current.time - prevTime) * 1000);
    // Cap idle pauses to 1.5s so replay feels snappy
    const delayMs = Math.min(rawDelayMs, 1500) / playbackSpeed;

    timerRef.current = window.setTimeout(() => {
      termRef.current?.write(current.data);
      setCurrentFrameIdx((idx) => idx + 1);
    }, delayMs);

    return () => {
      if (timerRef.current) window.clearTimeout(timerRef.current);
    };
  }, [replayId, isPlaying, currentFrameIdx, frames, playbackSpeed]);

  const handleRestartReplay = () => {
    if (timerRef.current) window.clearTimeout(timerRef.current);
    termRef.current?.clear();
    termRef.current?.reset();
    setCurrentFrameIdx(0);
    setIsPlaying(true);
  };

  if (!isOpen) return null;

  const filteredLogs = logs.filter((entry) => {
    if (category === 'ssh.session' && !entry.event_type.startsWith('ssh.session') && entry.event_type !== 'ssh.access_denied') {
      return false;
    }
    if (category === 'ssh.command' && entry.event_type !== 'ssh.command') {
      return false;
    }
    if (
      category === 'auth_user' &&
      !entry.event_type.startsWith('auth.') &&
      !entry.event_type.startsWith('user.') &&
      !entry.event_type.startsWith('token.') &&
      !entry.event_type.startsWith('node.') &&
      !entry.event_type.startsWith('audit.')
    ) {
      return false;
    }
    if (category === 'agent' && !entry.event_type.startsWith('agent.')) {
      return false;
    }

    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return (
      entry.actor.toLowerCase().includes(q) ||
      entry.event_type.toLowerCase().includes(q) ||
      entry.target.toLowerCase().includes(q) ||
      entry.details.toLowerCase().includes(q) ||
      entry.client_ip.toLowerCase().includes(q)
    );
  });

  const renderEventBadge = (eventType: string, status: string) => {
    if (status === 'denied') {
      return (
        <span
          className="badge-tag"
          style={{
            background: 'rgba(239, 68, 68, 0.15)',
            color: '#f87171',
            borderColor: 'rgba(239, 68, 68, 0.35)',
          }}
        >
          {eventType}
        </span>
      );
    }
    if (eventType === 'ssh.command') {
      return (
        <span
          className="badge-tag"
          style={{
            background: 'rgba(245, 158, 11, 0.14)',
            color: '#fbbf24',
            borderColor: 'rgba(245, 158, 11, 0.35)',
          }}
        >
          command
        </span>
      );
    }
    if (eventType.startsWith('ssh.session')) {
      return (
        <span
          className="badge-tag"
          style={{
            background: 'rgba(56, 189, 248, 0.14)',
            color: 'var(--accent-cyan)',
            borderColor: 'rgba(56, 189, 248, 0.3)',
          }}
        >
          {eventType}
        </span>
      );
    }
    if (eventType.startsWith('agent.')) {
      return (
        <span
          className="badge-tag"
          style={{
            background: 'rgba(16, 185, 129, 0.14)',
            color: '#34d399',
            borderColor: 'rgba(16, 185, 129, 0.3)',
          }}
        >
          {eventType}
        </span>
      );
    }
    return (
      <span
        className="badge-tag"
        style={{
          background: 'rgba(168, 85, 247, 0.14)',
          color: '#c084fc',
          borderColor: 'rgba(168, 85, 247, 0.3)',
        }}
      >
        {eventType}
      </span>
    );
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: '900px', width: '94vw' }}
      >
        {/* Header */}
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
              <FileText size={18} />
            </div>
            <div>
              <h3 style={{ fontSize: '1.15rem', fontWeight: 600 }}>Security Auditing, Command Logs & Session Replay</h3>
              <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>
                Configure opt-in auditing layers and inspect user activity, executed commands, and recorded SSH sessions
              </p>
            </div>
          </div>
          <button className="tab-close" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        {error && (
          <div
            style={{
              padding: '8px 12px',
              borderRadius: '6px',
              background: 'rgba(239, 68, 68, 0.1)',
              border: '1px solid rgba(239, 68, 68, 0.25)',
              color: '#f87171',
              fontSize: '0.8rem',
              marginTop: '12px',
            }}
          >
            {error}
          </div>
        )}

        {replayId ? (
          /* Built-in xterm.js Session Replay View */
          <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                background: 'var(--bg-primary)',
                padding: '10px 14px',
                borderRadius: '8px',
                border: '1px solid var(--border-subtle)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                <button
                  className="btn btn-secondary btn-sm"
                  onClick={() => {
                    if (timerRef.current) window.clearTimeout(timerRef.current);
                    setIsPlaying(false);
                    setReplayId(null);
                  }}
                >
                  <ArrowLeft size={14} />
                  <span>Back to Logs</span>
                </button>
                <span style={{ fontWeight: 600, fontSize: '0.88rem', color: '#f8fafc' }}>
                  Session Replay: {replayTitle}
                </span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <button
                  className="btn btn-primary btn-sm"
                  onClick={() => {
                    if (currentFrameIdx >= frames.length) {
                      handleRestartReplay();
                    } else {
                      setIsPlaying(!isPlaying);
                    }
                  }}
                >
                  {isPlaying ? <Pause size={13} /> : <Play size={13} />}
                  <span>{isPlaying ? 'Pause' : currentFrameIdx >= frames.length ? 'Replay Again' : 'Play'}</span>
                </button>
                <button className="btn btn-secondary btn-sm" onClick={handleRestartReplay} title="Restart">
                  <RotateCcw size={13} />
                </button>
                {([1, 2, 4] as const).map((spd) => (
                  <button
                    key={spd}
                    className={`btn btn-sm ${playbackSpeed === spd ? 'btn-primary' : 'btn-secondary'}`}
                    style={{ padding: '4px 8px', fontSize: '0.73rem' }}
                    onClick={() => setPlaybackSpeed(spd)}
                  >
                    {spd}x
                  </button>
                ))}
                <span style={{ fontSize: '0.75rem', color: 'var(--text-dim)', marginLeft: '4px' }}>
                  {currentFrameIdx} / {frames.length} frames
                </span>
              </div>
            </div>

            <div
              ref={playerContainerRef}
              style={{
                height: '380px',
                width: '100%',
                background: '#020617',
                borderRadius: '8px',
                border: '1px solid var(--border-subtle)',
                padding: '8px',
                overflow: 'hidden',
              }}
            />
          </div>
        ) : (
          <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '14px' }}>
            {/* Layer Toggles (All Off by Default, Admin-Controlled) */}
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))',
                gap: '10px',
              }}
            >
              {[
                {
                  key: 'enable_audit_log' as const,
                  title: '1. Control-Plane & Session Audit',
                  desc: 'Log logins, RBAC changes, agent connections & SSH sessions',
                  icon: <Shield size={16} color="#c084fc" />,
                  enabled: settings.enable_audit_log,
                },
                {
                  key: 'enable_command_logging' as const,
                  title: '2. SSH Command History',
                  desc: 'Capture shell commands executed inside user SSH terminals',
                  icon: <TerminalIcon size={16} color="#fbbf24" />,
                  enabled: settings.enable_command_logging,
                },
                {
                  key: 'enable_session_recording' as const,
                  title: '3. Full Terminal Recording',
                  desc: 'Record terminal output for in-browser asciicast session replay',
                  icon: <Video size={16} color="var(--accent-cyan)" />,
                  enabled: settings.enable_session_recording,
                },
              ].map((item) => (
                <div
                  key={item.key}
                  style={{
                    background: 'var(--bg-primary)',
                    border: item.enabled ? '1px solid rgba(56, 189, 248, 0.4)' : '1px solid var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '12px',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    gap: '10px',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'flex-start', gap: '9px' }}>
                    <div style={{ marginTop: '2px' }}>{item.icon}</div>
                    <div>
                      <div style={{ fontSize: '0.82rem', fontWeight: 600, color: '#f8fafc' }}>{item.title}</div>
                      <div style={{ fontSize: '0.72rem', color: 'var(--text-dim)', marginTop: '2px', lineHeight: 1.35 }}>
                        {item.desc}
                      </div>
                    </div>
                  </div>

                  <button
                    type="button"
                    disabled={savingSettings}
                    onClick={() => handleToggleSetting(item.key)}
                    className={`btn btn-sm ${item.enabled ? 'btn-primary' : 'btn-secondary'}`}
                    style={{
                      minWidth: '68px',
                      justifyContent: 'center',
                      fontSize: '0.74rem',
                      padding: '5px 10px',
                      flexShrink: 0,
                    }}
                  >
                    {item.enabled ? 'Enabled' : 'Disabled'}
                  </button>
                </div>
              ))}
            </div>

            {/* Filter & Action Toolbar */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '8px' }}>
              <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
                {[
                  { id: 'all', label: 'All Events' },
                  { id: 'ssh.session', label: 'SSH Sessions' },
                  { id: 'ssh.command', label: 'Shell Commands' },
                  { id: 'auth_user', label: 'Auth & Admin' },
                  { id: 'agent', label: 'Agents' },
                ].map((tab) => (
                  <button
                    key={tab.id}
                    type="button"
                    className={`btn btn-sm ${category === tab.id ? 'btn-primary' : 'btn-secondary'}`}
                    style={{ fontSize: '0.75rem', padding: '5px 10px' }}
                    onClick={() => setCategory(tab.id as any)}
                  >
                    {tab.label}
                  </button>
                ))}
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <div style={{ position: 'relative' }}>
                  <Search
                    size={13}
                    style={{ position: 'absolute', left: '9px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-dim)' }}
                  />
                  <input
                    type="text"
                    placeholder="Search user, VM, command..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    style={{
                      background: 'var(--bg-surface)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '6px',
                      padding: '5px 10px 5px 28px',
                      color: 'var(--text-main)',
                      fontSize: '0.76rem',
                      width: '190px',
                      outline: 'none',
                    }}
                  />
                </div>

                <button className="btn btn-secondary btn-sm" onClick={fetchSettingsAndLogs} title="Refresh Logs">
                  <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
                </button>
                <button className="btn btn-secondary btn-sm" onClick={handleExportJSON} title="Export JSON">
                  <Download size={13} />
                  <span style={{ fontSize: '0.74rem' }}>Export</span>
                </button>
                {logs.length > 0 && (
                  <button
                    className="btn btn-secondary btn-sm"
                    style={{ color: '#ef4444' }}
                    onClick={handleClearLogs}
                    title="Clear All Logs"
                  >
                    <Trash2 size={13} />
                  </button>
                )}
              </div>
            </div>

            {/* Audit Log List */}
            <div
              style={{
                display: 'flex',
                flexDirection: 'column',
                gap: '6px',
                maxHeight: '340px',
                overflowY: 'auto',
                paddingRight: '2px',
              }}
            >
              {filteredLogs.length === 0 ? (
                <div
                  style={{
                    textAlign: 'center',
                    padding: '32px 16px',
                    background: 'var(--bg-surface)',
                    borderRadius: '8px',
                    border: '1px solid var(--border-subtle)',
                    color: 'var(--text-dim)',
                    fontSize: '0.82rem',
                  }}
                >
                  {!settings.enable_audit_log && !settings.enable_command_logging && !settings.enable_session_recording
                    ? 'Auditing is currently disabled. Enable any of the 3 layers above to start recording events.'
                    : 'No matching audit log entries recorded yet.'}
                </div>
              ) : (
                filteredLogs.map((entry) => (
                  <div
                    key={entry.id}
                    style={{
                      background: 'var(--bg-surface)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '7px',
                      padding: '9px 12px',
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      gap: '12px',
                    }}
                  >
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', minWidth: 0, flex: 1 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                        {renderEventBadge(entry.event_type, entry.status)}
                        <span style={{ fontSize: '0.8rem', fontWeight: 600, color: '#f8fafc' }}>
                          {entry.actor}
                        </span>
                        {entry.target && (
                          <span style={{ fontSize: '0.76rem', color: 'var(--accent-cyan)' }}>
                            &rarr; {entry.target}
                          </span>
                        )}
                        <span style={{ fontSize: '0.7rem', color: 'var(--text-dim)' }}>
                          {new Date(entry.timestamp).toLocaleString()}
                        </span>
                        {entry.client_ip && (
                          <span style={{ fontSize: '0.68rem', color: 'var(--text-dim)' }}>
                            ({entry.client_ip})
                          </span>
                        )}
                      </div>

                      {entry.details && (
                        <div
                          style={{
                            fontSize: '0.77rem',
                            color: entry.event_type === 'ssh.command' ? '#fbbf24' : 'var(--text-muted)',
                            fontFamily:
                              entry.event_type === 'ssh.command'
                                ? "'JetBrains Mono', 'Fira Code', monospace"
                                : 'inherit',
                            background:
                              entry.event_type === 'ssh.command' ? 'rgba(2, 6, 23, 0.65)' : 'transparent',
                            padding: entry.event_type === 'ssh.command' ? '4px 8px' : '0',
                            borderRadius: '4px',
                            overflowX: 'auto',
                          }}
                        >
                          {entry.details}
                        </div>
                      )}
                    </div>

                    {entry.recording_id && (
                      <button
                        type="button"
                        className="btn btn-secondary btn-sm"
                        style={{
                          flexShrink: 0,
                          color: 'var(--accent-cyan)',
                          borderColor: 'rgba(56, 189, 248, 0.35)',
                          padding: '5px 10px',
                          fontSize: '0.74rem',
                        }}
                        onClick={() => openReplay(entry.recording_id!, `${entry.actor}@${entry.target}`)}
                      >
                        <Play size={12} />
                        <span>Replay</span>
                      </button>
                    )}
                  </div>
                ))
              )}
            </div>
          </div>
        )}

        <div style={{ marginTop: '16px', display: 'flex', justifyContent: 'flex-end' }}>
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
};

