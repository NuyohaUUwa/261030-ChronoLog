package storage_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"chronolog/server/internal/auth"
	"chronolog/server/internal/storage"
)

// copyDir 递归复制目录用于模拟全量备份与异地恢复
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// TestSyntheticDataBackupAndRestore 实证证据：合成数据备份与恢复全流程 (PT-05 / S05)
func TestSyntheticDataBackupAndRestore(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)

	// 1. 初始化原始数据目录 Dir A
	sourceDir, err := os.MkdirTemp("", "chronolog-synthetic-source-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sourceDir)

	accountDBPath := filepath.Join(sourceDir, "accounts.db")
	accountStore, err := storage.NewAccountStore(accountDBPath)
	if err != nil {
		t.Fatalf("初始化账号库失败: %v", err)
	}

	email := "synthetic_user@test.local"
	plainPassword := "TestPass123!"
	pwdHash, err := auth.HashPassword(plainPassword)
	if err != nil {
		t.Fatal(err)
	}

	_, err = accountStore.CreateUser(email, pwdHash, "topic_synthetic_123")
	if err != nil {
		t.Fatalf("创建合成用户失败: %v", err)
	}

	mdStore := storage.NewMarkdownStore(sourceDir)
	indexMgr := storage.NewIndexManager(sourceDir)

	// 写入多条合成笔记（包含不同标签与待办）
	note1, err := mdStore.SaveNoteRaw(email, "合成测试笔记1：明天上午10点参加技术方案评审", loc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = mdStore.MutateNoteAtomic(email, note1.ID, func(n *storage.Note) error {
		n.PolishedContent = "### 技术方案评审\n明天上午 10:00 参加技术方案评审。"
		n.Tags = []string{"工作/技术", "重要/待办"}
		n.Reminders = []storage.Reminder{
			{
				ID:                 "rem_synth_1",
				Title:              "技术方案评审",
				RemindAt:           "2026-11-08T10:00:00+08:00",
				Status:             "pending",
				NotificationStatus: "pending",
			},
		}
		return nil
	}, loc)
	if err != nil {
		t.Fatal(err)
	}

	note2, err := mdStore.SaveNoteRaw(email, "合成测试笔记2：购买日常耗材与咖啡豆", loc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = mdStore.MutateNoteAtomic(email, note2.ID, func(n *storage.Note) error {
		n.PolishedContent = "日常采购清单：咖啡豆与滤纸。"
		n.Tags = []string{"生活/日常"}
		return nil
	}, loc)
	if err != nil {
		t.Fatal(err)
	}

	// 建立原始索引并验证可查
	allNotes, _ := mdStore.ListAllNotes(email)
	origIdx, err := indexMgr.GetUserIndex(email)
	if err != nil {
		t.Fatal(err)
	}
	if err := origIdx.RebuildIndex(allNotes, loc); err != nil {
		t.Fatalf("重建索引失败: %v", err)
	}

	// 安全关闭数据库以确保 WAL 写入落盘
	indexMgr.Close()
	accountStore.Close()

	// 2. 模拟全量数据备份
	backupDir, err := os.MkdirTemp("", "chronolog-synthetic-backup-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(backupDir)

	if err := copyDir(sourceDir, backupDir); err != nil {
		t.Fatalf("全量备份失败: %v", err)
	}

	// 3. 模拟在新环境/新数据目录恢复数据
	restoredDir, err := os.MkdirTemp("", "chronolog-synthetic-restored-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(restoredDir)

	if err := copyDir(backupDir, restoredDir); err != nil {
		t.Fatalf("恢复数据到新目录失败: %v", err)
	}

	// 4. 在恢复后的目录重新启动验证
	// 4.1 验证账号库与密码凭据
	restoredAccountStore, err := storage.NewAccountStore(filepath.Join(restoredDir, "accounts.db"))
	if err != nil {
		t.Fatalf("恢复后打开账号库失败: %v", err)
	}
	defer restoredAccountStore.Close()

	restoredUser, err := restoredAccountStore.GetUserByEmail(email)
	if err != nil {
		t.Fatalf("恢复后读取用户失败: %v", err)
	}
	if !auth.CheckPassword(plainPassword, restoredUser.PasswordHash) {
		t.Fatalf("恢复后用户密码凭据验证失败")
	}
	if restoredUser.NtfyTopic != "topic_synthetic_123" {
		t.Fatalf("恢复后用户 Topic 不符: %s", restoredUser.NtfyTopic)
	}

	// 4.2 验证 Markdown 权威原稿数据完整性
	restoredMdStore := storage.NewMarkdownStore(restoredDir)
	restoredNotes, err := restoredMdStore.ListAllNotes(email)
	if err != nil {
		t.Fatalf("恢复后读取笔记列表失败: %v", err)
	}
	if len(restoredNotes) != 2 {
		t.Fatalf("恢复后笔记数量不符: 期望 2, 实际 %d", len(restoredNotes))
	}

	// 4.3 模拟极端冷启动：即使索引丢失/损坏，也能从权威 Markdown 重建并精准检索
	_ = os.Remove(filepath.Join(restoredDir, "users", email, "index.db"))
	restoredIndexMgr := storage.NewIndexManager(restoredDir)
	defer restoredIndexMgr.Close()

	restoredIdx, err := restoredIndexMgr.GetUserIndex(email)
	if err != nil {
		t.Fatalf("恢复后初始化索引失败: %v", err)
	}
	if err := restoredIdx.RebuildIndex(restoredNotes, loc); err != nil {
		t.Fatalf("恢复后从 Markdown 重建索引失败: %v", err)
	}

	// 4.4 验证按标签检索
	tagNotes, totalTag, err := restoredIdx.QueryNotes(storage.NoteFilter{Tag: "工作/技术", Page: 1, Limit: 10})
	if err != nil || totalTag != 1 || len(tagNotes) != 1 {
		t.Fatalf("恢复后按标签检索失败: total=%d, err=%v", totalTag, err)
	}
	if tagNotes[0].ID != note1.ID {
		t.Fatalf("检索到的笔记ID不符: %s vs %s", tagNotes[0].ID, note1.ID)
	}

	// 4.5 验证待办日历索引
	summary, err := restoredIdx.GetCalendarSummary(2026, 11, loc)
	if err != nil {
		t.Fatalf("恢复后查询日历摘要失败: %v", err)
	}
	if len(summary.ReminderDates) != 1 || summary.ReminderDates[0].Date != "2026-11-08" {
		t.Fatalf("恢复后待办提醒日期索引不符: %+v", summary.ReminderDates)
	}

	t.Logf("=== S05 本机合成数据备份与恢复实证成功通过 ===")
	t.Logf("实证结果: 账号验证成功, 2条笔记完整恢复, Markdown重建索引检索正常, 待办索引一致")
}
