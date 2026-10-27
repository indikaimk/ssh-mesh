import React, { useState, useEffect } from 'react';
import { X, UserPlus, Trash2, Users, Clock, Tag } from 'lucide-react';
import { UserAccount } from '../types';

interface UsersModalProps {
  isOpen: boolean;
  onClose: () => void;
  authToken: string;
  currentUsername: string;
}

export const UsersModal: React.FC<UsersModalProps> = ({
  isOpen,
  onClose,
  authToken,
  currentUsername,
}) => {
  const [users, setUsers] = useState<UserAccount[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // New user form state
  const [newUsername, setNewUsername] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState<'admin' | 'operator' | 'viewer'>('operator');
  const [newAllowedNodes, setNewAllowedNodes] = useState('*');
  const [isCreating, setIsCreating] = useState(false);

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
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newUsername.trim() || !newPassword) return;

    setIsCreating(true);
    setError(null);

    // Parse allowed nodes input
    const rules =
      newAllowedNodes.trim() === '*' || newAllowedNodes.trim() === ''
        ? ['*']
        : newAllowedNodes
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean);

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
          allowed_nodes: JSON.stringify(rules),
        }),
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to create user');
      }

      setNewUsername('');
      setNewPassword('');
      setNewRole('operator');
      setNewAllowedNodes('*');
      await fetchUsers();
    } catch (err: any) {
      setError(err.message || 'Failed to create user');
    } finally {
      setIsCreating(false);
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

  const renderAllowedNodesBadge = (allowedNodesJson: string) => {
    let rules: string[] = ['*'];
    try {
      if (allowedNodesJson) {
        rules = JSON.parse(allowedNodesJson);
      }
    } catch (e) {
      rules = [allowedNodesJson];
    }

    if (rules.includes('*')) {
      return (
        <span style={{ fontSize: '0.72rem', color: 'var(--text-dim)', display: 'flex', alignItems: 'center', gap: '3px' }}>
          <Tag size={11} />
          <span>All Nodes (*)</span>
        </span>
      );
    }

    return (
      <span style={{ fontSize: '0.72rem', color: 'var(--accent-cyan)', display: 'flex', alignItems: 'center', gap: '3px' }}>
        <Tag size={11} />
        <span>Restricted to: {rules.join(', ')}</span>
      </span>
    );
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: '720px' }}
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
                Manage team credentials, roles, and node tag permissions (RBAC)
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

        <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '18px' }}>
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
            <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
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

              <div style={{ width: '130px' }}>
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
            </div>

            <div style={{ display: 'flex', gap: '10px', alignItems: 'flex-end' }}>
              <div style={{ flex: 1 }}>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '4px' }}>
                  Allowed Nodes / Tags (comma-separated, e.g. <code>*</code>, <code>env:staging</code>, <code>vps-node-1</code>)
                </label>
                <input
                  type="text"
                  placeholder="* (for all nodes) or env:staging, team:payments"
                  value={newAllowedNodes}
                  onChange={(e) => setNewAllowedNodes(e.target.value)}
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
          </form>

          {/* User List */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', maxHeight: '280px', overflowY: 'auto' }}>
            {loading && users.length === 0 && (
              <div style={{ textAlign: 'center', padding: '20px', color: 'var(--text-dim)', fontSize: '0.85rem' }}>
                Loading users...
              </div>
            )}
            {users.map((u) => (
              <div
                key={u.id}
                style={{
                  background: 'var(--bg-surface)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '8px',
                  padding: '12px 14px',
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                }}
              >
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

                  <div style={{ marginTop: '4px' }}>
                    {renderAllowedNodesBadge(u.allowed_nodes)}
                  </div>

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

                <div>
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
            ))}
          </div>

          {/* Role & Tag Explanations */}
          <div
            style={{
              padding: '12px',
              borderRadius: '8px',
              background: 'rgba(255, 255, 255, 0.02)',
              border: '1px solid var(--border-subtle)',
              fontSize: '0.75rem',
              color: 'var(--text-dim)',
              display: 'flex',
              flexDirection: 'column',
              gap: '4px',
            }}
          >
            <div>
              <strong style={{ color: 'var(--text-muted)' }}>Tag-Based Node Restrictions:</strong>
            </div>
            <div>• <code>*</code>: Grants access to all current and future connected nodes.</div>
            <div>• <code>env:staging</code>: Grants access only to nodes enrolled with tag <code>env=staging</code>.</div>
            <div>• <code>node:vps-01</code>: Grants access only to specific agent node ID <code>vps-01</code>.</div>
          </div>
        </div>

        <div style={{ marginTop: '20px', display: 'flex', justifyContent: 'flex-end' }}>
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
