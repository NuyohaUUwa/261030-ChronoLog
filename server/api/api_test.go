package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"chronolog/server/internal/auth"
	"chronolog/server/internal/config"
	"chronolog/server/internal/llm"
	"chronolog/server/internal/prompt"
	"chronolog/server/internal/storage"
)

func setupTestServer(t *testing.T) (*gin.Engine, *config.Config, func()) {
	gin.SetMode(gin.TestMode)
	tempDir, err := os.MkdirTemp("", "chronolog_api_test_*")
	if err != nil {
		t.Fatal(err)
	}

	loc, _ := time.LoadLocation("Asia/Shanghai")
	cfg := &config.Config{
		Server:  config.ServerConfig{Address: "127.0.0.1:8080"},
		Storage: config.StorageConfig{DataDir: tempDir},
		App:     config.AppConfig{Timezone: "Asia/Shanghai"},
		Auth: config.AuthConfig{
			JWTSecret:        "test-secret-key-32-chars-length!",
			TokenExpiryHours: 24,
			AdminEmails:      []string{"admin@test.local"},
			AdminSetupKey:    "test-admin-setup-key",
		},
		AI: config.AIConfig{
			Mode:           "mock",
			TimeoutSeconds: 5,
		},
		Location: loc,
	}

	promptsPath := filepath.Join(tempDir, "prompts.yaml")
	promptContent := `
version: "1.0"
model_settings:
  temperature: 0.2
  max_tokens: 1500
system_prompt: "你是测试秘书"
dynamic_context_template: "{{CURRENT_DATETIME}}\n{{USER_TAGS}}\n{{RAW_CONTENT}}"
few_shot_examples: []
constraints: ""
`
	if err := os.WriteFile(promptsPath, []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}
	cfg.Prompts.File = promptsPath

	accountsDBPath := filepath.Join(tempDir, "accounts.db")
	accountStore, err := storage.NewAccountStore(accountsDBPath)
	if err != nil {
		t.Fatal(err)
	}

	mdStore := storage.NewMarkdownStore(tempDir)
	indexMgr := storage.NewIndexManager(tempDir)
	promptMgr, err := prompt.NewPromptManager(promptsPath)
	if err != nil {
		t.Fatal(err)
	}
	llmClient := llm.NewMockClient()

	authHandler := NewAuthHandler(cfg, accountStore)
	notesHandler := NewNotesHandler(cfg, mdStore, indexMgr, llmClient, promptMgr)
	calendarHandler := NewCalendarHandler(cfg, indexMgr)
	promptsHandler := NewPromptsHandler(cfg, promptMgr, llmClient, indexMgr)
	systemHandler := NewSystemHandler(cfg, accountStore, mdStore, indexMgr)

	r := gin.New()
	apiV1 := r.Group("/api/v1")
	{
		apiV1.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		apiV1.POST("/auth/register", authHandler.Register)
		apiV1.POST("/auth/login", authHandler.Login)

		authRequired := apiV1.Group("")
		authRequired.Use(auth.AuthMiddleware(cfg.Auth.JWTSecret))
		{
			authRequired.GET("/auth/me", authHandler.GetMe)
			authRequired.POST("/notes", notesHandler.CreateNote)
			authRequired.GET("/notes", notesHandler.ListNotes)
			authRequired.GET("/notes/:id", notesHandler.GetNote)
			authRequired.PUT("/notes/:id", notesHandler.UpdateNote)
			authRequired.DELETE("/notes/:id", notesHandler.DeleteNote)
			authRequired.POST("/notes/:id/reminders/:rem_id/complete", notesHandler.CompleteReminder)
			authRequired.POST("/notes/:id/reminders/:rem_id/cancel", notesHandler.CancelReminder)
			authRequired.GET("/calendar/summary", calendarHandler.GetSummary)

			adminRequired := authRequired.Group("")
			adminRequired.Use(auth.AdminRequired())
			{
				adminRequired.GET("/prompts", promptsHandler.GetPrompts)
				adminRequired.POST("/system/rebuild-index", systemHandler.RebuildIndex)
			}
		}
	}

	cleanup := func() {
		indexMgr.Close()
		accountStore.Close()
		os.RemoveAll(tempDir)
	}

	return r, cfg, cleanup
}

func TestAPI_AuthAndNoteWorkflow(t *testing.T) {
	r, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. 注册普通用户 User A
	regBody, _ := json.Marshal(map[string]string{
		"email":    "userA@example.com",
		"password": "password123",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("注册普通用户失败: %d %s", w.Code, w.Body.String())
	}

	var regResp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &regResp)
	tokenA := regResp.Token
	if tokenA == "" || regResp.IsAdmin {
		t.Fatalf("用户 A token 异常或被错误标记为管理员")
	}

	// 2. 普通用户访问管理员接口（提示词）应被 403 拒绝
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/prompts", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("非管理员应返回 403 Forbidden, 实际: %d", w.Code)
	}

	// 3. User A 创建一条笔记（随手记原稿落盘）
	noteBody, _ := json.Marshal(map[string]string{
		"raw_content": "今天参加了产品例会，明天下午两点需要输出评审意见",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/notes", bytes.NewReader(noteBody))
	req.Header.Set("Authorization", "Bearer "+tokenA)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("创建笔记应返回 202 Accepted, 实际: %d", w.Code)
	}

	var createResp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	noteID := createResp["id"]
	if noteID == "" {
		t.Fatalf("返回的 noteID 不能为空")
	}

	// 稍作等待以确保 goroutine 执行完毕
	time.Sleep(100 * time.Millisecond)

	// 4. User A 查询自己的笔记详情
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/notes/"+noteID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("获取笔记详情失败: %d", w.Code)
	}

	var noteObj storage.Note
	_ = json.Unmarshal(w.Body.Bytes(), &noteObj)
	if noteObj.RawContent != "今天参加了产品例会，明天下午两点需要输出评审意见" {
		t.Fatalf("原稿内容不匹配: %s", noteObj.RawContent)
	}

	// 5. 跨账号隔离验证：注册 User B，User B 尝试读取 User A 的笔记应得到 404
	regBBody, _ := json.Marshal(map[string]string{
		"email":    "userB@example.com",
		"password": "password456",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBBody))
	r.ServeHTTP(w, req)
	var regBResp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &regBResp)
	tokenB := regBResp.Token

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/notes/"+noteID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("跨用户读笔记应当隔离并返回 404 Not Found, 实际返回: %d", w.Code)
	}

	// 6. 管理员注册安全校验：无 key 注册管理员邮箱被 403 拒绝
	regAdminWithoutKey, _ := json.Marshal(map[string]string{
		"email":    "admin@test.local",
		"password": "adminpassword",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regAdminWithoutKey))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("无 Admin Key 注册管理员邮箱应被 403 拦截, 实际: %d", w.Code)
	}

	// 携带正确 Admin Key 注册管理员成功
	regAdminBody, _ := json.Marshal(map[string]string{
		"email":     "admin@test.local",
		"password":  "adminpassword",
		"admin_key": "test-admin-setup-key",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regAdminBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("注册管理员失败: %d %s", w.Code, w.Body.String())
	}
	var adminResp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &adminResp)
	if !adminResp.IsAdmin {
		t.Fatalf("管理员邮箱应被识别为 is_admin=true")
	}

	// 管理员访问提示词配置应返回 200 OK
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/prompts", nil)
	req.Header.Set("Authorization", "Bearer "+adminResp.Token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("管理员访问提示词接口应返回 200, 实际: %d", w.Code)
	}
	fmt.Println("API 核心业务集成测试全项通过")
}

func TestAPI_PathTraversalDefense(t *testing.T) {
	r, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 注册用户
	regBody, _ := json.Marshal(map[string]string{
		"email":    "alice@test.local",
		"password": "password123",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	r.ServeHTTP(w, req)
	var regResp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &regResp)
	token := regResp.Token

	// 恶意路径尝试穿越
	maliciousPaths := []string{
		"/api/v1/notes/..%2f..%2fbob%2fnotes%2fsecret",
		"/api/v1/notes/..%5c..%5cbob%5cnotes%5csecret",
		"/api/v1/notes/sub%2fnote",
		"/api/v1/notes/note;hack",
	}

	for _, malPath := range maliciousPaths {
		w = httptest.NewRecorder()
		req, _ = http.NewRequest(http.MethodGet, malPath, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
			t.Errorf("恶意路径 %s 预期返回 404 或 400, 得到: %d", malPath, w.Code)
		}
	}
}

func TestAPI_FTSDoubleQuoteSearch(t *testing.T) {
	r, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 注册并登录
	regBody, _ := json.Marshal(map[string]string{
		"email":    "searcher@test.local",
		"password": "password123",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	r.ServeHTTP(w, req)
	var regResp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &regResp)
	token := regResp.Token

	// 创建测试笔记
	noteBody, _ := json.Marshal(map[string]string{
		"raw_content": "今天配置了 Docker 容器的 \"port_mapping\" 参数",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/notes", bytes.NewReader(noteBody))
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	time.Sleep(100 * time.Millisecond)

	// 带引号搜索测试 (不引起 SQLite FTS 语法错误 500)
	testKeywords := []string{
		`"port_mapping"`,
		`"`,
		`""`,
		`"""`,
		`Docker"`,
	}

	for _, kw := range testKeywords {
		w = httptest.NewRecorder()
		req, _ = http.NewRequest(http.MethodGet, "/api/v1/notes?keyword="+kw, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("带引号关键词 %q 检索应返回 200, 实际返回 %d: %s", kw, w.Code, w.Body.String())
		}
	}
}

func TestAPI_PreserveWhitespaceInRawNote(t *testing.T) {
	r, _, cleanup := setupTestServer(t)
	defer cleanup()

	regBody, _ := json.Marshal(map[string]string{
		"email":    "ws@test.local",
		"password": "password123",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	r.ServeHTTP(w, req)
	var regResp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &regResp)
	token := regResp.Token

	rawWithExactSpaces := "  \n\tline 1\n\t  line 2  \n"
	noteBody, _ := json.Marshal(map[string]string{
		"raw_content": rawWithExactSpaces,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/notes", bytes.NewReader(noteBody))
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("创建笔记失败: %d", w.Code)
	}

	var createResp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	noteID := createResp["id"]

	time.Sleep(100 * time.Millisecond)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/notes/"+noteID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	var noteObj storage.Note
	_ = json.Unmarshal(w.Body.Bytes(), &noteObj)
	if noteObj.RawContent != rawWithExactSpaces {
		t.Fatalf("原稿空白字符被篡改: 期望 %q, 得到 %q", rawWithExactSpaces, noteObj.RawContent)
	}
}

func TestAPI_RebuildIndexEndpoint(t *testing.T) {
	r, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 注册普通用户与管理员
	regAdminBody, _ := json.Marshal(map[string]string{
		"email":     "admin@test.local",
		"password":  "adminpassword",
		"admin_key": "test-admin-setup-key",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regAdminBody))
	r.ServeHTTP(w, req)
	var adminResp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &adminResp)

	// 管理员触发重建索引
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/system/rebuild-index", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+adminResp.Token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("管理员触发重建索引接口失败: %d %s", w.Code, w.Body.String())
	}
}

func TestAPI_ReminderCompleteAndCancel(t *testing.T) {
	r, _, cleanup := setupTestServer(t)
	defer cleanup()

	// 注册用户
	regBody, _ := json.Marshal(map[string]string{
		"email":    "user_rem@example.com",
		"password": "password123",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	r.ServeHTTP(w, req)
	var regResp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &regResp)
	token := regResp.Token

	// 创建笔记
	createBody, _ := json.Marshal(map[string]string{
		"raw_content": "明天下午开会讨论方案",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/notes", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	var createResp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	noteID := createResp["id"]

	// 等待并更新增加一条测试待办
	time.Sleep(50 * time.Millisecond)
	remID := "rem_test_1"
	reminders := []storage.Reminder{
		{
			ID:                 remID,
			Title:              "开会讨论方案",
			RemindAt:           "2026-11-08T15:00:00+08:00",
			Status:             "pending",
			NotificationStatus: "pending",
		},
	}
	updateBody, _ := json.Marshal(map[string]any{
		"reminders": reminders,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/api/v1/notes/"+noteID, bytes.NewReader(updateBody))
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("更新笔记提醒失败: %d %s", w.Code, w.Body.String())
	}

	// 1. 标记完成
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/notes/%s/reminders/%s/complete", noteID, remID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("标记提醒完成失败: %d %s", w.Code, w.Body.String())
	}

	// 2. 取消提醒
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/notes/%s/reminders/%s/cancel", noteID, remID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("取消提醒失败: %d %s", w.Code, w.Body.String())
	}
	var cancelResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &cancelResp)
	if cancelResp["updated_at"] == nil || cancelResp["updated_at"] == "" {
		t.Fatalf("取消待办响应缺少 updated_at: %s", w.Body.String())
	}
	latestUpdatedAt := cancelResp["updated_at"].(string)

	// 使用取消待办后取得的 latestUpdatedAt 进行正文编辑，验证不会产生 409 冲突 (R3-02)
	editBody, _ := json.Marshal(map[string]any{
		"polished_content": "更新后的正文",
		"updated_at":       latestUpdatedAt,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/api/v1/notes/"+noteID, bytes.NewReader(editBody))
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("使用同步后的版本保存编辑失败: %d %s", w.Code, w.Body.String())
	}

	// 检查最终状态
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/notes/"+noteID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	var noteObj storage.Note
	_ = json.Unmarshal(w.Body.Bytes(), &noteObj)
	if len(noteObj.Reminders) != 1 || noteObj.Reminders[0].Status != "cancelled" {
		t.Fatalf("期望提醒状态为 cancelled, 得到: %+v", noteObj.Reminders)
	}

	// 3. 取消不存在的提醒应返回 404
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/notes/%s/reminders/non_existent/cancel", noteID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("取消不存在的提醒期望 404, 得到 %d", w.Code)
	}
}
