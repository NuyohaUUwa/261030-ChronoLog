package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"chronolog/server/internal/config"
	"chronolog/server/internal/llm"
	"chronolog/server/internal/prompt"
	"chronolog/server/internal/storage"
)

type PromptsHandler struct {
	cfg       *config.Config
	promptMgr *prompt.PromptManager
	llmClient llm.Client
	indexMgr  *storage.IndexManager
}

func NewPromptsHandler(
	cfg *config.Config,
	promptMgr *prompt.PromptManager,
	llmClient llm.Client,
	indexMgr *storage.IndexManager,
) *PromptsHandler {
	return &PromptsHandler{
		cfg:       cfg,
		promptMgr: promptMgr,
		llmClient: llmClient,
		indexMgr:  indexMgr,
	}
}

// GetPrompts 获取当前提示词配置内容（管理员专属）
func (h *PromptsHandler) GetPrompts(c *gin.Context) {
	content, err := h.promptMgr.GetRawYAML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "READ_ERROR", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"yaml": content})
}

// UpdatePrompts 更新提示词配置（热生效，管理员专属）
func (h *PromptsHandler) UpdatePrompts(c *gin.Context) {
	var req struct {
		YAML string `json:"yaml" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "YAML 内容不能为空"}})
		return
	}

	if err := h.promptMgr.UpdateRawYAML(req.YAML); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_YAML", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "提示词更新成功并已热生效"})
}

// TestPrompt 在线测试提示词调优效果（管理员专属）
func (h *PromptsHandler) TestPrompt(c *gin.Context) {
	var req struct {
		RawContent string `json:"raw_content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "测试内容不能为空"}})
		return
	}

	email := c.GetString("user_email")
	var userTagNames []string
	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		tags, _ := idx.GetUserTags()
		for _, t := range tags {
			userTagNames = append(userTagNames, t.Tag)
		}
	}

	now := time.Now().In(h.cfg.Location)
	systemPrompt, userPrompt, err := h.promptMgr.RenderPrompt(req.RawContent, now, userTagNames)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "RENDER_ERROR", "message": err.Error()}})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(h.cfg.AI.TimeoutSeconds)*time.Second)
	defer cancel()

	result, err := h.llmClient.ProcessNote(ctx, systemPrompt, userPrompt, req.RawContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "AI_ERROR", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"system_prompt": systemPrompt,
		"user_prompt":   userPrompt,
		"result":        result,
	})
}
