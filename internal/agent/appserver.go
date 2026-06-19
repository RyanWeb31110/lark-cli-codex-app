package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

type appServerClient struct {
	cfg    Config
	logger *log.Logger

	mu            sync.Mutex
	startMu       sync.Mutex
	pendingMu     sync.Mutex
	nextID        int64
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	cancel        context.CancelFunc
	waitCh        chan error
	notifications chan appServerMessage
	pending       map[int64]chan appServerMessage
}

type appServerMessage struct {
	ID     *int64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *appServerError `json:"error,omitempty"`
}

type appServerError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func newAppServerClient(cfg Config, logger *log.Logger) *appServerClient {
	if logger == nil {
		logger = log.New(os.Stderr, "lark-agent: ", log.LstdFlags)
	}
	return &appServerClient{
		cfg:           cfg,
		logger:        logger,
		notifications: make(chan appServerMessage, 256),
		pending:       map[int64]chan appServerMessage{},
	}
}

func (c *appServerClient) Execute(ctx context.Context, input, threadID string) (string, string, error) {
	if c == nil {
		return "", "", fmt.Errorf("Codex app-server client is not initialized")
	}
	if err := c.ensureStarted(ctx); err != nil {
		return "", "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	threadID = strings.TrimSpace(threadID)
	var err error
	if threadID != "" {
		if err := c.resumeThread(ctx, threadID); err != nil {
			c.logger.Printf("resume Codex thread failed; trying direct turn/start thread_id=%s: %v", threadID, err)
		}
	} else {
		threadID, err = c.startThread(ctx, false)
		if err != nil {
			return "", "", err
		}
	}
	if err := c.startTurn(ctx, threadID, input); err != nil {
		return "", "", err
	}

	var builder strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", threadID, fmt.Errorf("任务超时，超过 %s", c.cfg.Timeout)
		case err := <-c.waitCh:
			c.reset()
			if err == nil {
				return "", threadID, fmt.Errorf("Codex app-server 已退出")
			}
			return "", threadID, fmt.Errorf("Codex app-server 已退出: %w", err)
		case msg := <-c.notifications:
			if !messageThreadMatches(msg.Params, threadID) {
				continue
			}
			switch msg.Method {
			case "item/agentMessage/delta":
				builder.WriteString(messageStringParam(msg.Params, "delta"))
			case "item/completed":
				if builder.Len() == 0 {
					builder.WriteString(completedAgentMessage(msg.Params))
				}
			case "turn/completed":
				if err := turnCompletionError(msg.Params); err != nil {
					return "", threadID, err
				}
				result := strings.TrimSpace(builder.String())
				if result == "" {
					result = strings.TrimSpace(completedAgentMessage(msg.Params))
				}
				if result == "" {
					return "", threadID, fmt.Errorf("Codex 没有返回结果")
				}
				return trimForFeishu(result, c.cfg.ResultMaxChars), threadID, nil
			case "error":
				text := strings.TrimSpace(messageStringParam(msg.Params, "message"))
				if text != "" {
					return "", threadID, fmt.Errorf("%s", text)
				}
			}
		}
	}
}

func (c *appServerClient) NewThread(ctx context.Context) (string, error) {
	if c == nil {
		return "", fmt.Errorf("Codex app-server client is not initialized")
	}
	if err := c.ensureStarted(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startThread(ctx, false)
}

func (c *appServerClient) ResumeThread(ctx context.Context, threadID string) error {
	if c == nil {
		return fmt.Errorf("Codex app-server client is not initialized")
	}
	if err := c.ensureStarted(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resumeThread(ctx, threadID)
}

func (c *appServerClient) Close() error {
	c.startMu.Lock()
	defer c.startMu.Unlock()

	c.stopLocked()
	return nil
}

func (c *appServerClient) stopLocked() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	c.reset()
}

func (c *appServerClient) ensureStarted(ctx context.Context) error {
	c.startMu.Lock()
	defer c.startMu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		if c.waitCh != nil {
			select {
			case err := <-c.waitCh:
				if err != nil {
					c.logger.Printf("Codex app-server exited before reuse: %v", err)
				}
				c.stopLocked()
			default:
				return nil
			}
		} else {
			return nil
		}
	}

	if c.cmd != nil && c.cmd.Process != nil {
		return nil
	}

	codexBinary, err := resolveCodexBinary(c.cfg.CodexBinary)
	if err != nil {
		return err
	}

	procCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(procCtx, codexBinary, "app-server")
	cmd.Env = codexEnv(os.Environ())
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("open app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("open app-server stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start Codex app-server: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.cancel = cancel
	c.waitCh = make(chan error, 1)
	c.notifications = make(chan appServerMessage, 256)
	c.pending = map[int64]chan appServerMessage{}

	go c.readLoop(stdout)
	go func() {
		c.waitCh <- cmd.Wait()
	}()

	if err := c.initialize(ctx); err != nil {
		c.stopLocked()
		return err
	}
	return nil
}

func (c *appServerClient) initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"clientInfo": map[string]interface{}{
			"name":    "lark_cli_codex_app",
			"title":   "Lark Codex Bridge",
			"version": "0.1.0",
		},
	}
	if _, err := c.sendRequest(ctx, "initialize", params); err != nil {
		return fmt.Errorf("initialize Codex app-server: %w", err)
	}
	return c.sendNotification("initialized", nil)
}

func (c *appServerClient) startThread(ctx context.Context, ephemeral bool) (string, error) {
	params := map[string]interface{}{
		"approvalPolicy":        "never",
		"cwd":                   c.cfg.Workspace,
		"developerInstructions": larkBridgeDeveloperInstructions(c.cfg.ResultMaxChars),
		"ephemeral":             ephemeral,
		"sandbox":               "workspace-write",
		"serviceName":           "Lark Bridge",
	}
	if strings.TrimSpace(c.cfg.Model) != "" {
		params["model"] = strings.TrimSpace(c.cfg.Model)
	}

	result, err := c.sendRequest(ctx, "thread/start", params)
	if err != nil {
		return "", fmt.Errorf("start Codex thread: %w", err)
	}
	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return "", fmt.Errorf("parse thread/start response: %w", err)
	}
	if response.Thread.ID == "" {
		return "", fmt.Errorf("thread/start response missing thread id")
	}
	return response.Thread.ID, nil
}

func (c *appServerClient) resumeThread(ctx context.Context, threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("Codex thread id is required")
	}
	params := map[string]interface{}{
		"approvalPolicy": "never",
		"cwd":            c.cfg.Workspace,
		"sandbox":        "workspace-write",
		"threadId":       threadID,
	}
	if strings.TrimSpace(c.cfg.Model) != "" {
		params["model"] = strings.TrimSpace(c.cfg.Model)
	}
	result, err := c.sendRequest(ctx, "thread/resume", params)
	if err != nil {
		return fmt.Errorf("resume Codex thread %s: %w", threadID, err)
	}
	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("parse thread/resume response: %w", err)
	}
	if response.Thread.ID == "" {
		return fmt.Errorf("thread/resume response missing thread id")
	}
	return nil
}

func (c *appServerClient) startTurn(ctx context.Context, threadID, prompt string) error {
	params := map[string]interface{}{
		"approvalPolicy": "never",
		"threadId":       threadID,
		"cwd":            c.cfg.Workspace,
		"input": []map[string]string{
			{
				"type": "text",
				"text": prompt,
			},
		},
	}
	if strings.TrimSpace(c.cfg.Model) != "" {
		params["model"] = strings.TrimSpace(c.cfg.Model)
	}
	if strings.TrimSpace(c.cfg.ReasoningEffort) != "" {
		params["effort"] = strings.TrimSpace(c.cfg.ReasoningEffort)
	}
	_, err := c.sendRequest(ctx, "turn/start", params)
	if err != nil {
		return fmt.Errorf("start Codex turn: %w", err)
	}
	return nil
}

func (c *appServerClient) sendRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan appServerMessage, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.send(appServerMessage{ID: &id, Method: method, Params: mustMarshalRaw(params)}); err != nil {
		c.deletePending(id)
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.deletePending(id)
		return nil, ctx.Err()
	case err := <-c.waitCh:
		c.deletePending(id)
		c.reset()
		if err == nil {
			return nil, fmt.Errorf("Codex app-server exited")
		}
		return nil, fmt.Errorf("Codex app-server exited: %w", err)
	case msg := <-ch:
		if msg.Error != nil {
			return nil, fmt.Errorf("app-server error %d: %s", msg.Error.Code, msg.Error.Message)
		}
		return msg.Result, nil
	}
}

func (c *appServerClient) sendNotification(method string, params interface{}) error {
	msg := appServerMessage{Method: method}
	if params != nil {
		msg.Params = mustMarshalRaw(params)
	}
	return c.send(msg)
}

func (c *appServerClient) send(msg appServerMessage) error {
	if c.stdin == nil {
		return fmt.Errorf("Codex app-server stdin is not open")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := c.stdin.Write(payload); err != nil {
		c.reset()
		return fmt.Errorf("write Codex app-server request: %w", err)
	}
	return nil
}

func (c *appServerClient) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var msg appServerMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			c.logger.Printf("ignore invalid app-server message: %v", err)
			continue
		}
		if msg.ID != nil {
			c.pendingMu.Lock()
			ch := c.pending[*msg.ID]
			delete(c.pending, *msg.ID)
			c.pendingMu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		select {
		case c.notifications <- msg:
		default:
			c.logger.Printf("drop app-server notification method=%s: buffer full", msg.Method)
		}
	}
	if err := scanner.Err(); err != nil {
		c.logger.Printf("Codex app-server read loop stopped: %v", err)
	}
}

func (c *appServerClient) deletePending(id int64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *appServerClient) reset() {
	c.cmd = nil
	c.stdin = nil
	c.cancel = nil
}

func mustMarshalRaw(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func messageThreadMatches(params json.RawMessage, threadID string) bool {
	if threadID == "" {
		return true
	}
	var payload struct {
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return false
	}
	return payload.ThreadID == threadID
}

func messageStringParam(params json.RawMessage, name string) string {
	var payload map[string]interface{}
	if err := json.Unmarshal(params, &payload); err != nil {
		return ""
	}
	value, _ := payload[name].(string)
	return value
}

func completedAgentMessage(params json.RawMessage) string {
	var payload struct {
		Item struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
		Turn struct {
			Items []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"items"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return ""
	}
	if payload.Item.Type == "agentMessage" {
		return payload.Item.Text
	}
	for i := len(payload.Turn.Items) - 1; i >= 0; i-- {
		if payload.Turn.Items[i].Type == "agentMessage" {
			return payload.Turn.Items[i].Text
		}
	}
	return ""
}

func turnCompletionError(params json.RawMessage) error {
	var payload struct {
		Turn struct {
			Status string `json:"status"`
			Error  *struct {
				Message           string `json:"message"`
				AdditionalDetails string `json:"additionalDetails"`
			} `json:"error"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil
	}
	if payload.Turn.Status == "" || payload.Turn.Status == "completed" {
		return nil
	}
	if payload.Turn.Error != nil && strings.TrimSpace(payload.Turn.Error.Message) != "" {
		message := strings.TrimSpace(payload.Turn.Error.Message)
		if strings.TrimSpace(payload.Turn.Error.AdditionalDetails) != "" {
			message += ": " + strings.TrimSpace(payload.Turn.Error.AdditionalDetails)
		}
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("Codex turn ended with status %s", payload.Turn.Status)
}
