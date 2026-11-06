package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMarkdownStore_AtomicWriteAndRead(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "chronolog_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	store := NewMarkdownStore(tempDir)
	email := "alice@example.com"
	loc, _ := time.LoadLocation("Asia/Shanghai")

	rawContent := "今天完成了微服务架构方案初稿，明天上午十点复盘。"
	note, err := store.SaveNoteRaw(email, rawContent, loc)
	if err != nil {
		t.Fatalf("SaveNoteRaw 失败: %v", err)
	}

	if note.ID == "" {
		t.Fatal("生成的 Note ID 不能为空")
	}
	if note.RawContent != rawContent {
		t.Fatalf("期望原稿 %q, 得到 %q", rawContent, note.RawContent)
	}
	if note.ProcessingStatus != "processing" {
		t.Fatalf("初始状态应为 processing, 得到 %s", note.ProcessingStatus)
	}

	// 验证磁盘文件真实存在
	notePath := filepath.Join(tempDir, "users", email, "notes", note.ID+".md")
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("读取磁盘 Markdown 文件失败: %v", err)
	}

	// 重新解析读取
	readNote, err := ParseNoteContent(data)
	if err != nil {
		t.Fatalf("ParseNoteContent 失败: %v", err)
	}

	if readNote.RawContent != rawContent {
		t.Fatalf("重新解析原稿不一致: %q != %q", readNote.RawContent, rawContent)
	}

	// 验证修改与版本冲突
	readNote.PolishedContent = "### 架构方案初稿完成\n- 明天 10:00 复盘"
	readNote.ProcessingStatus = "ready"
	err = store.UpdateNoteWithRevision(email, readNote, "invalid-timestamp", loc)
	if err != ErrRevisionConflict {
		t.Fatalf("预期版本冲突错误，得到: %v", err)
	}

	// 传入正确的 UpdatedAt 进行更新
	err = store.UpdateNoteWithRevision(email, readNote, readNote.UpdatedAt, loc)
	if err != nil {
		t.Fatalf("正确版本更新失败: %v", err)
	}

	updatedNote, err := store.ReadNote(email, note.ID)
	if err != nil {
		t.Fatalf("读取更新后笔记失败: %v", err)
	}
	if updatedNote.PolishedContent != readNote.PolishedContent {
		t.Fatalf("润色正文更新未落盘")
	}
}

func TestSanitizeEmail(t *testing.T) {
	valid, err := SanitizeEmail("user@example.com")
	if err != nil || valid != "user@example.com" {
		t.Fatalf("合法邮箱被误判: %v, %s", err, valid)
	}

	// 测试路径穿越注入
	badEmails := []string{
		"../hack@example.com",
		"..\\hack@example.com",
		"user/sub@example.com",
		"user\\sub@example.com",
		"",
	}
	for _, bad := range badEmails {
		_, err := SanitizeEmail(bad)
		if err == nil {
			t.Fatalf("恶意或无效邮箱未被拦截: %s", bad)
		}
	}
}

func TestValidateNoteID_And_PathTraversal(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "chronolog_traversal_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	store := NewMarkdownStore(tempDir)
	email := "alice@example.com"

	// 恶意 Note ID 列表（包含 Windows 反斜杠、Linux 正斜杠、以及特殊符号）
	maliciousNoteIDs := []string{
		`..\..\bob@example.com\notes\secret`,
		`../../bob@example.com/notes/secret`,
		`..\hack`,
		`../hack`,
		`sub/dir`,
		`sub\dir`,
		`note;rm -rf`,
		`note id with space`,
		``,
		`.`,
		`..`,
	}

	for _, badID := range maliciousNoteIDs {
		// 1. 验证 ValidateNoteID 直接拦截
		if err := ValidateNoteID(badID); err == nil {
			t.Errorf("ValidateNoteID 应拦截恶意 ID %q 但未拦截", badID)
		}

		// 2. 验证 ReadNote 拦截
		if _, err := store.ReadNote(email, badID); err == nil {
			t.Errorf("ReadNote 应拦截恶意 ID %q 但未拦截", badID)
		}

		// 3. 验证 DeleteNote 拦截
		if err := store.DeleteNote(email, badID); err == nil {
			t.Errorf("DeleteNote 应拦截恶意 ID %q 但未拦截", badID)
		}
	}

	// 验证合法 Note ID 允许通过
	validNoteIDs := []string{
		"20261106_120000_abcd",
		"note-1234_OK",
		"normalNote",
	}
	for _, validID := range validNoteIDs {
		if err := ValidateNoteID(validID); err != nil {
			t.Errorf("ValidateNoteID 误拦截合法 ID %q: %v", validID, err)
		}
	}
}

func TestMarkdownStore_PreserveExactWhitespace(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "chronolog_whitespace_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	store := NewMarkdownStore(tempDir)
	email := "whitespace@example.com"
	loc, _ := time.LoadLocation("Asia/Shanghai")

	rawWithSpaces := "   \n\tdef hello():\n\t    print('world')\n   \n"
	note, err := store.SaveNoteRaw(email, rawWithSpaces, loc)
	if err != nil {
		t.Fatalf("SaveNoteRaw 失败: %v", err)
	}

	if note.RawContent != rawWithSpaces {
		t.Fatalf("原稿空白未得到严格保真: 期望 %q, 得到 %q", rawWithSpaces, note.RawContent)
	}

	readNote, err := store.ReadNote(email, note.ID)
	if err != nil {
		t.Fatalf("ReadNote 失败: %v", err)
	}

	if readNote.RawContent != rawWithSpaces {
		t.Fatalf("读取落盘后的原稿空白未严格保真: 期望 %q, 得到 %q", rawWithSpaces, readNote.RawContent)
	}
}
