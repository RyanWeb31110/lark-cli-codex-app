package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yjwong/lark-cli/internal/api"
	"github.com/yjwong/lark-cli/internal/config"
	"github.com/yjwong/lark-cli/internal/inbound"
)

const (
	statusPending    = "pending"
	statusProcessing = "processing"
	statusCompleted  = "completed"
	statusFailed     = "failed"
)

var desktopIntentKeywords = []string{
	"computer use",
	"桌面 gui",
	"gui 操作",
	"gui操作",
	"桌面操作",
	"操作电脑",
	"操作桌面",
	"用鼠标",
	"在我电脑上",
	"本机桌面",
	"桌面自动化",
}

var desktopActionKeywords = []string{
	"打开",
	"点开",
	"点击",
	"单击",
	"双击",
	"右键",
	"启动",
	"关闭",
	"退出",
	"切换",
	"跳转",
	"访问",
	"输入",
	"键入",
	"粘贴",
	"复制",
	"拖动",
	"滚动",
	"滑动",
	"按下",
	"按一下",
	"回车",
	"刷新",
	"搜索",
	"open ",
	"click ",
	"double click",
	"right click",
	"launch ",
	"quit ",
	"close ",
	"switch ",
	"go to ",
	"visit ",
	"type ",
	"paste ",
	"copy ",
	"drag ",
	"scroll ",
	"press ",
	"search ",
}

var desktopTargetKeywords = []string{
	"safari",
	"chrome",
	"finder",
	"访达",
	"terminal",
	"iterm",
	"vscode",
	"xcode",
	"cursor",
	"notion",
	"preview",
	"system settings",
	"settings",
	"系统设置",
	"计算器",
	"飞书",
	"微信",
	"wechat",
	"浏览器",
	"桌面",
	"窗口",
	"标签页",
	"tab",
	"按钮",
	"菜单",
	"输入框",
	"弹窗",
	"页面",
	"链接",
	"复选框",
	"checkbox",
	"下拉",
	"dialog",
	"应用",
	"app",
	"鼠标",
	"键盘",
	"回车",
}

// Task represents one queued desktop GUI request.
type Task struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	ClaimedAt    string `json:"claimed_at,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
	MessageID    string `json:"message_id"`
	RootID       string `json:"root_id,omitempty"`
	ChatID       string `json:"chat_id"`
	SenderOpenID string `json:"sender_open_id,omitempty"`
	SenderUserID string `json:"sender_user_id,omitempty"`
	RequestText  string `json:"request_text"`
	Result       string `json:"result,omitempty"`
	Error        string `json:"error,omitempty"`
}

// Queue stores pending and processed desktop tasks on disk.
type Queue struct {
	rootDir string
	client  *api.Client
}

// NewQueue returns a persistent desktop task queue.
func NewQueue(rootDir string) *Queue {
	return &Queue{
		rootDir: rootDir,
		client:  api.NewClient(),
	}
}

// DefaultQueue returns the default queue derived from the Lark config dir.
func DefaultQueue() *Queue {
	return NewQueue(filepath.Join(config.GetConfigDir(), "desktop-tasks"))
}

// ExtractRequest returns the stripped GUI request when the message uses the GUI prefix.
func ExtractRequest(message string) (string, bool) {
	text := strings.TrimSpace(message)
	if text == "" {
		return "", false
	}

	if stripped, ok := stripExplicitPrefix(text); ok {
		return stripped, true
	}

	if looksLikeDesktopTask(text) {
		return text, true
	}

	return "", false
}

func stripExplicitPrefix(text string) (string, bool) {
	lower := strings.ToLower(text)
	switch {
	case strings.HasPrefix(lower, "/gui "):
		return strings.TrimSpace(text[5:]), true
	case strings.HasPrefix(lower, "/gui\n"):
		return strings.TrimSpace(text[5:]), true
	case strings.HasPrefix(lower, "gui:"):
		return strings.TrimSpace(text[4:]), true
	case strings.HasPrefix(lower, "gui："):
		return strings.TrimSpace(text[4:]), true
	default:
		return "", false
	}
}

func looksLikeDesktopTask(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	score := 0

	if containsAny(lower, desktopIntentKeywords) {
		score++
	}
	if containsAny(lower, desktopActionKeywords) {
		score++
	}
	if containsAny(lower, desktopTargetKeywords) {
		score++
	}

	return score >= 2
}

func containsAny(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// Enqueue creates a new pending desktop task.
func (q *Queue) Enqueue(entry inbound.LoggedEvent, requestText string) (*Task, error) {
	if strings.TrimSpace(requestText) == "" {
		return nil, fmt.Errorf("desktop GUI request is empty")
	}
	if err := q.ensureDirs(); err != nil {
		return nil, err
	}

	task := &Task{
		ID:           uuid.NewString(),
		Status:       statusPending,
		CreatedAt:    time.Now().Format(time.RFC3339Nano),
		MessageID:    entry.MessageID,
		RootID:       entry.RootID,
		ChatID:       entry.ChatID,
		SenderOpenID: entry.SenderOpenID,
		SenderUserID: entry.SenderUserID,
		RequestText:  requestText,
	}

	if err := q.writeTask(filepath.Join(q.pendingDir(), task.ID+".json"), task); err != nil {
		return nil, err
	}
	return task, nil
}

// PopPending atomically moves the oldest pending task into processing.
func (q *Queue) PopPending() (*Task, error) {
	if err := q.ensureDirs(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(q.pendingDir())
	if err != nil {
		return nil, fmt.Errorf("read pending tasks: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		src := filepath.Join(q.pendingDir(), entry.Name())
		dst := filepath.Join(q.processingDir(), entry.Name())
		if err := os.Rename(src, dst); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("claim desktop task: %w", err)
		}

		task, err := q.readTask(dst)
		if err != nil {
			return nil, err
		}
		task.Status = statusProcessing
		task.ClaimedAt = time.Now().Format(time.RFC3339Nano)
		if err := q.writeTask(dst, task); err != nil {
			return nil, err
		}
		return task, nil
	}

	return nil, nil
}

// Complete marks a processing task as completed and optionally replies in Feishu.
func (q *Queue) Complete(id, result string, reply bool) (*Task, error) {
	task, err := q.finishTask(id, statusCompleted, result, "")
	if err != nil {
		return nil, err
	}
	if reply {
		if err := q.reply(task, result); err != nil {
			return nil, err
		}
	}
	return task, nil
}

// Fail marks a processing task as failed and optionally replies in Feishu.
func (q *Queue) Fail(id, errorText string, reply bool) (*Task, error) {
	task, err := q.finishTask(id, statusFailed, "", errorText)
	if err != nil {
		return nil, err
	}
	if reply {
		if err := q.reply(task, "桌面 GUI 任务失败："+errorText); err != nil {
			return nil, err
		}
	}
	return task, nil
}

func (q *Queue) finishTask(id, status, result, errorText string) (*Task, error) {
	if err := q.ensureDirs(); err != nil {
		return nil, err
	}

	src := filepath.Join(q.processingDir(), id+".json")
	task, err := q.readTask(src)
	if err != nil {
		return nil, err
	}

	task.Status = status
	task.FinishedAt = time.Now().Format(time.RFC3339Nano)
	task.Result = strings.TrimSpace(result)
	task.Error = strings.TrimSpace(errorText)

	var dst string
	switch status {
	case statusCompleted:
		dst = filepath.Join(q.completedDir(), id+".json")
	case statusFailed:
		dst = filepath.Join(q.failedDir(), id+".json")
	default:
		return nil, fmt.Errorf("unsupported finish status: %s", status)
	}

	if err := q.writeTask(dst, task); err != nil {
		return nil, err
	}
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove processing task: %w", err)
	}
	return task, nil
}

func (q *Queue) reply(task *Task, text string) error {
	content, err := json.Marshal(map[string]string{"text": strings.TrimSpace(text)})
	if err != nil {
		return fmt.Errorf("marshal task reply: %w", err)
	}
	_, err = q.client.ReplyMessage(task.MessageID, "text", string(content), task.RootID, true)
	return err
}

// Reply sends a reply to the originating Feishu message for a task.
func (q *Queue) Reply(task *Task, text string) error {
	return q.reply(task, text)
}

func (q *Queue) ensureDirs() error {
	for _, dir := range []string{q.pendingDir(), q.processingDir(), q.completedDir(), q.failedDir()} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create desktop task directory %s: %w", dir, err)
		}
	}
	return nil
}

func (q *Queue) pendingDir() string    { return filepath.Join(q.rootDir, statusPending) }
func (q *Queue) processingDir() string { return filepath.Join(q.rootDir, statusProcessing) }
func (q *Queue) completedDir() string  { return filepath.Join(q.rootDir, statusCompleted) }
func (q *Queue) failedDir() string     { return filepath.Join(q.rootDir, statusFailed) }

func (q *Queue) writeTask(path string, task *Task) error {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal desktop task: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("write desktop task: %w", err)
	}
	return nil
}

func (q *Queue) readTask(path string) (*Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read desktop task: %w", err)
	}
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("parse desktop task: %w", err)
	}
	return &task, nil
}
