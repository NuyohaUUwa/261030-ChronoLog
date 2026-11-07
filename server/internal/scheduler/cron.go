package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chronolog/server/internal/storage"
)

type NotificationSender interface {
	Send(ctx context.Context, topic, title, body, clickURL string) error
}

type MockSender struct{}

func (m *MockSender) Send(ctx context.Context, topic, title, body, clickURL string) error {
	// 脱敏日志：严禁在控制台打印用户提醒正文及设备密钥/主题 (PT-07, R3-01)
	log.Printf("[Mock Notification] 标题: %s | 点击链接: %s", title, clickURL)
	return nil
}

type BarkSender struct {
	serverURL  string
	httpClient *http.Client
}

func NewBarkSender(serverURL string) *BarkSender {
	if serverURL == "" {
		serverURL = "https://api.day.app"
	}
	return &BarkSender{
		serverURL:  strings.TrimSuffix(serverURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type barkPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Group string `json:"group,omitempty"`
}

func (b *BarkSender) Send(ctx context.Context, topic, title, body, clickURL string) error {
	deviceKey := strings.TrimSpace(topic)
	if deviceKey == "" {
		return errors.New("bark device key 不能为空")
	}
	targetURL := fmt.Sprintf("%s/%s", b.serverURL, deviceKey)
	payload := barkPayload{
		Title: title,
		Body:  body,
		URL:   clickURL,
		Group: "ChronoLog",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 脱敏：绝不直接向上层暴露包含 device_key 的底层 URL 请求错误 (R3-01)
		return errors.New("bark 请求失败 (网络连接异常或请求超时)")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bark 返回非成功状态码: %d", resp.StatusCode)
	}
	return nil
}

type NtfySender struct {
	serverURL  string
	httpClient *http.Client
}

func NewNtfySender(serverURL string) *NtfySender {
	return &NtfySender{
		serverURL:  strings.TrimSuffix(serverURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *NtfySender) Send(ctx context.Context, topic, title, body, clickURL string) error {
	url := fmt.Sprintf("%s/%s", n.serverURL, topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	if clickURL != "" {
		req.Header.Set("Click", clickURL)
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy 返回非成功状态码: %d", resp.StatusCode)
	}
	return nil
}

type Scheduler struct {
	baseDataDir   string
	appBaseURL    string
	mdStore       *storage.MarkdownStore
	indexMgr      *storage.IndexManager
	accountStore  *storage.AccountStore
	sender        NotificationSender
	loc           *time.Location
	checkInterval time.Duration
	stopChan      chan struct{}
	wg            sync.WaitGroup
	running       bool
	mu            sync.Mutex
	sentCache     map[string]time.Time
	sentMu        sync.Mutex
}

func NewScheduler(
	baseDataDir string,
	appBaseURL string,
	mdStore *storage.MarkdownStore,
	indexMgr *storage.IndexManager,
	accountStore *storage.AccountStore,
	sender NotificationSender,
	loc *time.Location,
	checkInterval time.Duration,
) *Scheduler {
	if checkInterval <= 0 {
		checkInterval = 60 * time.Second
	}
	return &Scheduler{
		baseDataDir:   baseDataDir,
		appBaseURL:    appBaseURL,
		mdStore:       mdStore,
		indexMgr:      indexMgr,
		accountStore:  accountStore,
		sender:        sender,
		loc:           loc,
		checkInterval: checkInterval,
		stopChan:      make(chan struct{}),
		sentCache:     make(map[string]time.Time),
	}
}

// Start 启动后台定时检测协程
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.checkInterval)
		defer ticker.Stop()

		// 启动立即检查一次
		s.checkAndDispatch()

		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				s.checkAndDispatch()
			}
		}
	}()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	s.wg.Wait()
}

func (s *Scheduler) checkAndDispatch() {
	now := time.Now().In(s.loc)
	userDirs, err := s.listUserEmails()
	if err != nil {
		return
	}

	baseURL := s.appBaseURL
	if baseURL == "" {
		baseURL = "https://chronolog.uufang.space"
	}

	for _, email := range userDirs {
		notes, err := s.mdStore.ListAllNotes(email)
		if err != nil {
			continue
		}

		user, _ := s.accountStore.GetUserByEmail(email)
		topic := "default"
		if user != nil && user.NtfyTopic != "" {
			topic = user.NtfyTopic
		}

		for _, note := range notes {
			for _, rem := range note.Reminders {
				// 用户待办为 pending，且推送状态为 pending 或 (failed 且重试次数 < 3) (PT-04)
				canDispatch := rem.Status == "pending" && (rem.NotificationStatus == "pending" || (rem.NotificationStatus == "failed" && rem.NotificationRetries < 3))
				if canDispatch {
					remTime, err := time.Parse(time.RFC3339, rem.RemindAt)
					if err == nil && !remTime.After(now) {
						remID := rem.ID
						noteID := note.ID

						// 检查近期发送缓存防重复推送 (PT-04: 发送后落盘异常重复边界)
						s.sentMu.Lock()
						lastSent, recentlySent := s.sentCache[remID]
						if recentlySent && time.Since(lastSent) < 10*time.Minute {
							s.sentMu.Unlock()
							s.updateReminderStatus(email, noteID, remID, true)
							continue
						}
						s.sentMu.Unlock()

						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						clickURL := fmt.Sprintf("%s/?note_id=%s", strings.TrimSuffix(baseURL, "/"), noteID)
						sendErr := s.sender.Send(ctx, topic, "ChronoLog 备忘提醒", rem.Title, clickURL)
						cancel()

						if sendErr != nil {
							log.Printf("[提醒推送失败] 笔记ID: %s, 提醒ID: %s: %v", noteID, remID, sendErr)
							s.updateReminderStatus(email, noteID, remID, false)
						} else {
							log.Printf("[提醒推送成功] 笔记ID: %s, 提醒ID: %s", noteID, remID)
							s.sentMu.Lock()
							s.sentCache[remID] = time.Now()
							s.sentMu.Unlock()
							s.updateReminderStatus(email, noteID, remID, true)
						}
					}
				}
			}
		}
	}
}

func (s *Scheduler) updateReminderStatus(email, noteID, remID string, success bool) {
	updatedNote, mutateErr := s.mdStore.MutateNoteAtomic(email, noteID, func(targetNote *storage.Note) error {
		for i := range targetNote.Reminders {
			if targetNote.Reminders[i].ID == remID {
				if success {
					targetNote.Reminders[i].NotificationStatus = "sent"
				} else {
					targetNote.Reminders[i].NotificationStatus = "failed"
					targetNote.Reminders[i].NotificationRetries++
				}
				break
			}
		}
		return nil
	}, s.loc)

	if mutateErr == nil && updatedNote != nil {
		if idx, err := s.indexMgr.GetUserIndex(email); err == nil {
			_ = idx.IndexNote(updatedNote, s.loc)
		}
	}
}

func (s *Scheduler) listUserEmails() ([]string, error) {
	usersDir := filepath.Join(s.baseDataDir, "users")
	return storage.ListUserDirs(usersDir)
}
