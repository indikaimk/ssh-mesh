package sshserver

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
	"golang.org/x/crypto/ssh"
)

// Server encapsulates the embedded SSH server running inside the agent.
type Server struct {
	config  *ssh.ServerConfig
	signer  ssh.Signer
	shell   string
	closeMu sync.Mutex
	closed  bool
}

// Config provides configuration options for the embedded SSH server.
type Config struct {
	Shell       string
	SharedToken string // Token used to authenticate the control plane SSH client
}

// NewServer initializes the embedded SSH server with an auto-generated or loaded host key.
func NewServer(cfg Config) (*Server, error) {
	// Generate an ephemeral ED25519 host key for the agent
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate host key: %w", err)
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	shellPath := cfg.Shell
	if shellPath == "" {
		if path, err := exec.LookPath("bash"); err == nil {
			shellPath = path
		} else if path, err := exec.LookPath("sh"); err == nil {
			shellPath = path
		} else {
			shellPath = "/bin/sh"
		}
	}

	sshConfig := &ssh.ServerConfig{
		// Authenticate via password or token sent as password from the control plane
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if cfg.SharedToken != "" && string(password) != cfg.SharedToken {
				return nil, errors.New("invalid password or token")
			}
			return &ssh.Permissions{
				Extensions: map[string]string{
					"user": conn.User(),
				},
			}, nil
		},
		NoClientAuth: cfg.SharedToken == "",
	}
	sshConfig.AddHostKey(signer)

	return &Server{
		config: sshConfig,
		signer: signer,
		shell:  shellPath,
	}, nil
}

// ServeListener accepts connections from a net.Listener (e.g. Yamux stream listener)
func (s *Server) ServeListener(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			s.closeMu.Lock()
			isClosed := s.closed
			s.closeMu.Unlock()
			if isClosed {
				return nil
			}
			return err
		}
		go s.HandleConn(conn)
	}
}

// HandleConn handles an individual incoming stream as an SSH connection.
func (s *Server) HandleConn(netConn net.Conn) {
	defer netConn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(netConn, s.config)
	if err != nil {
		log.Printf("[Agent SSH] Handshake failed: %v", err)
		return
	}
	defer sshConn.Close()

	// Discard out-of-band global requests
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		chType := newChannel.ChannelType()
		if chType != "session" && chType != "mesh-fs" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("[Agent SSH] Could not accept channel: %v", err)
			return
		}

		if chType == "mesh-fs" {
			go ssh.DiscardRequests(requests)
			go s.handleFileChannel(channel)
			continue
		}

		go s.handleSession(channel, requests, sshConn.User())
	}
}

func (s *Server) handleFileChannel(channel ssh.Channel) {
	defer channel.Close()

	reader := bufio.NewReader(channel)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}

	var req protocol.FileOpRequest
	if err := json.Unmarshal(line, &req); err != nil {
		writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: "invalid request json"})
		return
	}

	targetPath := strings.TrimSpace(req.Path)
	if targetPath == "" || targetPath == "~" || targetPath == "." {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			targetPath = home
		} else if cwd, err := os.Getwd(); err == nil {
			targetPath = cwd
		} else {
			targetPath = "/"
		}
	}
	if abs, err := filepath.Abs(targetPath); err == nil {
		targetPath = abs
	}

	switch req.Op {
	case "list":
		dirEntries, err := os.ReadDir(targetPath)
		if err != nil {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: err.Error(), Cwd: targetPath})
			return
		}
		entries := make([]protocol.FileEntry, 0, len(dirEntries))
		for _, de := range dirEntries {
			info, infoErr := de.Info()
			var size int64
			var modeStr string
			var modTimeStr string
			if infoErr == nil && info != nil {
				size = info.Size()
				modeStr = info.Mode().String()
				modTimeStr = info.ModTime().UTC().Format(time.RFC3339)
			}
			entries = append(entries, protocol.FileEntry{
				Name:    de.Name(),
				Path:    filepath.Join(targetPath, de.Name()),
				Size:    size,
				Mode:    modeStr,
				ModTime: modTimeStr,
				IsDir:   de.IsDir(),
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir != entries[j].IsDir {
				return entries[i].IsDir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		writeFileResp(channel, protocol.FileOpResponse{
			OK:      true,
			Cwd:     targetPath,
			Entries: entries,
		})

	case "download":
		info, err := os.Stat(targetPath)
		if err != nil {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: err.Error()})
			return
		}
		if info.IsDir() {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: "cannot download a directory"})
			return
		}
		f, err := os.Open(targetPath)
		if err != nil {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: err.Error()})
			return
		}
		defer f.Close()

		writeFileResp(channel, protocol.FileOpResponse{OK: true, Size: info.Size(), Cwd: targetPath})
		_, _ = io.Copy(channel, f)

	case "upload":
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: err.Error()})
			return
		}
		f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: err.Error()})
			return
		}
		// Acknowledge ready to receive file stream
		writeFileResp(channel, protocol.FileOpResponse{OK: true, Cwd: targetPath})
		written, copyErr := io.Copy(f, reader)
		_ = f.Close()
		if copyErr != nil {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: copyErr.Error()})
			return
		}
		writeFileResp(channel, protocol.FileOpResponse{OK: true, Size: written, Cwd: targetPath})

	case "mkdir":
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: err.Error()})
			return
		}
		writeFileResp(channel, protocol.FileOpResponse{OK: true, Cwd: targetPath})

	case "delete":
		if targetPath == "/" || targetPath == "" {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: "refusing to delete root directory"})
			return
		}
		if err := os.RemoveAll(targetPath); err != nil {
			writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: err.Error()})
			return
		}
		writeFileResp(channel, protocol.FileOpResponse{OK: true, Cwd: targetPath})

	default:
		writeFileResp(channel, protocol.FileOpResponse{OK: false, Error: "unknown file operation: " + req.Op})
	}
}

func writeFileResp(w io.Writer, resp protocol.FileOpResponse) {
	raw, _ := json.Marshal(resp)
	_, _ = w.Write(append(raw, '\n'))
}

func (s *Server) handleSession(channel ssh.Channel, requests <-chan *ssh.Request, targetUser string) {
	defer channel.Close()

	var (
		cmd     *exec.Cmd
		ptyFile *os.File
		ptyMu   sync.Mutex
		ptyCols = uint32(80)
		ptyRows = uint32(24)
	)

	for req := range requests {
		switch req.Type {
		case "pty-req":
			termLen := binary.BigEndian.Uint32(req.Payload[0:4])
			// Read cols and rows from payload
			if len(req.Payload) >= int(4+termLen+8) {
				ptyCols = binary.BigEndian.Uint32(req.Payload[4+termLen : 4+termLen+4])
				ptyRows = binary.BigEndian.Uint32(req.Payload[4+termLen+4 : 4+termLen+8])
			}
			_ = req.Reply(true, nil)

		case "window-change":
			if len(req.Payload) >= 8 {
				cols := binary.BigEndian.Uint32(req.Payload[0:4])
				rows := binary.BigEndian.Uint32(req.Payload[4:8])
				ptyMu.Lock()
				ptyCols = cols
				ptyRows = rows
				if ptyFile != nil {
					_ = pty.Setsize(ptyFile, &pty.Winsize{
						Cols: uint16(cols),
						Rows: uint16(rows),
					})
				}
				ptyMu.Unlock()
			}
			_ = req.Reply(true, nil)

		case "shell":
			isRoot := os.Geteuid() == 0
			if targetUser != "" && targetUser != "mesh-user" && targetUser != "default" {
				if isRoot && targetUser != "root" {
					cmd = exec.Command("su", "-l", targetUser)
				} else {
					cmd = exec.Command(s.shell)
				}
			} else {
				cmd = exec.Command(s.shell)
			}
			cmd.Env = append(os.Environ(), "TERM=xterm-256color")

			ptyMu.Lock()
			ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
				Cols: uint16(ptyCols),
				Rows: uint16(ptyRows),
			})
			if err != nil {
				ptyMu.Unlock()
				log.Printf("[Agent SSH] Failed to start PTY: %v", err)
				_ = req.Reply(false, nil)
				return
			}
			ptyFile = ptmx
			ptyMu.Unlock()

			_ = req.Reply(true, nil)

			// Pipe PTY <-> SSH Channel
			var once sync.Once
			closeAll := func() {
				ptyMu.Lock()
				if ptyFile != nil {
					_ = ptyFile.Close()
				}
				ptyMu.Unlock()
				_ = channel.Close()
			}

			go func() {
				_, _ = io.Copy(channel, ptmx)
				once.Do(closeAll)
			}()

			go func() {
				_, _ = io.Copy(ptmx, channel)
				once.Do(closeAll)
			}()

			// Wait for process termination
			go func() {
				_ = cmd.Wait()
				exitStatus := uint32(0)
				if cmd.ProcessState != nil {
					if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
						exitStatus = uint32(status.ExitStatus())
					}
				}
				statusPayload := make([]byte, 4)
				binary.BigEndian.PutUint32(statusPayload, exitStatus)
				_, _ = channel.SendRequest("exit-status", false, statusPayload)
				once.Do(closeAll)
			}()

		case "exec":
			// Support single command execution if requested
			if len(req.Payload) < 4 {
				_ = req.Reply(false, nil)
				continue
			}
			cmdLen := binary.BigEndian.Uint32(req.Payload[0:4])
			if len(req.Payload) < int(4+cmdLen) {
				_ = req.Reply(false, nil)
				continue
			}
			commandStr := string(req.Payload[4 : 4+cmdLen])

			cmd = exec.Command(s.shell, "-c", commandStr)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				_ = req.Reply(false, nil)
				continue
			}
			stderr, err := cmd.StderrPipe()
			if err != nil {
				_ = req.Reply(false, nil)
				continue
			}
			stdin, err := cmd.StdinPipe()
			if err != nil {
				_ = req.Reply(false, nil)
				continue
			}

			if err := cmd.Start(); err != nil {
				_ = req.Reply(false, nil)
				continue
			}
			_ = req.Reply(true, nil)

			go io.Copy(channel, stdout)
			go io.Copy(channel.Stderr(), stderr)
			go io.Copy(stdin, channel)

			go func() {
				_ = cmd.Wait()
				exitStatus := uint32(cmd.ProcessState.ExitCode())
				statusPayload := make([]byte, 4)
				binary.BigEndian.PutUint32(statusPayload, exitStatus)
				_, _ = channel.SendRequest("exit-status", false, statusPayload)
				_ = channel.Close()
			}()

		default:
			_ = req.Reply(false, nil)
		}
	}
}

// Close gracefully closes the embedded SSH server
func (s *Server) Close() {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.closed = true
}
