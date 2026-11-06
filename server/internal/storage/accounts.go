package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrUserExists   = errors.New("用户已存在")
	ErrUserNotFound = errors.New("用户不存在")
)

type User struct {
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	NtfyTopic    string    `json:"ntfy_topic"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AccountStore struct {
	db *sql.DB
}

// NewAccountStore 初始化账号持久化库
func NewAccountStore(dbPath string) (*AccountStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("创建账号库目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开账号数据库失败: %w", err)
	}

	// 开启 WAL 模式提高并发读写性能
	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("配置 accounts.db WAL 模式失败: %w", err)
	}

	initSQL := `
	CREATE TABLE IF NOT EXISTS users (
		email TEXT PRIMARY KEY COLLATE NOCASE,
		password_hash TEXT NOT NULL,
		ntfy_topic TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	`
	if _, err := db.Exec(initSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化 users 表失败: %w", err)
	}

	return &AccountStore{db: db}, nil
}

// Close 关闭数据库
func (s *AccountStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// CreateUser 创建新用户
func (s *AccountStore) CreateUser(email, passwordHash, ntfyTopic string) (*User, error) {
	now := time.Now()
	nowStr := now.Format(time.RFC3339)

	stmt := `INSERT INTO users (email, password_hash, ntfy_topic, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.Exec(stmt, email, passwordHash, ntfyTopic, nowStr, nowStr)
	if err != nil {
		// 检查是否主键冲突
		if existing, _ := s.GetUserByEmail(email); existing != nil {
			return nil, ErrUserExists
		}
		return nil, err
	}

	return &User{
		Email:        email,
		PasswordHash: passwordHash,
		NtfyTopic:    ntfyTopic,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// GetUserByEmail 查询用户
func (s *AccountStore) GetUserByEmail(email string) (*User, error) {
	query := `SELECT email, password_hash, ntfy_topic, created_at, updated_at FROM users WHERE email = ?`
	row := s.db.QueryRow(query, email)

	var u User
	var createdStr, updatedStr string
	err := row.Scan(&u.Email, &u.PasswordHash, &u.NtfyTopic, &createdStr, &updatedStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	u.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &u, nil
}

// UpdateUserTopic 更新用户的 Ntfy 主题
func (s *AccountStore) UpdateUserTopic(email, topic string) error {
	nowStr := time.Now().Format(time.RFC3339)
	stmt := `UPDATE users SET ntfy_topic = ?, updated_at = ? WHERE email = ?`
	res, err := s.db.Exec(stmt, topic, nowStr, email)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}
