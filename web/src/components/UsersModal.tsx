import React, { useState, useEffect, useMemo } from 'react';
import { X, UserPlus, Trash2, Users, Clock, Tag, Server, Edit2, Check, Plus } from 'lucide-react';
import { AgentInfo, UserAccount } from '../types';

interface UsersModalProps {
  isOpen: boolean;
  onClose: () => void;
  authToken: string;
  currentUsername: string;
  agents?: AgentInfo[];
}

const parseRulesJson = (raw: string): string[] => {
  if (!raw || raw.trim() === '') return ['*'];
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed) && parsed.length > 0) {
      return parsed.map((s) => String(s).trim()).filter(Boolean);
    }
  } catch {
    return raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
  }
  return ['*'];
};

export const UsersModal: React.FC<UsersModalProps> = ({
  isOpen,
  onClose,
  authToken,
  currentUsername,
  agents = [],
}) => {
  const [users, setUsers] = useState<UserAccount[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // New user form state
  const [newUsername, setNewUsername] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState<'admin' | 'operator' | 'viewer'>('operator');
  const [newRules, setNewRules] = useState<string[]>(['*']);
  const [customRuleInput, setCustomRuleInput] = useState('');
  const [isCreating, setIsCreating] = useState(false);

  // Edit existing user state
  const [editingUserId, setEditingUserId] = useState<string | null>(null);
  const [editRole, setEditRole] = useState<'admin' | 'operator' | 'viewer'>('operator');
  const [editRules, setEditRules] = useState<string[]>(['*']);
  const [editCustomRuleInput, setEditCustomRuleInput] = useState('');
  const [isSavingEdit, setIsSavingEdit] = useState(false);

  // Discover all unique tags across connected agents
  const availableTags = useMemo(() => {
    const tagSet = new Set<string>();
    agents.forEach((a) => {
      if (a.tags) {
        Object.entries(a.tags).forEach(([k, v]) => {
          tagSet.add(`${k}:${v}`);
        });
      }
    });
    return Array.from(tagSet);
  }, [agents]);

  const fetchUsers = async () => {
    if (!authToken) return;
    setLoading(true);
    setError(null);
    try {
      const res = await fetch('/api/v1/users', {
        headers: { Authorization: `Bearer ${authToken}` },
      });
      if (res.ok) {
        const data = await res.json();
        setUsers(data.users || []);
      } else {
        const text = await res.text();
        setError(text || 'Failed to fetch users');
      }
    } catch (err: any) {
      setError(err.message || 'Error loading users');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      fetchUsers();
      setEditingUserId(null);
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const toggleRuleInList = (list: string[], rule: string): string[] => {
    const clean = list.filter((r) => r !== '*');
    if (clean.includes(rule)) {
      const next = clean.filter((r) => r !== rule);
      return next.length > 0 ? next : [];
    }
    return [...clean, rule];
  };

  const addCustomRuleToList = (list: string[], input: string): string[] => {
    const parts = input
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    if (parts.length === 0) return list;
    if (parts.includes('*')) return ['*'];
    const clean = list.filter((r) => r !== '*');
    const merged = Array.from(new Set([...clean, ...parts]));
    return merged;
  };

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newUsername.trim() || !newPassword) return;

    setIsCreating(true);
    setError(null);

    let finalRules = newRole === 'admin' ? ['*'] : newRules;
    if (customRuleInput.trim()) {
      finalRules = addCustomRuleToList(finalRules, customRuleInput);
    }
    if (finalRules.length === 0) {
      setError('Please select at least one VM/tag or choose "All VMs (*)".');
      setIsCreating(false);
      return;
    }

    try {
      const res = await fetch('/api/v1/users', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${authToken}`,
        },
        body: JSON.stringify({
          username: newUsername.trim(),
          password: newPassword,
          role: newRole,
          allowed_nodes: JSON.stringify(finalRules),
        }),
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to create user');
      }

      setNewUsername('');
      setNewPassword('');
      setNewRole('operator');
      setNewRules(['*']);
      setCustomRuleInput('');
      await fetchUsers();
    } catch (err: any) {
      setError(err.message || 'Failed to create user');
    } finally {
      setIsCreating(false);
    }
  };

  const startEditingUser = (u: UserAccount) => {
    setEditingUserId(u.id);
    setEditRole(u.role);
    setEditRules(parseRulesJson(u.allowed_nodes));
    setEditCustomRuleInput('');
    setError(null);
  };

  const handleSaveUserEdit = async (userId: string) => {
    setIsSavingEdit(true);
    setError(null);

    let finalRules = editRole === 'admin' ? ['*'] : editRules;
    if (editCustomRuleInput.trim()) {
      finalRules = addCustomRuleToList(finalRules, editCustomRuleInput);
    }
    if (finalRules.length === 0) {
      setError('Please select at least one VM/tag or choose "All VMs (*)".');
      setIsSavingEdit(false);
      return;
    }

    try {
      const res = await fetch('/api/v1/users', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${authToken}`,
        },
        body: JSON.stringify({
          id: userId,
          role: editRole,
          allowed_nodes: JSON.stringify(finalRules),
        }),
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to update user');
      }

      setEditingUserId(null);
      await fetchUsers();
    } catch (err: any) {
      setError(err.message || 'Failed to update user');
    } finally {
      setIsSavingEdit(false);
    }
  };

  const handleDeleteUser = async (id: string, username: string) => {
    if (username === currentUsername) {
      alert('You cannot delete your own account.');
      return;
    }
    if (!confirm(`Are you sure you want to delete user "${username}"?`)) {
      return;
    }

    try {
      const res = await fetch(`/api/v1/users?id=${encodeURIComponent(id)}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${authToken}` },
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to delete user');
      }
      await fetchUsers();
    } catch (err: any) {
      alert(err.message || 'Delete failed');
    }
  };

  const getRoleBadge = (role: string) => {
    switch (role) {
      case 'admin':
        return (
          <span
            className="badge-tag"
            style={{
              background: 'rgba(168, 85, 247, 0.15)',
              color: '#c084fc',
              borderColor: 'rgba(168, 85, 247, 0.35)',
            }}
          >
            Admin
          </span>
        );
      case 'operator':
        return (
          <span
            className="badge-tag"
            style={{
              background: 'rgba(56, 189, 248, 0.12)',
              color: 'var(--accent-cyan)',
              borderColor: 'rgba(56, 189, 248, 0.25)',
            }}
          >
            Operator
          </span>
        );
      case 'viewer':
      default:
        return (
          <span
            className="badge-tag"
            style={{
              background: 'rgba(148, 163, 184, 0.12)',
              color: '#94a3b8',
              borderColor: 'rgba(148, 163, 184, 0.25)',
            }}
          >
            Viewer (Read-Only)
          </span>
        );
    }
  };

  const renderAllowedNodesBadge = (allowedNodesJson: string, role: string) => {
    const rules = role === 'admin' ? ['*'] : parseRulesJson(allowedNodesJson);

    if (rules.includes('*')) {
      return (
        <span style={{ fontSize: '0.72rem', color: 'var(--text-dim)', display: 'flex', alignItems: 'center', gap: '4px' }}>
          <Server size={11} />
          <span>Access: All VMs (*)</span>
        </span>
      );
    }

    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: '5px', flexWrap: 'wrap', marginTop: '2px' }}>
        <span style={{ fontSize: '0.72rem', color: 'var(--text-dim)', display: 'flex', alignItems: 'center', gap: '3px' }}>
          <Tag size={11} color="var(--accent-cyan)" />
          <span>Allowed:</span>
        </span>
        {rules.map((r) => (
          <span
            key={r}
            className="meta-chip"
            style={{
              color: 'var(--accent-cyan)',
              borderColor: 'rgba(56, 189, 248, 0.3)',
              background: 'rgba(56, 189, 248, 0.08)',
              fontSize: '0.7rem',
            }}
          >
            {r}
          </span>
        ))}
      </div>
    );
  };

  const renderRuleSelector = (
    role: 'admin' | 'operator' | 'viewer',
    rules: string[],
    setRules: (next: string[]) => void,
    customInput: string,
    setCustomInput: (val: string) => void
  ) => {
    if (role === 'admin') {
      return (
        <div style={{ fontSize: '0.76rem', color: 'var(--text-dim)', padding: '4px 0' }}>
          Admin accounts automatically have full access to all VMs (<code>*</code>).
        </div>
      );
    }

    const isAllNodes = rules.includes('*');

    return (
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: '10px',
          background: 'rgba(15, 23, 42, 0.45)',
          padding: '10px 12px',
          borderRadius: '6px',
          border: '1px solid var(--border-subtle)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '8px' }}>
          <span style={{ fontSize: '0.76rem', fontWeight: 600, color: 'var(--text-muted)' }}>
            VM Access Scope:
          </span>
          <div style={{ display: 'flex', gap: '6px' }}>
            <button
              type="button"
              className={`btn btn-sm ${isAllNodes ? 'btn-primary' : 'btn-secondary'}`}
              style={{ padding: '4px 10px', fontSize: '0.74rem' }}
              onClick={() => setRules(['*'])}
            >
              All VMs (*)
            </button>
            <button
              type="button"
              className={`btn btn-sm ${!isAllNodes ? 'btn-primary' : 'btn-secondary'}`}
              style={{ padding: '4px 10px', fontSize: '0.74rem' }}
              onClick={() => {
                if (isAllNodes) {
                  setRules([]);
                }
              }}
            >
              Restrict to Specific VMs / Tags
            </button>
          </div>
        </div>

        {!isAllNodes && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
            {/* Connected VMs Picker */}
            <div>
              <div style={{ fontSize: '0.72rem', color: 'var(--text-dim)', marginBottom: '5px' }}>
                Select Allowed VMs ({agents.length} connected):
              </div>
              {agents.length === 0 ? (
                <div style={{ fontSize: '0.73rem', color: 'var(--text-dim)', fontStyle: 'italic' }}>
                  No VMs currently connected — add a rule by hostname or tag below.
                </div>
              ) : (
                <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
                  {agents.map((a) => {
                    const nodeRule = `node:${a.id}`;
                    const selected = rules.includes(nodeRule) || rules.includes(a.id) || rules.includes(a.hostname);
                    return (
                      <button
                        key={a.id}
                        type="button"
                        onClick={() => {
                          // Clean any bare id/hostname variants when toggling
                          const cleaned = rules.filter(
                            (r) => r !== '*' && r !== a.id && r !== a.hostname && r !== nodeRule
                          );
                          setRules(selected ? cleaned : [...cleaned, nodeRule]);
                        }}
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: '6px',
                          padding: '4px 9px',
                          borderRadius: '6px',
                          fontSize: '0.74rem',
                          cursor: 'pointer',
                          border: selected
                            ? '1px solid var(--accent-cyan)'
                            : '1px solid var(--border-subtle)',
                          background: selected
                            ? 'rgba(56, 189, 248, 0.16)'
                            : 'var(--bg-surface)',
                          color: selected ? '#f8fafc' : 'var(--text-muted)',
                        }}
                      >
                        <span className={`status-dot ${a.status}`} style={{ width: '6px', height: '6px' }} />
                        <span>{a.hostname}</span>
                        <span style={{ opacity: 0.6, fontSize: '0.68rem' }}>({a.private_ip})</span>
                        {selected && <Check size={12} color="var(--accent-cyan)" />}
                      </button>
                    );
                  })}
                </div>
              )}
            </div>

            {/* Discovered Tags Picker */}
            {availableTags.length > 0 && (
              <div>
                <div style={{ fontSize: '0.72rem', color: 'var(--text-dim)', marginBottom: '5px' }}>
                  Or Grant Access by VM Tag:
                </div>
                <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
                  {availableTags.map((tag) => {
                    const selected = rules.includes(tag);
                    return (
                      <button
                        key={tag}
                        type="button"
                        onClick={() => setRules(toggleRuleInList(rules, tag))}
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: '5px',
                          padding: '3px 8px',
                          borderRadius: '6px',
                          fontSize: '0.73rem',
                          cursor: 'pointer',
                          border: selected
                            ? '1px solid #c084fc'
                            : '1px solid var(--border-subtle)',
                          background: selected
                            ? 'rgba(168, 85, 247, 0.18)'
                            : 'var(--bg-surface)',
                          color: selected ? '#e9d5ff' : 'var(--text-muted)',
                        }}
                      >
                        <Tag size={11} />
                        <span>{tag}</span>
                        {selected && <Check size={11} color="#c084fc" />}
                      </button>
                    );
                  })}
                </div>
              </div>
            )}

            {/* Custom Rule Input */}
            <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
              <input
                type="text"
                placeholder="Add custom tag or VM (e.g. env:staging, node:vps-02)"
                value={customInput}
                onChange={(e) => setCustomInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    if (customInput.trim()) {
                      setRules(addCustomRuleToList(rules, customInput));
                      setCustomInput('');
                    }
                  }
                }}
                style={{
                  flex: 1,
                  background: 'var(--bg-surface)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '6px',
                  padding: '5px 9px',
                  color: 'var(--text-main)',
                  fontSize: '0.76rem',
                  outline: 'none',
                }}
              />
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                style={{ padding: '5px 10px', fontSize: '0.74rem' }}
                onClick={() => {
                  if (customInput.trim()) {
                    setRules(addCustomRuleToList(rules, customInput));
                    setCustomInput('');
                  }
                }}
              >
                <Plus size={12} />
                <span>Add Rule</span>
              </button>
            </div>

            {/* Active Rules Summary Pills */}
            {rules.length > 0 && (
              <div style={{ display: 'flex', gap: '5px', flexWrap: 'wrap', alignItems: 'center' }}>
                <span style={{ fontSize: '0.7rem', color: 'var(--text-dim)' }}>Selected rules:</span>
                {rules.map((r) => (
                  <span
                    key={r}
                    className="meta-chip"
                    style={{
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: '4px',
                      color: 'var(--accent-cyan)',
                      borderColor: 'rgba(56, 189, 248, 0.35)',
                      background: 'rgba(56, 189, 248, 0.1)',
                    }}
                  >
                    <span>{r}</span>
                    <X
                      size={11}
                      style={{ cursor: 'pointer' }}
                      onClick={() => setRules(rules.filter((item) => item !== r))}
                    />
                  </span>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: '760px' }}
      >
        <div className="modal-header">
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div
              style={{
                width: '34px',
                height: '34px',
                borderRadius: '8px',
                background: 'rgba(168, 85, 247, 0.15)',
                color: '#c084fc',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <Users size={18} />
            </div>
            <div>
              <h3 style={{ fontSize: '1.15rem', fontWeight: 600 }}>Team & Node Access Control</h3>
              <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>
                Manage team credentials, roles, and per-user VM or tag permissions
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

        <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Add User Form */}
          <form
            onSubmit={handleCreateUser}
            style={{
              background: 'var(--bg-primary)',
              padding: '14px',
              borderRadius: '8px',
              border: '1px solid var(--border-subtle)',
              display: 'flex',
              flexDirection: 'column',
              gap: '10px',
            }}
          >
            <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap', alignItems: 'flex-end' }}>
              <div style={{ flex: 1, minWidth: '130px' }}>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '4px' }}>
                  Username
                </label>
                <input
                  type="text"
                  placeholder="e.g. dev_sarah"
                  required
                  value={newUsername}
                  onChange={(e) => setNewUsername(e.target.value)}
                  style={{
                    width: '100%',
                    background: 'var(--bg-surface)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '6px',
                    padding: '7px 10px',
                    color: 'var(--text-main)',
                    fontSize: '0.82rem',
                    outline: 'none',
                  }}
                />
              </div>

              <div style={{ flex: 1, minWidth: '130px' }}>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '4px' }}>
                  Password
                </label>
                <input
                  type="password"
                  placeholder="Password"
                  required
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  style={{
                    width: '100%',
                    background: 'var(--bg-surface)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '6px',
                    padding: '7px 10px',
                    color: 'var(--text-main)',
                    fontSize: '0.82rem',
                    outline: 'none',
                  }}
                />
              </div>

              <div style={{ width: '150px' }}>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '4px' }}>
                  Role
                </label>
                <select
                  value={newRole}
                  onChange={(e) => setNewRole(e.target.value as any)}
                  style={{
                    width: '100%',
                    background: 'var(--bg-surface)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '6px',
                    padding: '7px 10px',
                    color: 'var(--text-main)',
                    fontSize: '0.82rem',
                    outline: 'none',
                  }}
                >
                  <option value="operator">Operator (SSH)</option>
                  <option value="admin">Admin (Full)</option>
                  <option value="viewer">Viewer (Read-Only)</option>
                </select>
              </div>

              <button
                type="submit"
                className="btn btn-primary btn-sm"
                disabled={isCreating}
                style={{ height: '34px', flexShrink: 0 }}
              >
                <UserPlus size={14} />
                <span>Add User</span>
              </button>
            </div>

            {renderRuleSelector(newRole, newRules, setNewRules, customRuleInput, setCustomRuleInput)}
          </form>

          {/* User List */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', maxHeight: '310px', overflowY: 'auto' }}>
            {loading && users.length === 0 && (
              <div style={{ textAlign: 'center', padding: '20px', color: 'var(--text-dim)', fontSize: '0.85rem' }}>
                Loading users...
              </div>
            )}
            {users.map((u) => {
              const isEditing = editingUserId === u.id;
              return (
                <div
                  key={u.id}
                  style={{
                    background: 'var(--bg-surface)',
                    border: isEditing ? '1px solid var(--accent-cyan)' : '1px solid var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '12px 14px',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '10px',
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                        <span style={{ fontWeight: 600, fontSize: '0.9rem', color: '#f8fafc' }}>
                          {u.username}
                        </span>
                        {u.username === currentUsername && (
                          <span style={{ fontSize: '0.7rem', color: 'var(--text-dim)' }}>(You)</span>
                        )}
                        {getRoleBadge(u.role)}
                      </div>

                      {!isEditing && (
                        <div style={{ marginTop: '4px' }}>
                          {renderAllowedNodesBadge(u.allowed_nodes, u.role)}
                        </div>
                      )}

                      <div style={{ display: 'flex', gap: '14px', marginTop: '4px', fontSize: '0.73rem', color: 'var(--text-dim)' }}>
                        <span>Joined: {new Date(u.created_at).toLocaleDateString()}</span>
                        {u.last_login_at && (
                          <span style={{ display: 'flex', alignItems: 'center', gap: '3px' }}>
                            <Clock size={11} />
                            Last login: {new Date(u.last_login_at).toLocaleDateString()}
                          </span>
                        )}
                      </div>
                    </div>

                    <div style={{ display: 'flex', gap: '6px' }}>
                      {!isEditing ? (
                        <button
                          className="btn btn-secondary btn-sm"
                          style={{ padding: '6px 10px', fontSize: '0.75rem' }}
                          onClick={() => startEditingUser(u)}
                          title="Edit Role & Allowed VMs"
                        >
                          <Edit2 size={13} />
                          <span>Edit Access</span>
                        </button>
                      ) : (
                        <>
                          <button
                            className="btn btn-secondary btn-sm"
                            style={{ padding: '6px 10px', fontSize: '0.75rem' }}
                            onClick={() => setEditingUserId(null)}
                          >
                            Cancel
                          </button>
                          <button
                            className="btn btn-primary btn-sm"
                            style={{ padding: '6px 10px', fontSize: '0.75rem' }}
                            disabled={isSavingEdit}
                            onClick={() => handleSaveUserEdit(u.id)}
                          >
                            <Check size={13} />
                            <span>Save</span>
                          </button>
                        </>
                      )}

                      {u.username !== currentUsername && (
                        <button
                          className="btn btn-secondary btn-sm"
                          style={{ color: '#ef4444', padding: '6px 8px' }}
                          onClick={() => handleDeleteUser(u.id, u.username)}
                          title="Delete User"
                        >
                          <Trash2 size={13} />
                        </button>
                      )}
                    </div>
                  </div>

                  {isEditing && (
                    <div
                      style={{
                        paddingTop: '8px',
                        borderTop: '1px solid var(--border-subtle)',
                        display: 'flex',
                        flexDirection: 'column',
                        gap: '10px',
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                        <label style={{ fontSize: '0.76rem', color: 'var(--text-muted)', fontWeight: 600 }}>
                          Role:
                        </label>
                        <select
                          value={editRole}
                          onChange={(e) => setEditRole(e.target.value as any)}
                          style={{
                            background: 'var(--bg-primary)',
                            border: '1px solid var(--border-subtle)',
                            borderRadius: '6px',
                            padding: '5px 10px',
                            color: 'var(--text-main)',
                            fontSize: '0.8rem',
                            outline: 'none',
                          }}
                        >
                          <option value="operator">Operator (SSH)</option>
                          <option value="admin">Admin (Full)</option>
                          <option value="viewer">Viewer (Read-Only)</option>
                        </select>
                      </div>

                      {renderRuleSelector(
                        editRole,
                        editRules,
                        setEditRules,
                        editCustomRuleInput,
                        setEditCustomRuleInput
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>

        <div style={{ marginTop: '16px', display: 'flex', justifyContent: 'flex-end' }}>
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
