import React, { useState, useEffect, useRef } from 'react';
import {
  X,
  Folder,
  File,
  Upload,
  Download,
  Trash2,
  FolderPlus,
  RefreshCw,
  ArrowUp,
  Home,
} from 'lucide-react';
import { FileEntry } from '../types';

interface FileManagerModalProps {
  isOpen: boolean;
  onClose: () => void;
  vmId: string;
  hostname: string;
  authToken: string;
  userRole: string;
}

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
};

export const FileManagerModal: React.FC<FileManagerModalProps> = ({
  isOpen,
  onClose,
  vmId,
  hostname,
  authToken,
  userRole,
}) => {
  const [cwd, setCwd] = useState<string>('');
  const [pathInput, setPathInput] = useState<string>('');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [isDragging, setIsDragging] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // New folder state
  const [showNewFolder, setShowNewFolder] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');

  const fileInputRef = useRef<HTMLInputElement>(null);

  const fetchDir = async (targetPath: string) => {
    if (!authToken || !vmId) return;
    setLoading(true);
    setError(null);
    try {
      const q = new URLSearchParams({
        vm_id: vmId,
        path: targetPath,
      });
      const res = await fetch(`/api/v1/files/list?${q.toString()}`, {
        headers: { Authorization: `Bearer ${authToken}` },
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to list directory');
      }
      const data = await res.json();
      setCwd(data.cwd || targetPath || '/');
      setPathInput(data.cwd || targetPath || '/');
      setEntries(data.entries || []);
    } catch (err: any) {
      setError(err.message || 'Error listing files');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen && vmId) {
      fetchDir('');
      setShowNewFolder(false);
      setNewFolderName('');
    }
  }, [isOpen, vmId]);

  if (!isOpen) return null;

  const navigateUp = () => {
    if (!cwd || cwd === '/') return;
    const parts = cwd.split('/').filter(Boolean);
    parts.pop();
    const parent = '/' + parts.join('/');
    fetchDir(parent);
  };

  const handleUploadFiles = async (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0) return;
    if (userRole === 'viewer') {
      setError('Viewer accounts have read-only access and cannot upload files.');
      return;
    }

    setUploading(true);
    setError(null);

    const formData = new FormData();
    Array.from(fileList).forEach((f) => {
      formData.append('file', f);
    });

    try {
      const q = new URLSearchParams({
        vm_id: vmId,
        path: cwd || '.',
      });
      const res = await fetch(`/api/v1/files/upload?${q.toString()}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${authToken}` },
        body: formData,
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Upload failed');
      }
      await fetchDir(cwd);
    } catch (err: any) {
      setError(err.message || 'Upload failed');
    } finally {
      setUploading(false);
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    }
  };

  const handleCreateFolder = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newFolderName.trim()) return;
    setError(null);
    const fullPath = cwd === '/' ? `/${newFolderName.trim()}` : `${cwd}/${newFolderName.trim()}`;

    try {
      const res = await fetch('/api/v1/files/mkdir', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${authToken}`,
        },
        body: JSON.stringify({
          vm_id: vmId,
          path: fullPath,
        }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Failed to create folder');
      }
      setNewFolderName('');
      setShowNewFolder(false);
      await fetchDir(cwd);
    } catch (err: any) {
      setError(err.message || 'Failed to create folder');
    }
  };

  const handleDelete = async (entry: FileEntry) => {
    if (!confirm(`Delete ${entry.is_dir ? 'folder' : 'file'} "${entry.name}"?`)) return;
    setError(null);
    try {
      const q = new URLSearchParams({
        vm_id: vmId,
        path: entry.path,
      });
      const res = await fetch(`/api/v1/files?${q.toString()}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${authToken}` },
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Delete failed');
      }
      await fetchDir(cwd);
    } catch (err: any) {
      setError(err.message || 'Failed to delete');
    }
  };

  const getDownloadUrl = (filePath: string) => {
    const q = new URLSearchParams({
      vm_id: vmId,
      path: filePath,
      token: authToken,
    });
    return `/api/v1/files/download?${q.toString()}`;
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: '840px', width: '92vw' }}
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
              <Folder size={18} />
            </div>
            <div>
              <h3 style={{ fontSize: '1.12rem', fontWeight: 600 }}>
                File Manager &mdash; {hostname}
              </h3>
              <p style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>
                Browse directories, download files, or drag-and-drop files to upload over the encrypted reverse tunnel
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

        {/* Navigation & Action Bar */}
        <div
          style={{
            marginTop: '14px',
            display: 'flex',
            gap: '8px',
            alignItems: 'center',
            flexWrap: 'wrap',
          }}
        >
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => fetchDir('')}
            title="Home Directory"
          >
            <Home size={14} />
          </button>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={navigateUp}
            disabled={!cwd || cwd === '/'}
            title="Parent Directory (..)"
          >
            <ArrowUp size={14} />
          </button>

          <form
            onSubmit={(e) => {
              e.preventDefault();
              fetchDir(pathInput.trim());
            }}
            style={{ flex: 1, display: 'flex', gap: '6px', minWidth: '200px' }}
          >
            <input
              type="text"
              value={pathInput}
              onChange={(e) => setPathInput(e.target.value)}
              placeholder="/home/ubuntu"
              style={{
                flex: 1,
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '6px',
                padding: '6px 10px',
                color: 'var(--text-main)',
                fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
                fontSize: '0.8rem',
                outline: 'none',
              }}
            />
            <button type="submit" className="btn btn-secondary btn-sm">
              Go
            </button>
          </form>

          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => fetchDir(cwd)}
            title="Refresh"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>

          {userRole !== 'viewer' && (
            <>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => setShowNewFolder(!showNewFolder)}
              >
                <FolderPlus size={14} />
                <span>New Folder</span>
              </button>

              <input
                ref={fileInputRef}
                type="file"
                multiple
                style={{ display: 'none' }}
                onChange={(e) => handleUploadFiles(e.target.files)}
              />
              <button
                type="button"
                className="btn btn-primary btn-sm"
                disabled={uploading}
                onClick={() => fileInputRef.current?.click()}
              >
                <Upload size={14} />
                <span>{uploading ? 'Uploading...' : 'Upload Files'}</span>
              </button>
            </>
          )}
        </div>

        {/* Optional New Folder Form */}
        {showNewFolder && (
          <form
            onSubmit={handleCreateFolder}
            style={{
              marginTop: '10px',
              display: 'flex',
              gap: '8px',
              alignItems: 'center',
              background: 'var(--bg-primary)',
              padding: '8px 12px',
              borderRadius: '6px',
              border: '1px solid var(--border-subtle)',
            }}
          >
            <span style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>Folder Name:</span>
            <input
              type="text"
              placeholder="e.g. backups"
              value={newFolderName}
              onChange={(e) => setNewFolderName(e.target.value)}
              autoFocus
              style={{
                flex: 1,
                background: 'var(--bg-surface)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '6px',
                padding: '5px 10px',
                color: 'var(--text-main)',
                fontSize: '0.8rem',
                outline: 'none',
              }}
            />
            <button type="submit" className="btn btn-primary btn-sm">
              Create
            </button>
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => setShowNewFolder(false)}
            >
              Cancel
            </button>
          </form>
        )}

        {/* File List + Drag and Drop Zone */}
        <div
          onDragOver={(e) => {
            e.preventDefault();
            if (userRole !== 'viewer') setIsDragging(true);
          }}
          onDragLeave={() => setIsDragging(false)}
          onDrop={(e) => {
            e.preventDefault();
            setIsDragging(false);
            if (userRole !== 'viewer') {
              handleUploadFiles(e.dataTransfer.files);
            }
          }}
          style={{
            marginTop: '12px',
            background: isDragging ? 'rgba(56, 189, 248, 0.08)' : 'var(--bg-primary)',
            border: isDragging
              ? '2px dashed var(--accent-cyan)'
              : '1px solid var(--border-subtle)',
            borderRadius: '8px',
            maxHeight: '380px',
            minHeight: '260px',
            overflowY: 'auto',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          {loading && entries.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-dim)', fontSize: '0.85rem' }}>
              Loading directory contents...
            </div>
          ) : entries.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-dim)', fontSize: '0.85rem' }}>
              Directory is empty. {userRole !== 'viewer' && 'Drag & drop files here to upload.'}
            </div>
          ) : (
            entries.map((entry) => (
              <div
                key={entry.path}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '8px 12px',
                  borderBottom: '1px solid rgba(255, 255, 255, 0.04)',
                  fontSize: '0.82rem',
                }}
              >
                <div
                  onClick={() => {
                    if (entry.is_dir) {
                      fetchDir(entry.path);
                    }
                  }}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '10px',
                    cursor: entry.is_dir ? 'pointer' : 'default',
                    flex: 1,
                    minWidth: 0,
                  }}
                >
                  {entry.is_dir ? (
                    <Folder size={16} color="var(--accent-cyan)" style={{ flexShrink: 0 }} />
                  ) : (
                    <File size={16} color="var(--text-muted)" style={{ flexShrink: 0 }} />
                  )}
                  <span
                    style={{
                      color: entry.is_dir ? 'var(--accent-cyan)' : '#f8fafc',
                      fontWeight: entry.is_dir ? 600 : 400,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {entry.name}
                  </span>
                </div>

                <div style={{ display: 'flex', alignItems: 'center', gap: '14px', flexShrink: 0 }}>
                  <span
                    style={{
                      fontSize: '0.73rem',
                      color: 'var(--text-dim)',
                      fontFamily: "'JetBrains Mono', monospace",
                      width: '86px',
                      textAlign: 'right',
                    }}
                  >
                    {entry.is_dir ? '--' : formatBytes(entry.size)}
                  </span>
                  <span
                    style={{
                      fontSize: '0.72rem',
                      color: 'var(--text-dim)',
                      fontFamily: "'JetBrains Mono', monospace",
                      width: '90px',
                    }}
                  >
                    {entry.mode}
                  </span>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px', width: '64px', justifyContent: 'flex-end' }}>
                    {!entry.is_dir && (
                      <a
                        href={getDownloadUrl(entry.path)}
                        download={entry.name}
                        className="btn btn-secondary btn-sm"
                        style={{ padding: '4px 7px' }}
                        title={`Download ${entry.name}`}
                      >
                        <Download size={13} />
                      </a>
                    )}
                    {userRole !== 'viewer' && (
                      <button
                        type="button"
                        className="btn btn-secondary btn-sm"
                        style={{ padding: '4px 7px', color: '#ef4444' }}
                        onClick={() => handleDelete(entry)}
                        title={`Delete ${entry.name}`}
                      >
                        <Trash2 size={13} />
                      </button>
                    )}
                  </div>
                </div>
              </div>
            ))
          )}
        </div>

        <div
          style={{
            marginTop: '12px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            fontSize: '0.75rem',
            color: 'var(--text-dim)',
          }}
        >
          <span>
            {entries.length} items &bull; Drag and drop files anywhere into the list to upload to <code>{cwd}</code>
          </span>
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
};

