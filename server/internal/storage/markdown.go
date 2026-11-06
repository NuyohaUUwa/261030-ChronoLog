package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	ErrNoteNotFound     = errors.New("笔记不存在")
	ErrRevisionConflict = errors.New("数据已被修改，版本冲突 (409)")
	ErrInvalidEmail     = errors.New("无效的用户邮箱路径")
)

type Reminder struct {
	ID                 string `yaml:"id" json:"id"`
	Title              string `yaml:"title" json:"title"`
	RemindAt           string `yaml:"remind_at" json:"remind_at"` // 带时区偏移 RFC 3339
	Status             string `yaml:"status" json:"status"`       // pending | completed | cancelled
	NotificationStatus string `yaml:"notification_status" json:"notification_status"` // pending | sent | failed
	Reason             string `yaml:"reason,omitempty" json:"reason,omitempty"`
}

type NoteFrontMatter struct {
	ID               string     `yaml:"id" json:"id"`
	CreatedAt        string     `yaml:"created_at" json:"created_at"`
	UpdatedAt        string     `yaml:"updated_at" json:"updated_at"`
	RawContent       string     `yaml:"raw_content" json:"raw_content"`
	ProcessingStatus string     `yaml:"processing_status" json:"processing_status"` // processing | ready | failed
	FailureReason    string     `yaml:"failure_reason,omitempty" json:"failure_reason,omitempty"`
	Tags             []string   `yaml:"tags" json:"tags"`
	Reminders        []Reminder `yaml:"reminders" json:"reminders"`
}

type Note struct {
	NoteFrontMatter `yaml:",inline"`
	PolishedContent string `yaml:"-" json:"polished_content"`
}

type MarkdownStore struct {
	baseDataDir string
	noteLocks   sync.Map // key: email/noteID -> *sync.Mutex
}

func NewMarkdownStore(baseDataDir string) *MarkdownStore {
	return &MarkdownStore{
		baseDataDir: baseDataDir,
	}
}

func (s *MarkdownStore) getNoteLock(email, noteID string) *sync.Mutex {
	key := fmt.Sprintf("%s/%s", email, noteID)
	val, _ := s.noteLocks.LoadOrStore(key, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// SanitizeEmail 确保邮箱仅作为合规的单一目录名，杜绝路径穿越
func SanitizeEmail(email string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(email))
	if trimmed == "" || strings.Contains(trimmed, "..") || strings.ContainsAny(trimmed, "/\\:*?\"<>|") {
		return "", ErrInvalidEmail
	}
	return trimmed, nil
}

// GetUserNotesDir 获取用户的 notes 根目录
func (s *MarkdownStore) GetUserNotesDir(email string) (string, error) {
	safeEmail, err := SanitizeEmail(email)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.baseDataDir, "users", safeEmail, "notes"), nil
}

// GenerateNoteID 生成格式为 YYYYMMDD_HHMMSS_xxxx 的唯一笔记 ID
func GenerateNoteID(t time.Time) string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	randSuffix := hex.EncodeToString(b)
	return fmt.Sprintf("%s_%s", t.Format("20060102_150405"), randSuffix)
}

// SaveNoteRaw 创建并原子落盘初始随手记（保存权威原稿）
func (s *MarkdownStore) SaveNoteRaw(email, rawContent string, loc *time.Location) (*Note, error) {
	now := time.Now().In(loc)
	id := GenerateNoteID(now)
	timeStr := now.Format(time.RFC3339)

	note := &Note{
		NoteFrontMatter: NoteFrontMatter{
			ID:               id,
			CreatedAt:        timeStr,
			UpdatedAt:        timeStr,
			RawContent:       rawContent,
			ProcessingStatus: "processing",
			Tags:             []string{},
			Reminders:        []Reminder{},
		},
		PolishedContent: "",
	}

	if err := s.WriteNoteAtomic(email, note); err != nil {
		return nil, err
	}
	return note, nil
}

// WriteNoteAtomic 原子替换写入 Note Markdown 文件
func (s *MarkdownStore) WriteNoteAtomic(email string, note *Note) error {
	notesDir, err := s.GetUserNotesDir(email)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		return fmt.Errorf("创建笔记目录失败: %w", err)
	}

	mu := s.getNoteLock(email, note.ID)
	mu.Lock()
	defer mu.Unlock()

	// 序列化 YAML FrontMatter
	fmBytes, err := yaml.Marshal(&note.NoteFrontMatter)
	if err != nil {
		return fmt.Errorf("序列化 FrontMatter 失败: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fmBytes)
	buf.WriteString("---\n\n")
	buf.WriteString(strings.TrimSpace(note.PolishedContent))
	buf.WriteString("\n")

	targetPath := filepath.Join(notesDir, note.ID+".md")
	tmpPath := filepath.Join(notesDir, fmt.Sprintf(".%s.tmp", note.ID))

	// 写入临时文件并刷盘
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("创建临时笔记文件失败: %w", err)
	}

	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("写入临时笔记文件失败: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("刷盘失败: %w", err)
	}
	f.Close()

	// 原子替换
	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("原子替换笔记文件失败: %w", err)
	}

	return nil
}

// ReadNote 读取并解析单个 Markdown 笔记
func (s *MarkdownStore) ReadNote(email, noteID string) (*Note, error) {
	notesDir, err := s.GetUserNotesDir(email)
	if err != nil {
		return nil, err
	}

	targetPath := filepath.Join(notesDir, noteID+".md")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}

	return ParseNoteContent(data)
}

// ParseNoteContent 将 Markdown 文件内容解析为 Note 对象
func ParseNoteContent(data []byte) (*Note, error) {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("非法笔记格式: 缺少 YAML FrontMatter 起始标记")
	}

	// 查找第二个 ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return nil, fmt.Errorf("非法笔记格式: 缺少 YAML FrontMatter 结束标记")
	}

	yamlStr := rest[:idx]
	bodyStr := ""
	endOfFm := idx + 4 // 跳过 \n---
	if len(rest) > endOfFm {
		bodyStr = strings.TrimSpace(rest[endOfFm:])
	}

	var note Note
	if err := yaml.Unmarshal([]byte(yamlStr), &note.NoteFrontMatter); err != nil {
		return nil, fmt.Errorf("解析 FrontMatter YAML 失败: %w", err)
	}
	note.PolishedContent = bodyStr
	if note.Tags == nil {
		note.Tags = []string{}
	}
	if note.Reminders == nil {
		note.Reminders = []Reminder{}
	}

	return &note, nil
}

// UpdateNoteWithRevision 带版本冲突校验的更新
func (s *MarkdownStore) UpdateNoteWithRevision(email string, note *Note, expectedUpdatedAt string, loc *time.Location) error {
	existing, err := s.ReadNote(email, note.ID)
	if err != nil {
		return err
	}

	if expectedUpdatedAt != "" && existing.UpdatedAt != expectedUpdatedAt {
		return ErrRevisionConflict
	}

	note.UpdatedAt = time.Now().In(loc).Format(time.RFC3339)
	return s.WriteNoteAtomic(email, note)
}

// DeleteNote 删除笔记
func (s *MarkdownStore) DeleteNote(email, noteID string) error {
	notesDir, err := s.GetUserNotesDir(email)
	if err != nil {
		return err
	}

	mu := s.getNoteLock(email, noteID)
	mu.Lock()
	defer mu.Unlock()

	targetPath := filepath.Join(notesDir, noteID+".md")
	if err := os.Remove(targetPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNoteNotFound
		}
		return err
	}
	return nil
}

// ListUserDirs 扫描 users 根目录下的所有合法用户邮箱目录
func ListUserDirs(usersBaseDir string) ([]string, error) {
	if _, err := os.Stat(usersBaseDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(usersBaseDir)
	if err != nil {
		return nil, err
	}

	var users []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			users = append(users, entry.Name())
		}
	}
	return users, nil
}

func (s *MarkdownStore) ListAllNotes(email string) ([]*Note, error) {
	notesDir, err := s.GetUserNotesDir(email)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(notesDir); os.IsNotExist(err) {
		return []*Note{}, nil
	}

	entries, err := os.ReadDir(notesDir)
	if err != nil {
		return nil, err
	}

	var notes []*Note
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		noteID := strings.TrimSuffix(entry.Name(), ".md")
		note, err := s.ReadNote(email, noteID)
		if err == nil && note != nil {
			notes = append(notes, note)
		}
	}

	return notes, nil
}
