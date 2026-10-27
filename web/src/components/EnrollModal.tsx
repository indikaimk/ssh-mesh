import React, { useState } from 'react';
import { X, Copy, Check, ShieldCheck, Server } from 'lucide-react';

interface EnrollModalProps {
  isOpen: boolean;
  onClose: () => void;
  token: string;
}

export const EnrollModal: React.FC<EnrollModalProps> = ({ isOpen, onClose, token }) => {
  const [copiedCurl, setCopiedCurl] = useState(false);
  const [copiedBinary, setCopiedBinary] = useState(false);

  if (!isOpen) return null;

  const host = window.location.host;
  const isHttps = window.location.protocol === 'https:';
  const httpScheme = isHttps ? 'https' : 'http';
  const wsScheme = isHttps ? 'wss' : 'ws';

  const curlCmd = `curl -sSL ${httpScheme}://${host}/api/v1/install-script | sudo bash`;
  const binaryCmd = `./bin/ssh-mesh-agent -server ${wsScheme}://${host}/api/v1/agent/tunnel -token ${token}`;

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

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={(e) => e.stopPropagation()}>
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
              <Server size={18} />
            </div>
            <div>
              <h3 style={{ fontSize: '1.1rem', fontWeight: 600 }}>Enroll New Cloud VM</h3>
              <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>
                Connect a VPS or private subnet VM with no inbound ports required
              </p>
            </div>
          </div>
          <button className="tab-close" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: '18px', marginTop: '14px' }}>
          <div>
            <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'var(--text-main)', marginBottom: '6px' }}>
              Option 1: Quick Install (Linux VPS)
            </div>
            <p style={{ fontSize: '0.8rem', color: 'var(--text-dim)', marginBottom: '8px' }}>
              Downloads the agent and creates a background systemd service:
            </p>
            <div className="code-block">
              <code>{curlCmd}</code>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => handleCopy(curlCmd, 'curl')}
                style={{ marginLeft: '12px' }}
              >
                {copiedCurl ? <Check size={14} color="#10b981" /> : <Copy size={14} />}
                <span>{copiedCurl ? 'Copied' : 'Copy'}</span>
              </button>
            </div>
          </div>

          <div>
            <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'var(--text-main)', marginBottom: '6px' }}>
              Option 2: Direct Binary Execution (Testing / Dev)
            </div>
            <p style={{ fontSize: '0.8rem', color: 'var(--text-dim)', marginBottom: '8px' }}>
              Run the agent binary directly from terminal:
            </p>
            <div className="code-block">
              <code>{binaryCmd}</code>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => handleCopy(binaryCmd, 'binary')}
                style={{ marginLeft: '12px' }}
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
              <strong>Zero Inbound Firewall Exposure</strong>: The agent initiates an outbound-only connection. You never need to open port 22 or assign a public IPv4 address to the target node.
            </div>
          </div>
        </div>

        <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'flex-end' }}>
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
