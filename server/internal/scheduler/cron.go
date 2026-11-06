package scheduler

import (
	"context"
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
	log.Printf("[Mock Notification] 发送给主题 %s | 标题: %s | 内容: %s | 点击链接: %s", topic, title, body, clickURL)
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
}

func NewScheduler(
	baseDataDir string,
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
		mdStore:       mdStore,
		indexMgr:      indexMgr,
		accountStore:  accountStore,
		sender:        sender,
		loc:           loc,
		checkInterval: checkInterval,
		stopChan:      make(chan struct{}),
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
	// 遍历所有用户检查待推送提醒
	// 通过用户索引查询或者笔记扫描
	// 在个人多账号架构下，扫描每个注册用户
	now := time.Now().In(s.loc)

	// 从 accounts.db 查询用户列表
	// 由于 accounts.go 没暴露全量用户，增加一个内部扫描或遍历 data/users 目录
	userDirs, err := s.listUserEmails()
	if err != nil {
		return
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
			changed := false
			for i, rem := range note.Reminders {
				// 用户待办为 pending，且推送状态为 pending
				if rem.Status == "pending" && rem.NotificationStatus == "pending" {
					remTime, err := time.Parse(time.RFC3339, rem.RemindAt)
					if err == nil && !remTime.After(now) {
						// 到期，执行派发
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						clickURL := fmt.Sprintf("https://notes.uufang.space/?note_id=%s", note.ID)
						err := s.sender.Send(ctx, topic, "ChronoLog 备忘提醒", rem.Title, clickURL)
						cancel()

						if err != nil {
							log.Printf("[提醒推送失败] 笔记ID: %s, 提醒ID: %s: %v", note.ID, rem.ID, err)
							note.Reminders[i].NotificationStatus = "failed"
						} else {
							log.Printf("[提醒推送成功] 笔记ID: %s, 提醒ID: %s", note.ID, rem.ID)
							note.Reminders[i].NotificationStatus = "sent"
						}
						changed = true
					}
				}
			}

			if changed {
				note.UpdatedAt = time.Now().In(s.loc).Format(time.RFC3339)
				_ = s.mdStore.WriteNoteAtomic(email, note)
				if idx, err := s.indexMgr.GetUserIndex(email); err == nil {
					_ = idx.IndexNote(note, s.loc)
				}
			}
		}
	}
}

func (s *Scheduler) listUserEmails() ([]string, error) {
	usersDir := filepath.Join(s.baseDataDir, "users")
	return storage.ListUserDirs(usersDir)
}
