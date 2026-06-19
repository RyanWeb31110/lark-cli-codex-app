package agent

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yjwong/lark-cli/internal/inbound"
	_ "modernc.org/sqlite"
)

const recentSessionParseLimit = 80

var codexThreadIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

type codexSessionSummary struct {
	ID            string
	Title         string
	Summary       string
	CWD           string
	UpdatedAt     string
	UpdatedAtTime time.Time
	Path          string
}

type codexSessionCatalog struct {
	home string
}

func defaultCodexSessionCatalog() codexSessionCatalog {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return codexSessionCatalog{home: home}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return codexSessionCatalog{home: filepath.Join(home, ".codex")}
	}
	return codexSessionCatalog{}
}

func (c codexSessionCatalog) Recent(limit int) ([]codexSessionSummary, error) {
	if limit <= 0 {
		return nil, nil
	}
	if strings.TrimSpace(c.home) == "" {
		return nil, nil
	}

	appSessions, err := c.appThreads(limit)
	if err == nil && len(appSessions) >= limit {
		return appSessions[:limit], nil
	}
	if err == nil && len(appSessions) > 0 {
		fallback, fallbackErr := c.rolloutSessions(limit-len(appSessions), idsForSessions(appSessions))
		if fallbackErr != nil {
			return appSessions, nil
		}
		return append(appSessions, fallback...), nil
	}

	return c.rolloutSessions(limit, nil)
}

func (c codexSessionCatalog) rolloutSessions(limit int, skipIDs map[string]bool) ([]codexSessionSummary, error) {
	if limit <= 0 {
		return nil, nil
	}
	index, _ := c.readIndex()
	candidates, err := c.sessionCandidates()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].updatedAt.After(candidates[j].updatedAt)
	})

	parseLimit := recentSessionParseLimit
	if limit*12 > parseLimit {
		parseLimit = limit * 12
	}

	results := make([]codexSessionSummary, 0, limit)
	seen := map[string]bool{}
	for i, candidate := range candidates {
		if i >= parseLimit && len(results) >= limit {
			break
		}
		session, err := parseCodexSessionFile(candidate.path)
		if err != nil {
			continue
		}
		if session.ID == "" {
			session.ID = codexThreadIDFromPath(candidate.path)
		}
		if session.ID == "" || seen[session.ID] || skipIDs[session.ID] || shouldHideCodexSession(session) {
			continue
		}
		if info, ok := index[session.ID]; ok {
			if strings.TrimSpace(info.Title) != "" {
				session.Title = info.Title
			}
			if info.UpdatedAtTime.After(session.UpdatedAtTime) {
				session.UpdatedAt = info.UpdatedAt
				session.UpdatedAtTime = info.UpdatedAtTime
			}
		}
		if session.UpdatedAtTime.IsZero() {
			session.UpdatedAtTime = candidate.updatedAt
			session.UpdatedAt = candidate.updatedAt.Format(time.RFC3339)
		}
		session.Path = candidate.path
		session.Title = sessionTitle(session)
		session.Summary = sessionSummary(session)
		seen[session.ID] = true
		results = append(results, session)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (c codexSessionCatalog) appThreads(limit int) ([]codexSessionSummary, error) {
	dbPath := filepath.Join(c.home, "state_5.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(1000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		select id, title, preview, first_user_message, cwd, source, thread_source,
		       archived, created_at, updated_at, created_at_ms, updated_at_ms
		from threads
		order by updated_at desc
		limit 200
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]codexSessionSummary, 0, limit)
	for rows.Next() {
		var id, title, preview, firstUserMessage, cwd, source string
		var threadSource sql.NullString
		var archived int
		var createdAt, updatedAt int64
		var createdAtMS, updatedAtMS sql.NullInt64
		if err := rows.Scan(&id, &title, &preview, &firstUserMessage, &cwd, &source, &threadSource, &archived, &createdAt, &updatedAt, &createdAtMS, &updatedAtMS); err != nil {
			return nil, err
		}
		if archived != 0 {
			continue
		}
		session := codexSessionSummary{
			ID:        strings.TrimSpace(id),
			Title:     strings.TrimSpace(title),
			Summary:   appThreadSummary(preview, firstUserMessage),
			CWD:       strings.TrimSpace(cwd),
			UpdatedAt: appThreadTime(updatedAt, updatedAtMS).Format(time.RFC3339),
		}
		session.UpdatedAtTime = appThreadTime(updatedAt, updatedAtMS)
		if shouldHideAppThread(title, preview, firstUserMessage, source, threadSource.String) || shouldHideCodexSession(session) {
			continue
		}
		session.Title = sessionTitle(session)
		session.Summary = sessionSummary(session)
		sessions = append(sessions, session)
		if len(sessions) >= limit {
			break
		}
		_ = createdAt
		_ = createdAtMS
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (c codexSessionCatalog) FindByID(id string) (codexSessionSummary, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return codexSessionSummary{}, false, nil
	}
	sessions, err := c.Recent(recentSessionParseLimit)
	if err != nil {
		return codexSessionSummary{}, false, err
	}
	for _, session := range sessions {
		if session.ID == id {
			return session, true, nil
		}
	}
	return codexSessionSummary{}, false, nil
}

func (c codexSessionCatalog) ResolveOrdinal(arg string, limit int) (codexSessionSummary, bool, error) {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n <= 0 {
		return codexSessionSummary{}, false, nil
	}
	sessions, err := c.Recent(limit)
	if err != nil {
		return codexSessionSummary{}, false, err
	}
	if n > len(sessions) {
		return codexSessionSummary{}, false, nil
	}
	return sessions[n-1], true, nil
}

type codexIndexEntry struct {
	Title         string
	UpdatedAt     string
	UpdatedAtTime time.Time
}

func (c codexSessionCatalog) readIndex() (map[string]codexIndexEntry, error) {
	path := filepath.Join(c.home, "session_index.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]codexIndexEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	entries := map[string]codexIndexEntry{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var payload struct {
			ID        string `json:"id"`
			Thread    string `json:"thread_name"`
			UpdatedAt string `json:"updated_at"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &payload); err != nil {
			continue
		}
		id := strings.TrimSpace(payload.ID)
		if id == "" {
			continue
		}
		updatedAtTime, _ := time.Parse(time.RFC3339Nano, payload.UpdatedAt)
		entries[id] = codexIndexEntry{
			Title:         strings.TrimSpace(payload.Thread),
			UpdatedAt:     strings.TrimSpace(payload.UpdatedAt),
			UpdatedAtTime: updatedAtTime,
		}
	}
	return entries, scanner.Err()
}

type sessionCandidate struct {
	path      string
	updatedAt time.Time
}

func (c codexSessionCatalog) sessionCandidates() ([]sessionCandidate, error) {
	root := filepath.Join(c.home, "sessions")
	candidates := []sessionCandidate{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		if codexThreadIDFromPath(path) == "" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		candidates = append(candidates, sessionCandidate{path: path, updatedAt: info.ModTime()})
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return candidates, err
}

func parseCodexSessionFile(path string) (codexSessionSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return codexSessionSummary{}, err
	}
	defer file.Close()

	session := codexSessionSummary{ID: codexThreadIDFromPath(path)}
	lastUserMessage := ""
	lastAssistantMessage := ""

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 256*1024), 16*1024*1024)
	for scanner.Scan() {
		var line struct {
			Timestamp string          `json:"timestamp"`
			Type      string          `json:"type"`
			Payload   json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
			session.UpdatedAtTime = parsed
			session.UpdatedAt = line.Timestamp
		}
		switch line.Type {
		case "session_meta":
			var payload struct {
				ID  string `json:"id"`
				CWD string `json:"cwd"`
			}
			if err := json.Unmarshal(line.Payload, &payload); err == nil {
				if strings.TrimSpace(payload.ID) != "" {
					session.ID = strings.TrimSpace(payload.ID)
				}
				if strings.TrimSpace(payload.CWD) != "" {
					session.CWD = strings.TrimSpace(payload.CWD)
				}
			}
		case "response_item":
			role, text, phase := responseItemText(line.Payload)
			switch role {
			case "user":
				if text = cleanSessionUserText(text); text != "" {
					lastUserMessage = text
				}
			case "assistant":
				if text != "" && (phase == "" || phase == "final_answer") {
					lastAssistantMessage = text
				}
			}
		case "event_msg":
			kind, message, phase := eventMessageText(line.Payload)
			switch kind {
			case "user_message":
				if text := cleanSessionUserText(message); text != "" {
					lastUserMessage = text
				}
			case "agent_message":
				if message != "" && (phase == "" || phase == "final_answer") {
					lastAssistantMessage = message
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return codexSessionSummary{}, err
	}

	if lastUserMessage != "" {
		session.Title = previewText(lastUserMessage, 60)
	}
	session.Summary = buildThreadSummary(lastUserMessage, lastAssistantMessage)
	return session, nil
}

func responseItemText(raw json.RawMessage) (role, text, phase string) {
	var payload struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Phase   string `json:"phase"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", ""
	}
	parts := []string{}
	for _, item := range payload.Content {
		if strings.TrimSpace(item.Text) != "" {
			parts = append(parts, item.Text)
		}
	}
	return payload.Role, strings.TrimSpace(strings.Join(parts, "\n")), payload.Phase
}

func eventMessageText(raw json.RawMessage) (kind, message, phase string) {
	var payload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Phase   string `json:"phase"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", ""
	}
	return payload.Type, strings.TrimSpace(payload.Message), payload.Phase
}

func cleanSessionUserText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if idx := strings.LastIndex(text, "用户消息："); idx >= 0 {
		text = strings.TrimSpace(text[idx+len("用户消息："):])
	}
	if strings.HasPrefix(text, "{") {
		if extracted := inbound.ExtractMessageText("", text); strings.TrimSpace(extracted) != "" && extracted != text {
			text = extracted
		}
	}
	return collapseWhitespace(text)
}

func codexThreadIDFromPath(path string) string {
	return codexThreadIDPattern.FindString(filepath.Base(path))
}

func sessionTitle(session codexSessionSummary) string {
	title := strings.TrimSpace(session.Title)
	if title == "" {
		return "未命名 Codex 会话"
	}
	return previewText(title, 48)
}

func sessionSummary(session codexSessionSummary) string {
	if strings.TrimSpace(session.Summary) != "" {
		return previewText(session.Summary, 180)
	}
	return "暂无摘要"
}

func formatSessionChoiceTime(session codexSessionSummary) string {
	if session.UpdatedAtTime.IsZero() {
		return "更新时间未知"
	}
	return session.UpdatedAtTime.Local().Format("01-02 15:04")
}

func formatSessionProject(session codexSessionSummary) string {
	cwd := strings.TrimSpace(session.CWD)
	if cwd == "" {
		return ""
	}
	return filepath.Base(cwd)
}

func appThreadSummary(preview, firstUserMessage string) string {
	text := strings.TrimSpace(preview)
	if text == "" {
		text = strings.TrimSpace(firstUserMessage)
	}
	text = cleanSessionUserText(text)
	if text == "" {
		return "暂无摘要"
	}
	return "用户消息：" + previewText(text, 160)
}

func appThreadTime(seconds int64, milliseconds sql.NullInt64) time.Time {
	if milliseconds.Valid && milliseconds.Int64 > 0 {
		return time.UnixMilli(milliseconds.Int64)
	}
	if seconds > 0 {
		return time.Unix(seconds, 0)
	}
	return time.Time{}
}

func shouldHideAppThread(title, preview, firstUserMessage, source, threadSource string) bool {
	source = strings.TrimSpace(source)
	threadSource = strings.TrimSpace(threadSource)
	if source == "exec" {
		return true
	}
	if threadSource != "" && threadSource != "user" {
		return true
	}
	text := strings.Join([]string{title, preview, firstUserMessage, source, threadSource}, "\n")
	text = strings.TrimSpace(text)
	if strings.Contains(text, "The following is the Codex agent history") {
		return true
	}
	if strings.Contains(text, "Reviewed Codex session id:") {
		return true
	}
	if strings.Contains(text, "你是一个本地 Codex 执行代理") {
		return true
	}
	if strings.Contains(text, "只回复 pong") {
		return true
	}
	if strings.Contains(text, `"subagent"`) {
		return true
	}
	return false
}

func shouldHideCodexSession(session codexSessionSummary) bool {
	text := strings.TrimSpace(strings.Join([]string{session.Title, session.Summary}, "\n"))
	if text == "" {
		return true
	}
	normalized := strings.Trim(strings.ToLower(collapseWhitespace(text)), "。？！?!,.，、")
	switch normalized {
	case "#status", "status", "只回复 pong，不要做其他事", "只回复 pong，不要做其他事。":
		return true
	}
	if strings.Contains(text, "The following is the Codex agent history") {
		return true
	}
	if strings.Contains(text, "你是一个本地 Codex 执行代理") {
		return true
	}
	if strings.Contains(text, "Reviewed Codex session id:") {
		return true
	}
	return false
}

func idsForSessions(sessions []codexSessionSummary) map[string]bool {
	ids := map[string]bool{}
	for _, session := range sessions {
		if strings.TrimSpace(session.ID) != "" {
			ids[session.ID] = true
		}
	}
	return ids
}
