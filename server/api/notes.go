package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"chronolog/server/internal/config"
	"chronolog/server/internal/llm"
	"chronolog/server/internal/prompt"
	"chronolog/server/internal/storage"
)

type NotesHandler struct {
	cfg       *config.Config
	mdStore   *storage.MarkdownStore
	indexMgr  *storage.IndexManager
	llmClient llm.Client
	promptMgr *prompt.PromptManager
}

func NewNotesHandler(
	cfg *config.Config,
	mdStore *storage.MarkdownStore,
	indexMgr *storage.IndexManager,
	llmClient llm.Client,
	promptMgr *prompt.PromptManager,
) *NotesHandler {
	return &NotesHandler{
		cfg:       cfg,
		mdStore:   mdStore,
		indexMgr:  indexMgr,
		llmClient: llmClient,
		promptMgr: promptMgr,
	}
}

type CreateNoteRequest struct {
	RawContent string `json:"raw_content" binding:"required"`
}

type UpdateNoteRequest struct {
	PolishedContent *string             `json:"polished_content"`
	Tags            *[]string           `json:"tags"`
	Reminders       *[]storage.Reminder `json:"reminders"`
	UpdatedAt       string              `json:"updated_at"` // 修订版本校验
}

// CreateNote 创建随手记（毫秒级落盘原稿，异步触发 AI 流水线）
func (h *NotesHandler) CreateNote(c *gin.Context) {
	email := c.GetString("user_email")
	var req CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "随笔原稿不能为空"}})
		return
	}

	raw := strings.TrimSpace(req.RawContent)
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "随笔原稿不能为空"}})
		return
	}

	// 1. 毫秒级保存权威原稿至 Markdown 文件
	note, err := h.mdStore.SaveNoteRaw(email, raw, h.cfg.Location)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": "保存笔记原稿失败: " + err.Error()}})
		return
	}

	// 2. 同步写入 SQLite 检索元数据，确保创建后立即被查询到
	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(note, h.cfg.Location)
	}

	// 3. 异步启动 AI 处理流水线
	go h.processNoteAsync(email, note.ID)

	// 4. 返回 HTTP 202 Accepted
	c.JSON(http.StatusAccepted, gin.H{
		"id":     note.ID,
		"status": note.ProcessingStatus,
	})
}

// processNoteAsync 执行异步 AI 润色与结构化抽取流水线
func (h *NotesHandler) processNoteAsync(email, noteID string) {
	note, err := h.mdStore.ReadNote(email, noteID)
	if err != nil {
		log.Printf("[AI 流水线] 读取笔记失败 %s: %v", noteID, err)
		return
	}

	// 提取用户现有常用标签以作为上下文 Few-Shot 参考
	var userTagNames []string
	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		tags, _ := idx.GetUserTags()
		for _, t := range tags {
			userTagNames = append(userTagNames, t.Tag)
		}
	}

	// 锚定基准时间戳（使用笔记创建时的精准时间戳，防止相对时间漂移）
	refTime, err := time.Parse(time.RFC3339, note.CreatedAt)
	if err != nil {
		refTime = time.Now().In(h.cfg.Location)
	}

	systemPrompt, userPrompt, err := h.promptMgr.RenderPrompt(note.RawContent, refTime, userTagNames)
	if err != nil {
		log.Printf("[AI 流水线] 渲染提示词失败 %s: %v", noteID, err)
		h.markNoteFailed(email, noteID, "渲染提示词失败: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(h.cfg.AI.TimeoutSeconds)*time.Second)
	defer cancel()

	result, err := h.llmClient.ProcessNote(ctx, systemPrompt, userPrompt, note.RawContent)
	if err != nil {
		log.Printf("[AI 流水线] 大模型调用失败 %s: %v", noteID, err)
		h.markNoteFailed(email, noteID, "大模型处理失败: "+err.Error())
		return
	}

	// 检查处理期间笔记是否被用户手动修改过
	currentNote, err := h.mdStore.ReadNote(email, noteID)
	if err != nil {
		return // 可能已被删除
	}

	// 回填润色结果
	currentNote.ProcessingStatus = "ready"
	currentNote.FailureReason = ""
	currentNote.PolishedContent = result.PolishedContent
	currentNote.Tags = result.Tags
	currentNote.Reminders = result.Reminders
	currentNote.UpdatedAt = time.Now().In(h.cfg.Location).Format(time.RFC3339)

	if err := h.mdStore.WriteNoteAtomic(email, currentNote); err != nil {
		log.Printf("[AI 流水线] 原子写入笔记失败 %s: %v", noteID, err)
		return
	}

	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(currentNote, h.cfg.Location)
	}
}

func (h *NotesHandler) markNoteFailed(email, noteID, reason string) {
	currentNote, err := h.mdStore.ReadNote(email, noteID)
	if err != nil {
		return
	}
	currentNote.ProcessingStatus = "failed"
	currentNote.FailureReason = reason
	currentNote.UpdatedAt = time.Now().In(h.cfg.Location).Format(time.RFC3339)

	_ = h.mdStore.WriteNoteAtomic(email, currentNote)
	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(currentNote, h.cfg.Location)
	}
}

// ListNotes 多维查询笔记列表（支持日期、标签、全文检索）
func (h *NotesHandler) ListNotes(c *gin.Context) {
	email := c.GetString("user_email")
	idx, err := h.indexMgr.GetUserIndex(email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INDEX_ERROR", "message": err.Error()}})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	filter := storage.NoteFilter{
		Date:    c.Query("date"),
		Tag:     c.Query("tag"),
		Keyword: c.Query("keyword"),
		Page:    page,
		Limit:   limit,
	}

	notes, total, err := idx.QueryNotes(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "QUERY_ERROR", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": notes,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetNote 获取单条笔记详情（直接读取 Markdown 权威文件）
func (h *NotesHandler) GetNote(c *gin.Context) {
	email := c.GetString("user_email")
	noteID := c.Param("id")

	note, err := h.mdStore.ReadNote(email, noteID)
	if err != nil {
		if errors.Is(err, storage.ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, note)
}

// UpdateNote 更新笔记内容或标签（支持修订版本校验防冲突）
func (h *NotesHandler) UpdateNote(c *gin.Context) {
	email := c.GetString("user_email")
	noteID := c.Param("id")

	var req UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "参数错误"}})
		return
	}

	note, err := h.mdStore.ReadNote(email, noteID)
	if err != nil {
		if errors.Is(err, storage.ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}

	// 版本冲突检查
	if req.UpdatedAt != "" && note.UpdatedAt != req.UpdatedAt {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "REVISION_CONFLICT", "message": "笔记已被修改，请刷新后重试"}})
		return
	}

	if req.PolishedContent != nil {
		note.PolishedContent = *req.PolishedContent
	}
	if req.Tags != nil {
		note.Tags = *req.Tags
	}
	if req.Reminders != nil {
		note.Reminders = *req.Reminders
	}

	note.UpdatedAt = time.Now().In(h.cfg.Location).Format(time.RFC3339)

	if err := h.mdStore.WriteNoteAtomic(email, note); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}

	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(note, h.cfg.Location)
	}

	c.JSON(http.StatusOK, note)
}

// DeleteNote 删除笔记
func (h *NotesHandler) DeleteNote(c *gin.Context) {
	email := c.GetString("user_email")
	noteID := c.Param("id")

	if err := h.mdStore.DeleteNote(email, noteID); err != nil {
		if errors.Is(err, storage.ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}

	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.DeleteNoteIndex(noteID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功", "id": noteID})
}

// RetryNoteAI 重试失败的 AI 润色
func (h *NotesHandler) RetryNoteAI(c *gin.Context) {
	email := c.GetString("user_email")
	noteID := c.Param("id")

	note, err := h.mdStore.ReadNote(email, noteID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
		return
	}

	note.ProcessingStatus = "processing"
	note.FailureReason = ""
	note.UpdatedAt = time.Now().In(h.cfg.Location).Format(time.RFC3339)

	if err := h.mdStore.WriteNoteAtomic(email, note); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}
	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(note, h.cfg.Location)
	}

	go h.processNoteAsync(email, noteID)

	c.JSON(http.StatusAccepted, gin.H{
		"id":     noteID,
		"status": "processing",
	})
}

// CompleteReminder 标记单条待办已完成
func (h *NotesHandler) CompleteReminder(c *gin.Context) {
	email := c.GetString("user_email")
	noteID := c.Param("id")
	remID := c.Param("rem_id")

	note, err := h.mdStore.ReadNote(email, noteID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
		return
	}

	found := false
	for i := range note.Reminders {
		if note.Reminders[i].ID == remID {
			note.Reminders[i].Status = "completed"
			found = true
			break
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "REMINDER_NOT_FOUND", "message": "待办提醒项不存在"}})
		return
	}

	note.UpdatedAt = time.Now().In(h.cfg.Location).Format(time.RFC3339)
	if err := h.mdStore.WriteNoteAtomic(email, note); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}

	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(note, h.cfg.Location)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "待办已标记完成",
		"note_id":   noteID,
		"rem_id":    remID,
		"reminders": note.Reminders,
	})
}
