package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yjwong/lark-cli/internal/inbound"
)

type ThreadBinding struct {
	LarkKey         string `json:"lark_key"`
	ChatID          string `json:"chat_id,omitempty"`
	ThreadID        string `json:"thread_id,omitempty"`
	RootID          string `json:"root_id,omitempty"`
	CodexThreadID   string `json:"codex_thread_id"`
	Summary         string `json:"summary,omitempty"`
	LastUserMessage string `json:"last_user_message,omitempty"`
	LastResult      string `json:"last_result,omitempty"`
	LastActivityAt  string `json:"last_activity_at,omitempty"`
	Manual          bool   `json:"manual,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type threadBindingFile struct {
	Version  int                      `json:"version"`
	Bindings map[string]ThreadBinding `json:"bindings"`
}

type ThreadBindingStore struct {
	path string
	mu   sync.Mutex
}

func NewThreadBindingStore(path string) *ThreadBindingStore {
	return &ThreadBindingStore{path: strings.TrimSpace(path)}
}

func (s *ThreadBindingStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *ThreadBindingStore) Find(entry inbound.LoggedEvent) (ThreadBinding, string, bool, error) {
	if s == nil || s.path == "" {
		return ThreadBinding{}, "", false, nil
	}
	keys := bindingLookupKeys(entry)
	if len(keys) == 0 {
		return ThreadBinding{}, "", false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return ThreadBinding{}, "", false, err
	}
	for _, key := range keys {
		if binding, ok := data.Bindings[key]; ok && strings.TrimSpace(binding.CodexThreadID) != "" {
			return binding, key, true, nil
		}
	}
	return ThreadBinding{}, "", false, nil
}

func (s *ThreadBindingStore) Set(entry inbound.LoggedEvent, codexThreadID string, manual bool) (ThreadBinding, []string, error) {
	if s == nil || s.path == "" {
		return ThreadBinding{}, nil, nil
	}
	codexThreadID = strings.TrimSpace(codexThreadID)
	if codexThreadID == "" {
		return ThreadBinding{}, nil, fmt.Errorf("codex thread id is required")
	}
	keys := bindingSaveKeys(entry)
	if len(keys) == 0 {
		return ThreadBinding{}, nil, fmt.Errorf("cannot bind Codex thread: missing Lark chat or thread id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return ThreadBinding{}, nil, err
	}
	now := time.Now().Format(time.RFC3339Nano)
	existing, hasExisting := existingBindingForKeys(data, keys)
	createdAt := now
	if hasExisting && existing.CreatedAt != "" {
		createdAt = existing.CreatedAt
	}

	var first ThreadBinding
	for i, key := range keys {
		binding := ThreadBinding{
			LarkKey:         key,
			ChatID:          entry.ChatID,
			ThreadID:        entry.ThreadID,
			RootID:          effectiveRootID(entry),
			CodexThreadID:   codexThreadID,
			Summary:         existing.Summary,
			LastUserMessage: existing.LastUserMessage,
			LastResult:      existing.LastResult,
			LastActivityAt:  existing.LastActivityAt,
			Manual:          manual,
			CreatedAt:       createdAt,
			UpdatedAt:       now,
		}
		data.Bindings[key] = binding
		if i == 0 {
			first = binding
		}
	}

	if err := s.saveLocked(data); err != nil {
		return ThreadBinding{}, nil, err
	}
	return first, keys, nil
}

func (s *ThreadBindingStore) UpdateActivity(entry inbound.LoggedEvent, codexThreadID, userMessage, result string) (ThreadBinding, []string, error) {
	if s == nil || s.path == "" {
		return ThreadBinding{}, nil, nil
	}
	codexThreadID = strings.TrimSpace(codexThreadID)
	if codexThreadID == "" {
		return ThreadBinding{}, nil, fmt.Errorf("codex thread id is required")
	}
	keys := bindingSaveKeys(entry)
	if len(keys) == 0 {
		return ThreadBinding{}, nil, fmt.Errorf("cannot update Codex thread summary: missing Lark chat or thread id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return ThreadBinding{}, nil, err
	}

	now := time.Now().Format(time.RFC3339Nano)
	existing, hasExisting := existingBindingForKeys(data, keys)
	createdAt := now
	if hasExisting && existing.CreatedAt != "" {
		createdAt = existing.CreatedAt
	}
	manual := existing.Manual
	summary := buildThreadSummary(userMessage, result)
	lastUserMessage := previewText(userMessage, 240)
	lastResult := previewText(result, 320)

	var first ThreadBinding
	for i, key := range keys {
		binding := ThreadBinding{
			LarkKey:         key,
			ChatID:          entry.ChatID,
			ThreadID:        entry.ThreadID,
			RootID:          effectiveRootID(entry),
			CodexThreadID:   codexThreadID,
			Summary:         summary,
			LastUserMessage: lastUserMessage,
			LastResult:      lastResult,
			LastActivityAt:  now,
			Manual:          manual,
			CreatedAt:       createdAt,
			UpdatedAt:       now,
		}
		data.Bindings[key] = binding
		if i == 0 {
			first = binding
		}
	}

	if err := s.saveLocked(data); err != nil {
		return ThreadBinding{}, nil, err
	}
	return first, keys, nil
}

func (s *ThreadBindingStore) Delete(entry inbound.LoggedEvent) ([]string, bool, error) {
	if s == nil || s.path == "" {
		return nil, false, nil
	}
	keys := bindingSaveKeys(entry)
	if len(keys) == 0 {
		return nil, false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return nil, false, err
	}
	deleted := false
	for _, key := range keys {
		if _, ok := data.Bindings[key]; ok {
			delete(data.Bindings, key)
			deleted = true
		}
	}
	if err := s.saveLocked(data); err != nil {
		return nil, false, err
	}
	return keys, deleted, nil
}

func (s *ThreadBindingStore) loadLocked() (threadBindingFile, error) {
	data := threadBindingFile{
		Version:  1,
		Bindings: map[string]ThreadBinding{},
	}
	if s.path == "" {
		return data, nil
	}
	payload, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return data, nil
		}
		return data, fmt.Errorf("read Codex thread bindings: %w", err)
	}
	if len(payload) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return data, fmt.Errorf("parse Codex thread bindings: %w", err)
	}
	if data.Version == 0 {
		data.Version = 1
	}
	if data.Bindings == nil {
		data.Bindings = map[string]ThreadBinding{}
	}
	return data, nil
}

func (s *ThreadBindingStore) saveLocked(data threadBindingFile) error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create Codex thread bindings directory: %w", err)
	}
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal Codex thread bindings: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(payload, '\n'), 0600); err != nil {
		return fmt.Errorf("write Codex thread bindings: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace Codex thread bindings: %w", err)
	}
	return nil
}

func existingBindingForKeys(data threadBindingFile, keys []string) (ThreadBinding, bool) {
	for _, key := range keys {
		if binding, ok := data.Bindings[key]; ok {
			return binding, true
		}
	}
	return ThreadBinding{}, false
}

func bindingLookupKeys(entry inbound.LoggedEvent) []string {
	keys := []string{}
	if strings.TrimSpace(entry.ThreadID) != "" {
		keys = append(keys, "thread:"+strings.TrimSpace(entry.ThreadID))
	}
	if rootID := effectiveRootID(entry); rootID != "" {
		keys = append(keys, "root:"+rootID)
	}
	if strings.TrimSpace(entry.ChatID) != "" {
		keys = append(keys, "chat:"+strings.TrimSpace(entry.ChatID))
	}
	return uniqueStrings(keys)
}

func bindingSaveKeys(entry inbound.LoggedEvent) []string {
	keys := bindingLookupKeys(entry)
	if strings.TrimSpace(entry.RootID) == "" && strings.TrimSpace(entry.MessageID) != "" {
		keys = append(keys, "root:"+strings.TrimSpace(entry.MessageID))
	}
	return uniqueStrings(keys)
}

func primaryBindingKey(entry inbound.LoggedEvent) string {
	keys := bindingLookupKeys(entry)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func effectiveRootID(entry inbound.LoggedEvent) string {
	if strings.TrimSpace(entry.RootID) != "" {
		return strings.TrimSpace(entry.RootID)
	}
	if strings.TrimSpace(entry.ParentID) != "" {
		return strings.TrimSpace(entry.ParentID)
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func buildThreadSummary(userMessage, result string) string {
	userMessage = previewText(userMessage, 120)
	result = previewText(result, 160)
	switch {
	case userMessage != "" && result != "":
		return fmt.Sprintf("最近在处理：%s\n处理结果：%s", userMessage, result)
	case userMessage != "":
		return "最近在处理：" + userMessage
	case result != "":
		return "最近结果：" + result
	default:
		return ""
	}
}

func previewText(text string, maxRunes int) string {
	text = collapseWhitespace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return text
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}
