package chatdump

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDump 用给定环境变量重新初始化采集配置，返回主目录与观察目录。
func setupDump(t *testing.T, env map[string]string) (string, string) {
	t.Helper()
	base := t.TempDir()
	mainDir := filepath.Join(base, "chatdump")
	t.Setenv("CHAT_DUMP_DIR", mainDir)
	t.Setenv("CHAT_DUMP_MIN_FREE_MB", "0")
	for k, v := range env {
		t.Setenv(k, v)
	}
	once = sync.Once{}
	dumpAll = false
	dumpModelKeys = nil
	dumpExcludeUserIDs = map[int]struct{}{}
	watchUserIDs = map[int]struct{}{}
	watchDir = ""
	lastPruneDay = ""
	initConfig()
	return mainDir, watchDir
}

func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func dumpOnce(model string, userID int) *Session {
	s := NewSessionForUser(model, userID)
	if s.Enabled() {
		s.SetMeta(map[string]any{"user_id": userID})
		s.SetRequest([]byte(`{"model":"x"}`))
		s.Flush()
	}
	return s
}

// waitForFiles 等 Flush 的异步写盘落地。
func waitForFiles(t *testing.T, root string, want int) {
	t.Helper()
	require.Eventually(t, func() bool { return countFiles(t, root) >= want }, 2*time.Second, 10*time.Millisecond)
}

func TestWatchedUserGoesToWatchDirOnly(t *testing.T) {
	mainDir, wDir := setupDump(t, map[string]string{
		"CHAT_DUMP_MODEL_KEYS":       "opus",
		"CHAT_DUMP_EXCLUDE_USER_IDS": "16,1",
		"CHAT_DUMP_WATCH_USER_IDS":   "16",
	})

	dumpOnce("gpt-6-sol", 16) // 非 opus 模型也要抓
	dumpOnce("claude-opus-5-5", 16)
	waitForFiles(t, wDir, 2)

	assert.Zero(t, countFiles(t, mainDir), "观察用户不应写进主目录")
}

func TestOtherUsersUnchanged(t *testing.T) {
	mainDir, wDir := setupDump(t, map[string]string{
		"CHAT_DUMP_MODEL_KEYS":       "opus",
		"CHAT_DUMP_EXCLUDE_USER_IDS": "16,1",
		"CHAT_DUMP_WATCH_USER_IDS":   "16",
	})

	assert.False(t, dumpOnce("claude-opus-5-5", 1).Enabled(), "被排除且不在观察名单的用户不应抓取")
	assert.False(t, dumpOnce("gpt-6-sol", 7).Enabled(), "普通用户的非关键字模型不应抓取")
	dumpOnce("claude-opus-5-5", 7)
	waitForFiles(t, mainDir, 1)

	assert.Zero(t, countFiles(t, wDir), "普通用户不应写进观察目录")
}

func TestWatchWorksWhenMainDumpDisabled(t *testing.T) {
	_, wDir := setupDump(t, map[string]string{
		"CHAT_DUMP_ENABLED":        "false",
		"CHAT_DUMP_WATCH_USER_IDS": "16",
	})

	dumpOnce("claude-opus-5-5", 16)
	waitForFiles(t, wDir, 1)
	assert.False(t, dumpOnce("claude-opus-5-5", 7).Enabled(), "主采集关闭时普通用户不应抓取")
}

func TestWatchDirInsideMainDirIsRejected(t *testing.T) {
	base := t.TempDir()
	setupDump(t, map[string]string{
		"CHAT_DUMP_DIR":            filepath.Join(base, "chatdump"),
		"CHAT_DUMP_WATCH_DIR":      filepath.Join(base, "chatdump", "watch"),
		"CHAT_DUMP_WATCH_USER_IDS": "16",
	})

	assert.False(t, NewSessionForUser("gpt-6-sol", 16).Enabled(), "观察目录在主目录内时应关闭观察")
}

func TestPruneWatchDirKeepsRecentDays(t *testing.T) {
	_, wDir := setupDump(t, map[string]string{
		"CHAT_DUMP_WATCH_USER_IDS":  "16",
		"CHAT_DUMP_WATCH_KEEP_DAYS": "7",
	})
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.Local)
	for _, d := range []string{"2026-10-01", "2026-10-08", "2026-10-09", "2026-10-16", "notes"} {
		require.NoError(t, os.MkdirAll(filepath.Join(wDir, d), 0o755))
	}

	pruneWatchDir(now)

	for d, want := range map[string]bool{
		"2026-10-01": false, "2026-10-08": false,
		"2026-10-09": true, "2026-10-16": true, "notes": true,
	} {
		_, err := os.Stat(filepath.Join(wDir, d))
		assert.Equal(t, want, err == nil, d)
	}
}
