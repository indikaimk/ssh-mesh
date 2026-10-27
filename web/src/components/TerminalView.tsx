import React, { useEffect, useRef, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { TerminalTab } from '../types';
import { RefreshCw, Trash2, Maximize2, Minimize2 } from 'lucide-react';

interface TerminalViewProps {
  tab: TerminalTab;
  onClose: (tabId: string) => void;
}

export const TerminalView: React.FC<TerminalViewProps> = ({ tab }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting');
  const [isFullscreen, setIsFullscreen] = useState(false);

  const connect = () => {
    if (!containerRef.current) return;
    setStatus('connecting');

    // Clean up previous terminal if any
    if (wsRef.current) {
      wsRef.current.close();
    }
    if (containerRef.current) {
      containerRef.current.innerHTML = '';
    }

    const term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
      fontSize: 14,
      lineHeight: 1.25,
      theme: {
        background: '#020617',
        foreground: '#f8fafc',
        cursor: '#38bdf8',
        selectionBackground: 'rgba(56, 189, 248, 0.3)',
        black: '#0f172a',
        red: '#ef4444',
        green: '#10b981',
        yellow: '#f59e0b',
        blue: '#3b82f6',
        magenta: '#a855f7',
        cyan: '#06b6d4',
        white: '#cbd5e1',
        brightBlack: '#334155',
        brightRed: '#f87171',
        brightGreen: '#34d399',
        brightYellow: '#fbbf24',
        brightBlue: '#60a5fa',
        brightMagenta: '#c084fc',
        brightCyan: '#22d3ee',
        brightWhite: '#f8fafc',
      },
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();

    term.loadAddon(fitAddon);
    term.loadAddon(webLinksAddon);
    term.open(containerRef.current);

    terminalRef.current = term;
    fitAddonRef.current = fitAddon;

    try {
      fitAddon.fit();
    } catch (e) {
      // Container might not be visible yet
    }

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${proto}//${window.location.host}/api/v1/ws/terminal?vm_id=${encodeURIComponent(
      tab.agentId
    )}&cols=${term.cols}&rows=${term.rows}`;

    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';
    wsRef.current = ws;

    ws.onopen = () => {
      setStatus('connected');
      term.focus();
      // Send initial resize
      ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
    };

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        term.write(event.data);
      } else if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
      }
    };

    ws.onclose = () => {
      setStatus('disconnected');
      term.write('\r\n\x1b[33m[Connection closed by remote host]\x1b[0m\r\n');
    };

    ws.onerror = () => {
      setStatus('error');
      term.write('\r\n\x1b[31m[WebSocket connection error]\x1b[0m\r\n');
    };

    // Forward user keystrokes to WebSocket
    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    // Handle resize
    const handleResize = () => {
      try {
        fitAddon.fit();
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(
            JSON.stringify({
              type: 'resize',
              cols: term.cols,
              rows: term.rows,
            })
          );
        }
      } catch (err) {
        // Ignored
      }
    };

    window.addEventListener('resize', handleResize);
    return () => {
      window.removeEventListener('resize', handleResize);
    };
  };

  useEffect(() => {
    const cleanup = connect();
    return () => {
      if (cleanup) cleanup();
      if (wsRef.current) wsRef.current.close();
      if (terminalRef.current) terminalRef.current.dispose();
    };
  }, [tab.id, tab.agentId]);

  const handleClear = () => {
    terminalRef.current?.clear();
  };

  const handleReconnect = () => {
    connect();
  };

  const toggleFullscreen = () => {
    setIsFullscreen(!isFullscreen);
    setTimeout(() => {
      fitAddonRef.current?.fit();
    }, 150);
  };

  return (
    <div
      className={`terminal-wrapper ${isFullscreen ? 'fullscreen-mode' : ''}`}
      style={
        isFullscreen
          ? {
              position: 'fixed',
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              zIndex: 9999,
            }
          : {}
      }
    >
      <div className="terminal-toolbar">
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <span style={{ fontWeight: 600, color: 'var(--text-main)', display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span
              className={`status-dot ${status === 'connected' ? 'online' : ''}`}
              style={{
                background:
                  status === 'connected'
                    ? 'var(--status-online)'
                    : status === 'connecting'
                    ? 'var(--status-warning)'
                    : 'var(--status-offline)',
              }}
            />
            {tab.hostname}
          </span>
          <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--accent-cyan)' }}>
            {tab.privateIp}
          </span>
          <span className="badge-tag" style={{ textTransform: 'capitalize' }}>
            {status}
          </span>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <button
            className="btn btn-secondary btn-sm"
            onClick={handleReconnect}
            title="Reconnect Session"
          >
            <RefreshCw size={13} />
            <span>Reconnect</span>
          </button>
          <button
            className="btn btn-secondary btn-sm"
            onClick={handleClear}
            title="Clear Terminal Screen"
          >
            <Trash2 size={13} />
            <span>Clear</span>
          </button>
          <button
            className="btn btn-secondary btn-sm"
            onClick={toggleFullscreen}
            title={isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'}
          >
            {isFullscreen ? <Minimize2 size={13} /> : <Maximize2 size={13} />}
          </button>
        </div>
      </div>

      <div
        ref={containerRef}
        className="terminal-canvas"
        style={{
          padding: '12px 14px',
          background: '#020617',
        }}
      />
    </div>
  );
};
