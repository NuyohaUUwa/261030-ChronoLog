package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"chronolog/server/internal/config"
	"chronolog/server/internal/llm"
	"chronolog/server/internal/storage"
	"github.com/gin-gonic/gin"
)

type SystemHandler struct {
	cfg           *config.Config
	accountStore  *storage.AccountStore
	mdStore       *storage.MarkdownStore
	indexMgr      *storage.IndexManager
	dynamicClient *llm.DynamicClient
}

func NewSystemHandler(cfg *config.Config, accountStore *storage.AccountStore, mdStore *storage.MarkdownStore, indexMgr *storage.IndexManager, dynamicClient *llm.DynamicClient) *SystemHandler {
	return &SystemHandler{
		cfg:           cfg,
		accountStore:  accountStore,
		mdStore:       mdStore,
		indexMgr:      indexMgr,
		dynamicClient: dynamicClient,
	}
}

type RebuildIndexRequest struct {
	Email string `json:"email"`
}

// RebuildIndex 供管理员手动触发全量索引重建
func (h *SystemHandler) RebuildIndex(c *gin.Context) {
	var req RebuildIndexRequest
	_ = c.ShouldBindJSON(&req)

	var targetEmails []string
	if req.Email != "" {
		targetEmails = append(targetEmails, req.Email)
	} else {
		users, err := h.accountStore.ListUsers()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "DB_ERROR", "message": err.Error()}})
			return
		}
		for _, u := range users {
			targetEmails = append(targetEmails, u.Email)
		}
	}

	rebuiltUsers := 0
	totalNotes := 0
	for _, email := range targetEmails {
		notes, err := h.mdStore.ListAllNotes(email)
		if err != nil {
			log.Printf("[系统索引重建] 读取用户 %s 笔记列表失败: %v", email, err)
			continue
		}
		idx, err := h.indexMgr.GetUserIndex(email)
		if err != nil {
			log.Printf("[系统索引重建] 获取用户 %s 索引失败: %v", email, err)
			continue
		}
		if err := idx.RebuildIndex(notes, h.cfg.Location); err != nil {
			log.Printf("[系统索引重建] 重建用户 %s 索引失败: %v", email, err)
			continue
		}
		rebuiltUsers++
		totalNotes += len(notes)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"users_rebuilt": rebuiltUsers,
		"notes_rebuilt": totalNotes,
	})
}

func maskAPIKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if len(trimmed) <= 8 {
		if len(trimmed) == 0 {
			return ""
		}
		return "******"
	}
	return trimmed[:6] + "..." + trimmed[len(trimmed)-4:]
}

// GetAIConfig 获取当前 AI 服务配置（管理员专属）
func (h *SystemHandler) GetAIConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"mode":            h.cfg.AI.Mode,
		"base_url":        h.cfg.AI.BaseURL,
		"model":           h.cfg.AI.Model,
		"timeout_seconds": h.cfg.AI.TimeoutSeconds,
		"has_api_key":     h.cfg.AI.APIKey != "",
		"api_key_masked":  maskAPIKey(h.cfg.AI.APIKey),
	})
}

type UpdateAIConfigRequest struct {
	Mode           string `json:"mode" binding:"required"`
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// UpdateAIConfig 更新 AI 服务配置并热生效（管理员专属）
func (h *SystemHandler) UpdateAIConfig(c *gin.Context) {
	var req UpdateAIConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "请求参数不合法"}})
		return
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "mock" && mode != "remote" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "mode 必须为 mock 或 remote"}})
		return
	}

	timeoutSec := req.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 30
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = h.cfg.AI.APIKey
	}

	newAI := config.AIConfig{
		Mode:           mode,
		BaseURL:        strings.TrimSuffix(strings.TrimSpace(req.BaseURL), "/"),
		APIKey:         apiKey,
		Model:          strings.TrimSpace(req.Model),
		TimeoutSeconds: timeoutSec,
	}

	if err := h.cfg.SaveAIConfig(newAI); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "CONFIG_ERROR", "message": err.Error()}})
		return
	}

	// 热更新底层 LLM 客户端
	if h.dynamicClient != nil {
		if newAI.Mode == "remote" {
			h.dynamicClient.Switch(llm.NewRemoteClient(newAI.BaseURL, newAI.APIKey, newAI.Model, time.Duration(newAI.TimeoutSeconds)*time.Second))
		} else {
			h.dynamicClient.Switch(llm.NewMockClient())
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "AI 配置更新成功并已热生效",
		"mode":            h.cfg.AI.Mode,
		"base_url":        h.cfg.AI.BaseURL,
		"model":           h.cfg.AI.Model,
		"timeout_seconds": h.cfg.AI.TimeoutSeconds,
		"has_api_key":     h.cfg.AI.APIKey != "",
		"api_key_masked":  maskAPIKey(h.cfg.AI.APIKey),
	})
}

// TestAIConfig 测试指定或当前 AI 配置连通性（管理员专属）
func (h *SystemHandler) TestAIConfig(c *gin.Context) {
	var req struct {
		Mode    string `json:"mode"`
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
		Model   string `json:"model"`
	}
	_ = c.ShouldBindJSON(&req)

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = h.cfg.AI.Mode
	}
	if mode == "mock" {
		c.JSON(http.StatusOK, gin.H{
			"status":     "ok",
			"latency_ms": 1,
			"reply":      "Mock 模式运行正常",
		})
		return
	}

	baseURL := strings.TrimSuffix(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = h.cfg.AI.BaseURL
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = h.cfg.AI.APIKey
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = h.cfg.AI.Model
	}

	t0 := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	reply, err := llm.TestRawConnection(ctx, baseURL, apiKey, model, 20*time.Second)
	latency := time.Since(t0).Milliseconds()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"code":    "AI_CONNECT_FAILED",
				"message": "AI 连通性测试失败: " + err.Error(),
			},
			"latency_ms": latency,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"latency_ms": latency,
		"reply":      reply,
	})
}
