package wsconn

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// NetConn wraps a gorilla/websocket.Conn to implement the standard net.Conn interface.
// This allows Yamux or any stream multiplexer to run directly over WebSocket.
type NetConn struct {
	ws     *websocket.Conn
	reader io.Reader
	mu     sync.Mutex
}

// Wrap returns a net.Conn backed by a WebSocket.
func Wrap(ws *websocket.Conn) *NetConn {
	return &NetConn{ws: ws}
}

func (c *NetConn) Read(b []byte) (int, error) {
	for {
		if c.reader == nil {
			messageType, r, err := c.ws.NextReader()
			if err != nil {
				return 0, err
			}
			if messageType != websocket.BinaryMessage && messageType != websocket.TextMessage {
				continue
			}
			c.reader = r
		}

		n, err := c.reader.Read(b)
		if err == io.EOF {
			c.reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (c *NetConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.ws.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *NetConn) Close() error {
	return c.ws.Close()
}

func (c *NetConn) LocalAddr() net.Addr {
	return c.ws.LocalAddr()
}

func (c *NetConn) RemoteAddr() net.Addr {
	return c.ws.RemoteAddr()
}

func (c *NetConn) SetDeadline(t time.Time) error {
	if err := c.ws.SetReadDeadline(t); err != nil {
		return err
	}
	return c.ws.SetWriteDeadline(t)
}

func (c *NetConn) SetReadDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

func (c *NetConn) SetWriteDeadline(t time.Time) error {
	return c.ws.SetWriteDeadline(t)
}
