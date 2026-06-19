package agent

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexSessionCatalogRecentBuildsHumanReadableSummaries(t *testing.T) {
	home := t.TempDir()
	sessionDir := filepath.Join(home, "sessions", "2026", "06", "18")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	threadID := "019edb08-61df-7a41-b685-69c2fc2fa302"
	path := filepath.Join(sessionDir, "rollout-2026-06-18T22-00-20-"+threadID+".jsonl")
	content := strings.Join([]string{
		`{"timestamp":"2026-06-18T14:00:00Z","type":"session_meta","payload":{"id":"` + threadID + `","cwd":"/Users/me/WorkSpace/lark-cli-codex-app"}}`,
		`{"timestamp":"2026-06-18T14:01:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"你是一个本地 Codex 执行代理\n\n用户消息：\n#status"}]}}`,
		`{"timestamp":"2026-06-18T14:02:00Z","type":"event_msg","payload":{"type":"agent_message","message":"当前飞书话题还没有连接到 Codex 会话。","phase":"final_answer"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "session_index.jsonl"), []byte(`{"id":"`+threadID+`","thread_name":"Lark bridge status","updated_at":"2026-06-18T14:03:00Z"}`+"\n"), 0600); err != nil {
		t.Fatalf("WriteFile index returned error: %v", err)
	}

	sessions, err := (codexSessionCatalog{home: home}).Recent(5)
	if err != nil {
		t.Fatalf("Recent returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	session := sessions[0]
	if session.ID != threadID {
		t.Fatalf("ID = %q", session.ID)
	}
	if session.Title != "Lark bridge status" {
		t.Fatalf("Title = %q", session.Title)
	}
	if !strings.Contains(session.Summary, "#status") || !strings.Contains(session.Summary, "当前飞书话题") {
		t.Fatalf("Summary = %q", session.Summary)
	}
	if session.CWD != "/Users/me/WorkSpace/lark-cli-codex-app" {
		t.Fatalf("CWD = %q", session.CWD)
	}
	if !session.UpdatedAtTime.Equal(time.Date(2026, 6, 18, 14, 3, 0, 0, time.UTC)) {
		t.Fatalf("UpdatedAtTime = %s", session.UpdatedAtTime)
	}
}

func TestCodexSessionCatalogResolveOrdinal(t *testing.T) {
	home := t.TempDir()
	sessionDir := filepath.Join(home, "sessions", "2026", "06", "18")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	firstID := "019edb08-61df-7a41-b685-69c2fc2fa302"
	secondID := "019edb09-61df-7a41-b685-69c2fc2fa302"
	writeTestSession(t, sessionDir, firstID, "2026-06-18T14:00:00Z", "第一个任务")
	writeTestSession(t, sessionDir, secondID, "2026-06-18T15:00:00Z", "第二个任务")

	session, ok, err := (codexSessionCatalog{home: home}).ResolveOrdinal("1", 5)
	if err != nil {
		t.Fatalf("ResolveOrdinal returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected ordinal to resolve")
	}
	if session.ID != secondID {
		t.Fatalf("ID = %q, want newest session %q", session.ID, secondID)
	}
}

func TestCodexSessionCatalogPrefersAppThreadsAndFiltersNoise(t *testing.T) {
	home := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(home, "state_5.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`
		create table threads (
			id text primary key,
			rollout_path text not null default '',
			created_at integer not null,
			updated_at integer not null,
			source text not null,
			model_provider text not null default '',
			cwd text not null,
			title text not null,
			sandbox_policy text not null default '',
			approval_mode text not null default '',
			tokens_used integer not null default 0,
			has_user_event integer not null default 0,
			archived integer not null default 0,
			archived_at integer,
			git_sha text,
			git_branch text,
			git_origin_url text,
			cli_version text not null default '',
			first_user_message text not null default '',
			agent_nickname text,
			agent_role text,
			memory_mode text not null default 'enabled',
			model text,
			reasoning_effort text,
			agent_path text,
			created_at_ms integer,
			updated_at_ms integer,
			thread_source text,
			preview text not null default ''
		)
	`)
	if err != nil {
		t.Fatalf("create table returned error: %v", err)
	}

	insertThread := func(id, title, preview, source, threadSource string, updated int64) {
		t.Helper()
		_, err := db.Exec(`
			insert into threads (id, created_at, updated_at, source, cwd, title, first_user_message, preview, thread_source, created_at_ms, updated_at_ms)
			values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, updated-100, updated, source, "/Users/me/WorkSpace/lark-cli-codex-app", title, preview, preview, nullString(threadSource), (updated-100)*1000, updated*1000)
		if err != nil {
			t.Fatalf("insert thread returned error: %v", err)
		}
	}

	insertThread("019ed9b7-b6d6-7e53-81b3-ce56253a3080", "检查未提交代码", "这个项目有未提交代码吗？", "vscode", "user", 1781794900)
	insertThread("019edaca-bc88-7e63-ade1-b15595183d9d", "The following is the Codex agent history whose request action you are assessing.", "The following is the Codex agent history", `{"subagent":{"other":"guardian"}}`, "subagent", 1781795000)
	insertThread("019edb08-61df-7a41-b685-69c2fc2fa302", "你是一个本地 Codex 执行代理，这次任务来自飞书聊天消息。", "#status", "vscode", "", 1781795100)
	insertThread("019ed9eb-d096-7ea3-b1b3-62144d2d21be", "只回复 pong，不要做其他事。", "", "exec", "", 1781795200)

	sessions, err := (codexSessionCatalog{home: home}).Recent(5)
	if err != nil {
		t.Fatalf("Recent returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want only user App thread: %#v", len(sessions), sessions)
	}
	if sessions[0].Title != "检查未提交代码" {
		t.Fatalf("Title = %q", sessions[0].Title)
	}
	if !strings.Contains(sessions[0].Summary, "这个项目有未提交代码吗") {
		t.Fatalf("Summary = %q", sessions[0].Summary)
	}
}

func writeTestSession(t *testing.T, dir, id, timestamp, userText string) {
	t.Helper()
	path := filepath.Join(dir, "rollout-2026-06-18T22-00-20-"+id+".jsonl")
	content := `{"timestamp":"` + timestamp + `","type":"session_meta","payload":{"id":"` + id + `","cwd":"/tmp/project"}}` + "\n" +
		`{"timestamp":"` + timestamp + `","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + userText + `"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
