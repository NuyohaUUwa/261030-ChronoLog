package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	ErrNoteNotFound     = errors.New("笔记不存在")
	ErrRevisionConflict = errors.New("数据已被修改，版本冲突 (409)")
	ErrInvalidEmail     = errors.New("无效的用户邮箱路径")
	ErrInvalidNoteID    = errors.New("无效的笔记 ID，拒绝路径穿越")
)

var noteIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)

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
	Revision         int64      `yaml:"revision" json:"revision"` // 自增版本号防并发覆写
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

// ValidateNoteID 严格校验笔记 ID，防范 Windows/Linux 路径穿越注入
func ValidateNoteID(noteID string) error {
	trimmed := strings.TrimSpace(noteID)
	if trimmed == "" || len(trimmed) > 64 || !noteIDRegex.MatchString(trimmed) || strings.Contains(trimmed, "..") || strings.ContainsAny(trimmed, "/\\:*?\"<>|") {
		return ErrInvalidNoteID
	}
	return nil
}

// GetUserNotesDir 获取用户的 notes 根目录
func (s *MarkdownStore) GetUserNotesDir(email string) (string, error) {
	safeEmail, err := SanitizeEmail(email)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.baseDataDir, "users", safeEmail, "notes"), nil
}

// GetNoteFilePath 安全解析单条笔记的绝对文件路径并校验路径隔离
func (s *MarkdownStore) GetNoteFilePath(email, noteID string) (string, error) {
	safeEmail, err := SanitizeEmail(email)
	if err != nil {
		return "", err
	}
	if err := ValidateNoteID(noteID); err != nil {
		return "", err
	}

	userNotesDir := filepath.Join(s.baseDataDir, "users", safeEmail, "notes")
	targetPath := filepath.Clean(filepath.Join(userNotesDir, noteID+".md"))

	// 严格核验计算后的相对路径不能跳出当前用户的 notes 目录
	rel, err := filepath.Rel(userNotesDir, targetPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return "", ErrInvalidNoteID
	}
	return targetPath, nil
}

// GenerateNoteID 生成格式为 YYYYMMDD_HHMMSS_xxxx 的唯一笔记 ID
func GenerateNoteID(t time.Time) string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	randSuffix := hex.EncodeToString(b)
	return fmt.Sprintf("%s_%s", t.Format("20060102_150405"), randSuffix)
}

// SaveNoteRaw 创建并原子落盘初始随手记（保存权威原稿，保留全部空白字符）
func (s *MarkdownStore) SaveNoteRaw(email, rawContent string, loc *time.Location) (*Note, error) {
	now := time.Now().In(loc)
	id := GenerateNoteID(now)
	timeStr := now.Format(time.RFC3339Nano)

	note := &Note{
		NoteFrontMatter: NoteFrontMatter{
			ID:               id,
			CreatedAt:        timeStr,
			UpdatedAt:        timeStr,
			Revision:         1,
			RawContent:       rawContent, // 无损保留原样，不截断空白
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
	targetPath, err := s.GetNoteFilePath(email, note.ID)
	if err != nil {
		return err
	}

	notesDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		return fmt.Errorf("创建笔记目录失败: %w", err)
	}

	mu := s.getNoteLock(email, note.ID)
	mu.Lock()
	defer mu.Unlock()

	return s.writeNoteAtomicUnderLock(targetPath, note)
}

func (s *MarkdownStore) writeNoteAtomicUnderLock(targetPath string, note *Note) error {
	notesDir := filepath.Dir(targetPath)

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

	tmpPath := filepath.Join(notesDir, fmt.Sprintf(".%s_%d.tmp", note.ID, time.Now().UnixNano()))

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

	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("原子替换笔记文件失败: %w", err)
	}

	return nil
}

// ReadNote 读取并解析单个 Markdown 笔记
func (s *MarkdownStore) ReadNote(email, noteID string) (*Note, error) {
	targetPath, err := s.GetNoteFilePath(email, noteID)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}

	return ParseNoteContent(data)
}

// MutateNoteAtomic 在独占锁下安全执行 读取 -> 修改 -> 原子刷盘，避免并发覆写
func (s *MarkdownStore) MutateNoteAtomic(email, noteID string, mutateFn func(note *Note) error, loc *time.Location) (*Note, error) {
	targetPath, err := s.GetNoteFilePath(email, noteID)
	if err != nil {
		return nil, err
	}

	mu := s.getNoteLock(email, noteID)
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}

	note, err := ParseNoteContent(data)
	if err != nil {
		return nil, err
	}

	if err := mutateFn(note); err != nil {
		return nil, err
	}

	note.Revision++
	now := time.Now()
	if loc != nil {
		now = now.In(loc)
	}
	note.UpdatedAt = now.Format(time.RFC3339Nano)

	if err := s.writeNoteAtomicUnderLock(targetPath, note); err != nil {
		return nil, err
	}

	return note, nil
}

// ParseNoteContent 将 Markdown 文件内容解析为 Note 对象
func ParseNoteContent(data []byte) (*Note, error) {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("非法笔记格式: 缺少 YAML FrontMatter 起始标记")
	}

	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return nil, fmt.Errorf("非法笔记格式: 缺少 YAML FrontMatter 结束标记")
	}

	yamlStr := rest[:idx]
	bodyStr := ""
	endOfFm := idx + 4
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
	if note.Revision <= 0 {
		note.Revision = 1
	}

	return &note, nil
}

// UpdateNoteWithRevision 带版本冲突校验的更新
func (s *MarkdownStore) UpdateNoteWithRevision(email string, note *Note, expectedUpdatedAt string, loc *time.Location) error {
	_, err := s.MutateNoteAtomic(email, note.ID, func(existing *Note) error {
		if expectedUpdatedAt != "" && existing.UpdatedAt != expectedUpdatedAt {
			return ErrRevisionConflict
		}
		existing.PolishedContent = note.PolishedContent
		existing.Tags = note.Tags
		existing.Reminders = note.Reminders
		existing.ProcessingStatus = note.ProcessingStatus
		existing.FailureReason = note.FailureReason
		return nil
	}, loc)
	return err
}

// DeleteNote 删除笔记
func (s *MarkdownStore) DeleteNote(email, noteID string) error {
	targetPath, err := s.GetNoteFilePath(email, noteID)
	if err != nil {
		return err
	}

	mu := s.getNoteLock(email, noteID)
	mu.Lock()
	defer mu.Unlock()

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

// ListAllNotes 读取用户全部笔记（用于启动时索引同步与全量列表）
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
