package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type NoteFilter struct {
	Date    string // YYYY-MM-DD
	Tag     string
	Keyword string
	Page    int
	Limit   int
}

type CalendarReminderDate struct {
	Date       string `json:"date"`
	Count      int    `json:"count"`
	HasPending bool   `json:"has_pending"`
}

type CalendarSummary struct {
	ActiveDates   []string               `json:"active_dates"`
	ReminderDates []CalendarReminderDate `json:"reminder_dates"`
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

type UserIndex struct {
	dbPath string
	db     *sql.DB
	mu     sync.RWMutex
}

type IndexManager struct {
	mu          sync.Mutex
	baseDataDir string
	userIndices sync.Map // email -> *UserIndex
}

func NewIndexManager(baseDataDir string) *IndexManager {
	return &IndexManager{
		baseDataDir: baseDataDir,
	}
}

func (m *IndexManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userIndices.Range(func(key, value any) bool { _ = value.(*UserIndex).db.Close(); m.userIndices.Delete(key); return true })
}

// GetUserIndex 获取指定用户的 index.db 实例
func (m *IndexManager) GetUserIndex(email string) (*UserIndex, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	safeEmail, err := SanitizeEmail(email)
	if err != nil {
		return nil, err
	}

	if val, ok := m.userIndices.Load(safeEmail); ok {
		return val.(*UserIndex), nil
	}

	userDir := filepath.Join(m.baseDataDir, "users", safeEmail)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return nil, fmt.Errorf("创建用户索引目录失败: %w", err)
	}

	dbPath := filepath.Join(userDir, "index.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开索引数据库失败: %w", err)
	}

	// 启用 WAL 模式
	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置 index.db WAL 失败: %w", err)
	}

	schemaSQL := `
	CREATE TABLE IF NOT EXISTS notes_meta (
		id TEXT PRIMARY KEY,
		created_at TEXT NOT NULL,
		date_str TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		processing_status TEXT NOT NULL,
		failure_reason TEXT NOT NULL DEFAULT '',
		raw_content TEXT NOT NULL,
		polished_content TEXT NOT NULL,
		tags_json TEXT NOT NULL,
		reminders_json TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_notes_date ON notes_meta(date_str);
	CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes_meta(created_at DESC);
	CREATE TABLE IF NOT EXISTS reminder_dates (note_id TEXT NOT NULL, date_str TEXT NOT NULL, PRIMARY KEY(note_id, date_str));

	CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
		id UNINDEXED,
		raw_content,
		polished_content,
		tags,
		tokenize='trigram'
	);
	`
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化 notes_meta 表与 FTS5 失败: %w", err)
	}

	idx := &UserIndex{
		dbPath: dbPath,
		db:     db,
	}
	m.userIndices.Store(safeEmail, idx)
	return idx, nil
}

// IndexNote 同步单条笔记至 index.db
func (u *UserIndex) IndexNote(note *Note, loc *time.Location) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	parsedTime, err := time.Parse(time.RFC3339, note.CreatedAt)
	dateStr := ""
	if err == nil {
		dateStr = parsedTime.In(loc).Format("2006-01-02")
	} else {
		dateStr = time.Now().In(loc).Format("2006-01-02")
	}

	tagsJSON, _ := json.Marshal(note.Tags)
	remindersJSON, _ := json.Marshal(note.Reminders)
	tagsText := strings.Join(note.Tags, " ")

	tx, err := u.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := indexReminderDates(tx, note, loc); err != nil {
		return err
	}

	// 插入或更新元数据
	metaStmt := `
	INSERT INTO notes_meta (id, created_at, date_str, updated_at, processing_status, failure_reason, raw_content, polished_content, tags_json, reminders_json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		created_at = excluded.created_at,
		date_str = excluded.date_str,
		updated_at = excluded.updated_at,
		processing_status = excluded.processing_status,
		failure_reason = excluded.failure_reason,
		raw_content = excluded.raw_content,
		polished_content = excluded.polished_content,
		tags_json = excluded.tags_json,
		reminders_json = excluded.reminders_json;
	`
	if _, err := tx.Exec(metaStmt, note.ID, note.CreatedAt, dateStr, note.UpdatedAt, note.ProcessingStatus, note.FailureReason, note.RawContent, note.PolishedContent, string(tagsJSON), string(remindersJSON)); err != nil {
		return fmt.Errorf("写入 notes_meta 失败: %w", err)
	}

	// 更新 FTS5
	if _, err := tx.Exec(`DELETE FROM notes_fts WHERE id = ?`, note.ID); err != nil {
		return fmt.Errorf("清理 notes_fts 失败: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO notes_fts (id, raw_content, polished_content, tags) VALUES (?, ?, ?, ?)`, note.ID, note.RawContent, note.PolishedContent, tagsText); err != nil {
		return fmt.Errorf("写入 notes_fts 失败: %w", err)
	}

	return tx.Commit()
}

// DeleteNoteIndex 从 index.db 删除笔记
func (u *UserIndex) DeleteNoteIndex(noteID string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	tx, err := u.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM reminder_dates WHERE note_id = ?`, noteID); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM notes_meta WHERE id = ?`, noteID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notes_fts WHERE id = ?`, noteID); err != nil {
		return err
	}

	return tx.Commit()
}

// QueryNotes 按条件检索笔记列表
func (u *UserIndex) QueryNotes(filter NoteFilter) ([]*Note, int, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	offset := (filter.Page - 1) * filter.Limit

	var conditions []string
	var args []interface{}

	if filter.Date != "" {
		// 匹配笔记创建日期 OR 笔记中提醒的触发日期 (双模日历双向聚合)
		conditions = append(conditions, "(m.date_str = ? OR EXISTS (SELECT 1 FROM reminder_dates d WHERE d.note_id = m.id AND d.date_str = ?))")
		args = append(args, filter.Date, filter.Date)
	}

	if filter.Tag != "" {
		// 检索包含指定 tag 的记录（支持父子层级前缀匹配）
		conditions = append(conditions, "m.tags_json LIKE ?")
		args = append(args, "%"+filter.Tag+"%")
	}

	fromClause := "notes_meta m"
	kw := strings.TrimSpace(filter.Keyword)
	if kw != "" {
		runes := []rune(kw)
		if len(runes) >= 3 {
			fromClause += " JOIN notes_fts f ON m.id = f.id"
			conditions = append(conditions, "notes_fts MATCH ?")
			// 在 FTS5 中转义双引号，避免语法错误导致 500
			escapedKw := strings.ReplaceAll(kw, `"`, `""`)
			args = append(args, `"`+escapedKw+`"`)
		} else {
			// 短词（1-2字符）使用 LIKE 模糊匹配，转义特殊通配符
			escapedLike := strings.ReplaceAll(kw, "/", "//")
			escapedLike = strings.ReplaceAll(escapedLike, "%", "/%")
			escapedLike = strings.ReplaceAll(escapedLike, "_", "/_")
			conditions = append(conditions, "(m.raw_content LIKE ? ESCAPE '/' OR m.polished_content LIKE ? ESCAPE '/' OR m.tags_json LIKE ? ESCAPE '/')")
			pattern := "%" + escapedLike + "%"
			args = append(args, pattern, pattern, pattern)
		}
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// 查询总数
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s%s", fromClause, whereClause)
	var total int
	if err := u.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计笔记数量失败: %w", err)
	}

	// 查询分页数据
	dataQuery := fmt.Sprintf("SELECT m.id, m.created_at, m.updated_at, m.processing_status, m.failure_reason, m.raw_content, m.polished_content, m.tags_json, m.reminders_json FROM %s%s ORDER BY m.created_at DESC, m.id DESC LIMIT ? OFFSET ?", fromClause, whereClause)
	queryArgs := append(args, filter.Limit, offset)

	rows, err := u.db.Query(dataQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询笔记失败: %w", err)
	}
	defer rows.Close()

	var notes []*Note
	for rows.Next() {
		var n Note
		var tagsJSON, remindersJSON string
		if err := rows.Scan(&n.ID, &n.CreatedAt, &n.UpdatedAt, &n.ProcessingStatus, &n.FailureReason, &n.RawContent, &n.PolishedContent, &tagsJSON, &remindersJSON); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &n.Tags)
		_ = json.Unmarshal([]byte(remindersJSON), &n.Reminders)
		if n.Tags == nil {
			n.Tags = []string{}
		}
		if n.Reminders == nil {
			n.Reminders = []Reminder{}
		}
		notes = append(notes, &n)
	}

	return notes, total, nil
}

// GetCalendarSummary 聚合双模日历数据
func (u *UserIndex) GetCalendarSummary(year, month int, loc *time.Location) (*CalendarSummary, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	monthPrefix := fmt.Sprintf("%04d-%02d", year, month)

	// 查询有笔记的活跃日期
	queryActive := `SELECT DISTINCT date_str FROM notes_meta WHERE date_str LIKE ? ORDER BY date_str ASC`
	rows, err := u.db.Query(queryActive, monthPrefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activeDates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err == nil {
			activeDates = append(activeDates, d)
		}
	}
	if activeDates == nil {
		activeDates = []string{}
	}

	// 查询该月份具有 reminders 的分布
	// 从 notes_meta 中抽取非空 reminders_json
	queryReminders := `SELECT reminders_json FROM notes_meta WHERE reminders_json != '[]' AND reminders_json != ''`
	rRows, err := u.db.Query(queryReminders)
	if err != nil {
		return nil, err
	}
	defer rRows.Close()

	dateMap := make(map[string]*CalendarReminderDate)
	for rRows.Next() {
		var remJSON string
		if err := rRows.Scan(&remJSON); err != nil {
			continue
		}
		var rems []Reminder
		if err := json.Unmarshal([]byte(remJSON), &rems); err != nil {
			continue
		}

		for _, r := range rems {
			t, err := time.Parse(time.RFC3339, r.RemindAt)
			if err != nil {
				continue
			}
			rDateStr := t.In(loc).Format("2006-01-02")
			if strings.HasPrefix(rDateStr, monthPrefix) {
				item, exists := dateMap[rDateStr]
				if !exists {
					item = &CalendarReminderDate{
						Date:       rDateStr,
						Count:      0,
						HasPending: false,
					}
					dateMap[rDateStr] = item
				}
				item.Count++
				if r.Status == "pending" {
					item.HasPending = true
				}
			}
		}
	}

	var reminderDates []CalendarReminderDate
	for _, v := range dateMap {
		reminderDates = append(reminderDates, *v)
	}
	if reminderDates == nil {
		reminderDates = []CalendarReminderDate{}
	}

	return &CalendarSummary{
		ActiveDates:   activeDates,
		ReminderDates: reminderDates,
	}, nil
}

// GetUserTags 获取用户所有标签及出现频次
func (u *UserIndex) GetUserTags() ([]TagCount, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	rows, err := u.db.Query(`SELECT tags_json FROM notes_meta WHERE tags_json != '[]' AND tags_json != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err == nil {
			var tags []string
			if err := json.Unmarshal([]byte(tagsJSON), &tags); err == nil {
				for _, tag := range tags {
					trimmed := strings.TrimSpace(tag)
					if trimmed != "" {
						counts[trimmed]++
					}
				}
			}
		}
	}

	var res []TagCount
	for tag, count := range counts {
		res = append(res, TagCount{Tag: tag, Count: count})
	}
	return res, nil
}

// RebuildIndex 从 Markdown 列表全量重建索引
func (u *UserIndex) RebuildIndex(notes []*Note, loc *time.Location) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	tx, err := u.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM reminder_dates`); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM notes_meta`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notes_fts`); err != nil {
		return err
	}

	for _, note := range notes {
		if err := indexReminderDates(tx, note, loc); err != nil {
			return err
		}
		parsedTime, err := time.Parse(time.RFC3339, note.CreatedAt)
		dateStr := ""
		if err == nil {
			dateStr = parsedTime.In(loc).Format("2006-01-02")
		} else {
			dateStr = time.Now().In(loc).Format("2006-01-02")
		}

		tagsJSON, _ := json.Marshal(note.Tags)
		remindersJSON, _ := json.Marshal(note.Reminders)
		tagsText := strings.Join(note.Tags, " ")

		metaStmt := `
		INSERT INTO notes_meta (id, created_at, date_str, updated_at, processing_status, failure_reason, raw_content, polished_content, tags_json, reminders_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		if _, err := tx.Exec(metaStmt, note.ID, note.CreatedAt, dateStr, note.UpdatedAt, note.ProcessingStatus, note.FailureReason, note.RawContent, note.PolishedContent, string(tagsJSON), string(remindersJSON)); err != nil {
			return err
		}

		if _, err := tx.Exec(`INSERT INTO notes_fts (id, raw_content, polished_content, tags) VALUES (?, ?, ?, ?)`, note.ID, note.RawContent, note.PolishedContent, tagsText); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func indexReminderDates(tx *sql.Tx, note *Note, loc *time.Location) error {
	if _, err := tx.Exec(`DELETE FROM reminder_dates WHERE note_id = ?`, note.ID); err != nil {
		return err
	}
	for _, r := range note.Reminders {
		t, err := time.Parse(time.RFC3339, r.RemindAt)
		if err != nil {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO reminder_dates(note_id, date_str) VALUES (?, ?)`, note.ID, t.In(loc).Format("2006-01-02")); err != nil {
			return err
		}
	}
	return nil
}

// GetTotalCount 查询当前索引库中记录的总数
func (u *UserIndex) GetTotalCount() (int, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	var count int
	err := u.db.QueryRow("SELECT COUNT(*) FROM notes_meta").Scan(&count)
	return count, err
}
