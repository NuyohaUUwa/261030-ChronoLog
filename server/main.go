package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"chronolog/server/api"
	"chronolog/server/assets"
	"chronolog/server/internal/auth"
	"chronolog/server/internal/config"
	"chronolog/server/internal/llm"
	"chronolog/server/internal/prompt"
	"chronolog/server/internal/scheduler"
	"chronolog/server/internal/storage"
)

func main() {
	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	log.Printf("==================================================")
	log.Printf("  ChronoLog 服务端启动中")
	log.Printf("  监听地址: %s", cfg.Server.Address)
	log.Printf("  数据目录: %s", cfg.Storage.DataDir)
	log.Printf("  系统时区: %s", cfg.App.Timezone)
	log.Printf("  AI 模式:  %s", cfg.AI.Mode)
	log.Printf("  通知模式: %s", cfg.Notifications.Mode)
	log.Printf("==================================================")

	// 2. 初始化持久存储与目录
	if err := os.MkdirAll(cfg.Storage.DataDir, 0755); err != nil {
		log.Fatalf("初始化数据根目录失败: %v", err)
	}

	accountsDBPath := filepath.Join(cfg.Storage.DataDir, "accounts.db")
	accountStore, err := storage.NewAccountStore(accountsDBPath)
	if err != nil {
		log.Fatalf("初始化账号库失败: %v", err)
	}
	defer accountStore.Close()

	mdStore := storage.NewMarkdownStore(cfg.Storage.DataDir)
	indexMgr := storage.NewIndexManager(cfg.Storage.DataDir)

	// 3. 初始化提示词引擎
	promptMgr, err := prompt.NewPromptManager(cfg.Prompts.File)
	if err != nil {
		log.Fatalf("初始化提示词引擎失败: %v", err)
	}

	// 4. 初始化 AI 客户端
	var llmClient llm.Client
	if cfg.AI.Mode == "remote" {
		llmClient = llm.NewRemoteClient(cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model, time.Duration(cfg.AI.TimeoutSeconds)*time.Second)
	} else {
		llmClient = llm.NewMockClient()
	}

	// 5. 初始化通知派发器与定时轮询调度器
	var notificationSender scheduler.NotificationSender
	if cfg.Notifications.Mode == "remote" {
		notificationSender = scheduler.NewNtfySender(cfg.Notifications.NtfyServer)
	} else {
		notificationSender = &scheduler.MockSender{}
	}

	cronScheduler := scheduler.NewScheduler(
		cfg.Storage.DataDir,
		mdStore,
		indexMgr,
		accountStore,
		notificationSender,
		cfg.Location,
		time.Duration(cfg.Notifications.CheckIntervalSeconds)*time.Second,
	)
	cronScheduler.Start()
	defer cronScheduler.Stop()

	// 6. 构造 Gin 路由引擎
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// CORS 中间件（满足浏览器 Vite 调试及 Android WebView 跨域请求）
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// 初始化 API 处理器
	authHandler := api.NewAuthHandler(cfg, accountStore)
	notesHandler := api.NewNotesHandler(cfg, mdStore, indexMgr, llmClient, promptMgr)
	calendarHandler := api.NewCalendarHandler(cfg, indexMgr)
	promptsHandler := api.NewPromptsHandler(cfg, promptMgr, llmClient, indexMgr)

	// API 路由组
	apiV1 := r.Group("/api/v1")
	{
		// 健康检查
		apiV1.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status": "ok",
				"time":   time.Now().In(cfg.Location).Format(time.RFC3339),
			})
		})

		// 认证接口
		apiV1.POST("/auth/register", authHandler.Register)
		apiV1.POST("/auth/login", authHandler.Login)

		// 业务接口（需 JWT 认证）
		authRequired := apiV1.Group("")
		authRequired.Use(auth.AuthMiddleware(cfg.Auth.JWTSecret))
		{
			// 用户信息与设置
			authRequired.GET("/auth/me", authHandler.GetMe)
			authRequired.PUT("/auth/settings", authHandler.UpdateSettings)

			// 随手记与回顾
			authRequired.POST("/notes", notesHandler.CreateNote)
			authRequired.GET("/notes", notesHandler.ListNotes)
			authRequired.GET("/notes/:id", notesHandler.GetNote)
			authRequired.PUT("/notes/:id", notesHandler.UpdateNote)
			authRequired.DELETE("/notes/:id", notesHandler.DeleteNote)
			authRequired.POST("/notes/:id/retry", notesHandler.RetryNoteAI)
			authRequired.POST("/notes/:id/reminders/:rem_id/complete", notesHandler.CompleteReminder)

			// 双模日历与标签
			authRequired.GET("/calendar/summary", calendarHandler.GetSummary)
			authRequired.GET("/tags", calendarHandler.GetTags)

			// 提示词管理与调优（需管理员权限）
			adminRequired := authRequired.Group("")
			adminRequired.Use(auth.AdminRequired())
			{
				adminRequired.GET("/prompts", promptsHandler.GetPrompts)
				adminRequired.PUT("/prompts", promptsHandler.UpdatePrompts)
				adminRequired.POST("/prompts/test", promptsHandler.TestPrompt)
			}
		}
	}

	// 挂载静态资源（区分生产 embed 与开发模式）
	fsys, isEmbedded := assets.GetFileSystem()
	if isEmbedded && fsys != nil {
		fileServer := http.FileServer(fsys)
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			if strings.HasPrefix(path, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "API 接口不存在"}})
				return
			}

			// 检查内嵌资源中是否存在此文件，若不存在则 fallback 到 index.html (SPA 支持)
			f, err := fsys.Open(strings.TrimPrefix(path, "/"))
			if err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}

			// SPA fallback: 读取并返回 index.html
			c.Request.URL.Path = "/"
			fileServer.ServeHTTP(c.Writer, c.Request)
		})
	} else {
		// 开发模式（Vite 独立运行）
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "API 接口不存在"}})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"message": "ChronoLog 后端正在以 dev 模式运行，前端页面请访问 Vite 服务: http://127.0.0.1:5173",
			})
		})
	}

	srv := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务启动失败: %v", err)
		}
	}()

	fmt.Printf("\n🚀 ChronoLog 服务成功运行于: http://%s\n\n", cfg.Server.Address)

	// 优雅关机监听
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在优雅关闭 ChronoLog 服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("强制关闭服务失败: %v", err)
	}

	log.Println("ChronoLog 服务已安全退出")
}
