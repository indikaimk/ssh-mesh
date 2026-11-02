package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
	"golang.org/x/crypto/ssh"
)

func (api *ServerAPI) authorizeFileOp(w http.ResponseWriter, r *http.Request, vmID string, requireWrite bool) (string, *protocol.AgentInfo, bool) {
	if vmID == "" {
		http.Error(w, "missing vm_id parameter", http.StatusBadRequest)
		return "", nil, false
	}

	var tokenStr string
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		tokenStr = r.URL.Query().Get("token")
	}

	username := "admin"
	if api.authMgr != nil {
		if tokenStr == "" {
			http.Error(w, "unauthorized: missing token", http.StatusUnauthorized)
			return "", nil, false
		}
		claims, err := api.authMgr.VerifyJWT(tokenStr)
		if err != nil {
			http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
			return "", nil, false
		}
		username = claims.Username
		if requireWrite && claims.Role == "viewer" {
			if st := api.authMgr.GetStore(); st != nil {
				st.LogEvent(username, "file.access_denied", vmID, r.RemoteAddr, "denied", "Viewer role attempted file write/delete operation", nil)
			}
			http.Error(w, "forbidden: viewer role has read-only access", http.StatusForbidden)
			return "", nil, false
		}
	}

	agentInfo, err := api.broker.Get(vmID)
	if err != nil {
		http.Error(w, "agent not found or offline", http.StatusNotFound)
		return "", nil, false
	}

	if api.authMgr != nil && api.authMgr.GetStore() != nil {
		if userRecord, err := api.authMgr.GetStore().GetUser(username); err == nil && userRecord != nil {
			if !userRecord.CanAccessNode(agentInfo) {
				api.authMgr.GetStore().LogEvent(username, "file.access_denied", vmID, r.RemoteAddr, "denied", "Restricted by node/tag RBAC rules", nil)
				http.Error(w, "forbidden: you do not have permission to access this node", http.StatusForbidden)
				return "", nil, false
			}
		}
	}

	return username, agentInfo, true
}

func (api *ServerAPI) openAgentFileChannel(ctx context.Context, agentInfo *protocol.AgentInfo) (ssh.Channel, func(), error) {
	stream, err := api.broker.DialStream(ctx, agentInfo.ID)
	if err != nil {
		return nil, nil, err
	}

	sshPassword := agentInfo.Token
	if sshPassword == "" {
		sshPassword = "mesh-secret-token"
	}

	sshConfig := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(stream, "agent-ssh", sshConfig)
	if err != nil {
		_ = stream.Close()
		return nil, nil, fmt.Errorf("ssh handshake failed: %w", err)
	}

	sshClient := ssh.NewClient(sshConn, chans, reqs)
	ch, chReqs, err := sshClient.OpenChannel("mesh-fs", nil)
	if err != nil {
		_ = sshClient.Close()
		_ = stream.Close()
		return nil, nil, fmt.Errorf("failed to open mesh-fs channel: %w", err)
	}
	go ssh.DiscardRequests(chReqs)

	cleanup := func() {
		_ = ch.Close()
		_ = sshClient.Close()
		_ = stream.Close()
	}
	return ch, cleanup, nil
}

func sendFileOpReq(ch ssh.Channel, req protocol.FileOpRequest) error {
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = ch.Write(append(raw, '\n'))
	return err
}

func readFileOpResp(r *bufio.Reader) (*protocol.FileOpResponse, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var resp protocol.FileOpResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (api *ServerAPI) handleFilesList(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vmID := r.URL.Query().Get("vm_id")
	targetPath := r.URL.Query().Get("path")

	_, agentInfo, ok := api.authorizeFileOp(w, r, vmID, false)
	if !ok {
		return
	}

	ch, cleanup, err := api.openAgentFileChannel(r.Context(), agentInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer cleanup()

	if err := sendFileOpReq(ch, protocol.FileOpRequest{Op: "list", Path: targetPath}); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	reader := bufio.NewReader(ch)
	resp, err := readFileOpResp(reader)
	if err != nil {
		http.Error(w, "failed reading agent response: "+err.Error(), http.StatusBadGateway)
		return
	}
	if !resp.OK {
		http.Error(w, resp.Error, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (api *ServerAPI) handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vmID := r.URL.Query().Get("vm_id")
	targetPath := r.URL.Query().Get("path")
	if targetPath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	username, agentInfo, ok := api.authorizeFileOp(w, r, vmID, false)
	if !ok {
		return
	}

	ch, cleanup, err := api.openAgentFileChannel(r.Context(), agentInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer cleanup()

	if err := sendFileOpReq(ch, protocol.FileOpRequest{Op: "download", Path: targetPath}); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	reader := bufio.NewReader(ch)
	resp, err := readFileOpResp(reader)
	if err != nil {
		http.Error(w, "failed reading agent response: "+err.Error(), http.StatusBadGateway)
		return
	}
	if !resp.OK {
		http.Error(w, resp.Error, http.StatusBadRequest)
		return
	}

	if api.authMgr != nil && api.authMgr.GetStore() != nil {
		api.authMgr.GetStore().LogEvent(
			username,
			"file.download",
			vmID,
			r.RemoteAddr,
			"success",
			fmt.Sprintf("Downloaded file '%s' (%d bytes)", resp.Cwd, resp.Size),
			nil,
		)
	}

	fileName := filepath.Base(resp.Cwd)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	if resp.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.Size, 10))
	}
	_, _ = io.Copy(w, reader)
}

func (api *ServerAPI) handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vmID := r.URL.Query().Get("vm_id")
	destDir := r.URL.Query().Get("path")
	if destDir == "" {
		destDir = "."
	}

	username, agentInfo, ok := api.authorizeFileOp(w, r, vmID, true)
	if !ok {
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "invalid multipart upload: "+err.Error(), http.StatusBadRequest)
		return
	}

	var uploadedFiles []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "error reading multipart stream: "+err.Error(), http.StatusBadRequest)
			return
		}
		fileName := filepath.Base(part.FileName())
		if fileName == "" || fileName == "." || fileName == "/" {
			_ = part.Close()
			continue
		}

		destPath := filepath.Join(destDir, fileName)
		if err := api.streamFileUploadToAgent(r.Context(), agentInfo, destPath, part); err != nil {
			_ = part.Close()
			http.Error(w, fmt.Sprintf("upload failed for %s: %v", fileName, err), http.StatusBadGateway)
			return
		}
		_ = part.Close()
		uploadedFiles = append(uploadedFiles, destPath)

		if api.authMgr != nil && api.authMgr.GetStore() != nil {
			api.authMgr.GetStore().LogEvent(
				username,
				"file.upload",
				vmID,
				r.RemoteAddr,
				"success",
				fmt.Sprintf("Uploaded file to '%s'", destPath),
				nil,
			)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":       true,
		"uploaded": uploadedFiles,
	})
}

func (api *ServerAPI) streamFileUploadToAgent(ctx context.Context, agentInfo *protocol.AgentInfo, destPath string, src io.Reader) error {
	ch, cleanup, err := api.openAgentFileChannel(ctx, agentInfo)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := sendFileOpReq(ch, protocol.FileOpRequest{Op: "upload", Path: destPath}); err != nil {
		return err
	}

	reader := bufio.NewReader(ch)
	readyResp, err := readFileOpResp(reader)
	if err != nil {
		return err
	}
	if !readyResp.OK {
		return fmt.Errorf("%s", readyResp.Error)
	}

	if _, err := io.Copy(ch, src); err != nil {
		return err
	}
	_ = ch.CloseWrite()

	finalResp, err := readFileOpResp(reader)
	if err != nil {
		return err
	}
	if !finalResp.OK {
		return fmt.Errorf("%s", finalResp.Error)
	}
	return nil
}

func (api *ServerAPI) handleFilesMkdir(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		VMID string `json:"vm_id"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	username, agentInfo, ok := api.authorizeFileOp(w, r, req.VMID, true)
	if !ok {
		return
	}

	ch, cleanup, err := api.openAgentFileChannel(r.Context(), agentInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer cleanup()

	if err := sendFileOpReq(ch, protocol.FileOpRequest{Op: "mkdir", Path: req.Path}); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	reader := bufio.NewReader(ch)
	resp, err := readFileOpResp(reader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !resp.OK {
		http.Error(w, resp.Error, http.StatusBadRequest)
		return
	}

	if api.authMgr != nil && api.authMgr.GetStore() != nil {
		api.authMgr.GetStore().LogEvent(username, "file.mkdir", req.VMID, r.RemoteAddr, "success", fmt.Sprintf("Created directory '%s'", resp.Cwd), nil)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (api *ServerAPI) handleFilesDelete(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vmID := r.URL.Query().Get("vm_id")
	targetPath := r.URL.Query().Get("path")
	if targetPath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	username, agentInfo, ok := api.authorizeFileOp(w, r, vmID, true)
	if !ok {
		return
	}

	ch, cleanup, err := api.openAgentFileChannel(r.Context(), agentInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer cleanup()

	if err := sendFileOpReq(ch, protocol.FileOpRequest{Op: "delete", Path: targetPath}); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	reader := bufio.NewReader(ch)
	resp, err := readFileOpResp(reader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !resp.OK {
		http.Error(w, resp.Error, http.StatusBadRequest)
		return
	}

	if api.authMgr != nil && api.authMgr.GetStore() != nil {
		api.authMgr.GetStore().LogEvent(username, "file.delete", vmID, r.RemoteAddr, "success", fmt.Sprintf("Deleted '%s'", resp.Cwd), nil)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

