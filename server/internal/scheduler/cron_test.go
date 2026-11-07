package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"chronolog/server/internal/storage"
)

func TestBarkSender(t *testing.T) {
	var received barkPayload
	var authHeader string
	var path string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		authHeader = r.Header.Get("Content-Type")
		_ = authHeader
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer srv.Close()

	sender := NewBarkSender(srv.URL)
	err := sender.Send(context.Background(), "my_device_key", "测试标题", "测试内容", "https://chronolog.uufang.space/?note_id=123")
	if err != nil {
		t.Fatalf("BarkSender.Send failed: %v", err)
	}

	if path != "/my_device_key" {
		t.Errorf("expected path /my_device_key, got %s", path)
	}
	if received.Title != "测试标题" || received.Body != "测试内容" || received.URL != "https://chronolog.uufang.space/?note_id=123" || received.Group != "ChronoLog" {
		t.Errorf("unexpected payload: %+v", received)
	}
}

func TestBarkSender_SanitizedErrorOnFailure(t *testing.T) {
	// 指向一个无响应端口以模拟网络故障，验证绝不泄露 secretKey (R3-01)
	sender := NewBarkSender("http://127.0.0.1:59999")
	secretKey := "super_secret_device_canary_key"
	err := sender.Send(context.Background(), secretKey, "title", "body", "")
	if err == nil {
		t.Fatal("expected error on connection failure, got nil")
	}
	if strings.Contains(err.Error(), secretKey) {
		t.Fatalf("BarkSender error leaked secret device key: %v", err)
	}
}

type spySender struct {
	callCount int32
	shouldErr bool
}

func (s *spySender) Send(ctx context.Context, topic, title, body, clickURL string) error {
	atomic.AddInt32(&s.callCount, 1)
	if s.shouldErr {
		return fmt.Errorf("mock send error")
	}
	return nil
}

func TestScheduler_DispatchAndCancelledIgnored(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "chronolog-scheduler-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	accountDBPath := filepath.Join(tempDir, "accounts.db")
	accountStore, err := storage.NewAccountStore(accountDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer accountStore.Close()

	email := "test@example.com"
	_, err = accountStore.CreateUser(email, "hash", "topic_123")
	if err != nil {
		t.Fatal(err)
	}

	mdStore := storage.NewMarkdownStore(tempDir)
	indexMgr := storage.NewIndexManager(tempDir)
	defer indexMgr.Close()

	loc := time.FixedZone("CST", 8*3600)

	// 创建带到期待办的笔记
	note, err := mdStore.SaveNoteRaw(email, "明天上午买牛奶", loc)
	if err != nil {
		t.Fatal(err)
	}

	// 注入两条待办：一条 pending（已到期），一条 cancelled（已取消）
	pastTime := time.Now().In(loc).Add(-1 * time.Minute).Format(time.RFC3339)
	_, err = mdStore.MutateNoteAtomic(email, note.ID, func(n *storage.Note) error {
		n.Reminders = []storage.Reminder{
			{
				ID:                 "rem_pending",
				Title:              "买牛奶",
				RemindAt:           pastTime,
				Status:             "pending",
				NotificationStatus: "pending",
			},
			{
				ID:                 "rem_cancelled",
				Title:              "已取消的任务",
				RemindAt:           pastTime,
				Status:             "cancelled",
				NotificationStatus: "pending",
			},
		}
		return nil
	}, loc)
	if err != nil {
		t.Fatal(err)
	}

	spy := &spySender{}
	sched := NewScheduler(tempDir, "https://test.local", mdStore, indexMgr, accountStore, spy, loc, 1*time.Minute)

	// 执行一次检查与派发
	sched.checkAndDispatch()

	// 验证：只有 rem_pending 被发送，rem_cancelled 绝不发送 (callCount 应为 1)
	if count := atomic.LoadInt32(&spy.callCount); count != 1 {
		t.Fatalf("expected 1 send call, got %d", count)
	}

	// 检查落盘状态：rem_pending 变为 sent
	updated, err := mdStore.ReadNote(email, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range updated.Reminders {
		if r.ID == "rem_pending" && r.NotificationStatus != "sent" {
			t.Fatalf("rem_pending should be sent, got %s", r.NotificationStatus)
		}
	}

	// 再次执行派发：因为近期已发送且状态为 sent，callCount 应该依然是 1
	sched.checkAndDispatch()
	if count := atomic.LoadInt32(&spy.callCount); count != 1 {
		t.Fatalf("expected call count to remain 1, got %d", count)
	}
}
