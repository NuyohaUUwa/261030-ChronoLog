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

// CreateNote 创建随手记（毫秒级落盘原稿，保持原汁原味首尾空白不被丢弃）
func (h *NotesHandler) CreateNote(c *gin.Context) {
	email := c.GetString("user_email")
	var req CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "随笔原稿不能为空"}})
		return
	}

	// 仅使用 TrimSpace 判断是否全为空白，保存时严格保留用户输入的原始字符串（包含首尾空格与换行）
	if strings.TrimSpace(req.RawContent) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "随笔原稿不能为空"}})
		return
	}

	// 1. 毫秒级保存权威原稿至 Markdown 文件
	note, err := h.mdStore.SaveNoteRaw(email, req.RawContent, h.cfg.Location)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": "保存笔记原稿失败: " + err.Error()}})
		return
	}

	// 2. 同步写入 SQLite 检索元数据，确保创建后立即被查询到
	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		if indexErr := idx.IndexNote(note, h.cfg.Location); indexErr != nil {
			log.Printf("[索引警告] 初始创建笔记索引写入失败: %v", indexErr)
		}
	}

	// 3. 异步启动 AI 处理流水线
	go h.ProcessNoteAsync(email, note.ID, note.CurrentTaskID)

	// 4. 返回 HTTP 202 Accepted
	c.JSON(http.StatusAccepted, gin.H{
		"id":     note.ID,
		"status": note.ProcessingStatus,
	})
}

// ProcessNoteAsync 执行异步 AI 润色与结构化抽取流水线（防并发覆盖与旧任务竞态）
func (h *NotesHandler) ProcessNoteAsync(email, noteID, taskID string) {
	note, err := h.mdStore.ReadNote(email, noteID)
	if err != nil {
		log.Printf("[AI 流水线] 读取笔记失败 %s: %v", noteID, err)
		return
	}

	if taskID != "" && note.CurrentTaskID != "" && note.CurrentTaskID != taskID {
		log.Printf("[AI 流水线] 任务 %s 已被新任务替代，放弃处理", taskID)
		return
	}

	initialRevision := note.Revision

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

	payload, err := h.promptMgr.BuildPayload(note.RawContent, refTime, userTagNames)
	if err != nil {
		log.Printf("[AI 流水线] 渲染提示词失败 %s: %v", noteID, err)
		h.markNoteFailed(email, noteID, taskID, "渲染提示词失败")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(h.cfg.AI.TimeoutSeconds)*time.Second)
	defer cancel()

	result, err := h.llmClient.ProcessNote(ctx, payload)
	if err != nil {
		log.Printf("[AI 流水线] 大模型调用失败 %s (内部错误已脱敏)", noteID)
		safeReason := "大模型服务处理失败，请稍后重试"
		if strings.Contains(err.Error(), "context deadline exceeded") {
			safeReason = "大模型调用超时，请稍后重试"
		} else if strings.Contains(err.Error(), "合法 JSON") || strings.Contains(err.Error(), "响应缺少") || strings.Contains(err.Error(), "空响应") {
			safeReason = "大模型响应格式解析异常，请稍后重试"
		}
		h.markNoteFailed(email, noteID, taskID, safeReason)
		return
	}

	// 使用独占锁 MutateNoteAtomic 进行原子写回，核对任务与版本防并发覆盖
	updatedNote, err := h.mdStore.MutateNoteAtomic(email, noteID, func(currentNote *storage.Note) error {
		// 校验任务 ID：若已被用户发起的重试等新任务替代，直接放弃写回
		if taskID != "" && currentNote.CurrentTaskID != taskID {
			return errors.New("任务已被新任务替代")
		}

		currentNote.ProcessingStatus = "ready"
		currentNote.FailureReason = ""

		// 如果用户在处理期间从未手动编辑（未修改正文、未调整/清空标签与待办）
		if !currentNote.UserEdited && currentNote.Revision == initialRevision {
			currentNote.PolishedContent = result.PolishedContent
			currentNote.Tags = result.Tags
			currentNote.Reminders = result.Reminders
		}
		return nil
	}, h.cfg.Location)

	if err != nil {
		log.Printf("[AI 流水线] 原子写入笔记跳过/失败 %s: %v", noteID, err)
		return
	}

	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(updatedNote, h.cfg.Location)
	}
}

func (h *NotesHandler) markNoteFailed(email, noteID, taskID, reason string) {
	updatedNote, err := h.mdStore.MutateNoteAtomic(email, noteID, func(currentNote *storage.Note) error {
		if taskID != "" && currentNote.CurrentTaskID != taskID {
			return errors.New("任务已被新任务替代")
		}
		currentNote.ProcessingStatus = "failed"
		currentNote.FailureReason = reason
		return nil
	}, h.cfg.Location)

	if err == nil {
		if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
			_ = idx.IndexNote(updatedNote, h.cfg.Location)
		}
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

// GetNote 获取单条笔记详情（严格校验 ID 隔离防目录穿越）
func (h *NotesHandler) GetNote(c *gin.Context) {
	email := c.GetString("user_email")
	noteID := c.Param("id")

	if err := storage.ValidateNoteID(noteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
		return
	}

	note, err := h.mdStore.ReadNote(email, noteID)
	if err != nil {
		if errors.Is(err, storage.ErrNoteNotFound) || errors.Is(err, storage.ErrInvalidNoteID) {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, note)
}

// UpdateNote 更新笔记内容或标签（原子锁中进行版本冲突校验）
func (h *NotesHandler) UpdateNote(c *gin.Context) {
	email := c.GetString("user_email")
	noteID := c.Param("id")

	if err := storage.ValidateNoteID(noteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
		return
	}

	var req UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "参数错误"}})
		return
	}

	updatedNote, err := h.mdStore.MutateNoteAtomic(email, noteID, func(note *storage.Note) error {
		// 版本冲突检查（在同个互斥锁保护下执行）
		if req.UpdatedAt != "" && note.UpdatedAt != req.UpdatedAt {
			return storage.ErrRevisionConflict
		}

		note.UserEdited = true
		if req.PolishedContent != nil {
			note.PolishedContent = *req.PolishedContent
		}
		if req.Tags != nil {
			note.Tags = *req.Tags
		}
		if req.Reminders != nil {
			note.Reminders = *req.Reminders
		}
		return nil
	}, h.cfg.Location)

	if err != nil {
		if errors.Is(err, storage.ErrRevisionConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "REVISION_CONFLICT", "message": "笔记已被修改，请刷新后重试"}})
			return
		}
		if errors.Is(err, storage.ErrNoteNotFound) || errors.Is(err, storage.ErrInvalidNoteID) {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}

	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(updatedNote, h.cfg.Location)
	}

	c.JSON(http.StatusOK, updatedNote)
}

// DeleteNote 删除笔记
func (h *NotesHandler) DeleteNote(c *gin.Context) {
	email := c.GetString("user_email")
	noteID := c.Param("id")

	if err := storage.ValidateNoteID(noteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
		return
	}

	if err := h.mdStore.DeleteNote(email, noteID); err != nil {
		if errors.Is(err, storage.ErrNoteNotFound) || errors.Is(err, storage.ErrInvalidNoteID) {
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

	if err := storage.ValidateNoteID(noteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
		return
	}

	newTaskID := storage.GenerateTaskID()
	updatedNote, err := h.mdStore.MutateNoteAtomic(email, noteID, func(note *storage.Note) error {
		if note.ProcessingStatus == "processing" {
			return errors.New("任务已在处理中")
		}
		note.ProcessingStatus = "processing"
		note.FailureReason = ""
		note.CurrentTaskID = newTaskID
		return nil
	}, h.cfg.Location)

	if err != nil {
		if err.Error() == "任务已在处理中" {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "TASK_RUNNING", "message": "该笔记已在处理中，请稍候"}})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
		return
	}

	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(updatedNote, h.cfg.Location)
	}

	go h.ProcessNoteAsync(email, noteID, newTaskID)

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

	if err := storage.ValidateNoteID(noteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
		return
	}

	var updatedReminders []storage.Reminder
	updatedNote, err := h.mdStore.MutateNoteAtomic(email, noteID, func(note *storage.Note) error {
		found := false
		for i := range note.Reminders {
			if note.Reminders[i].ID == remID {
				note.Reminders[i].Status = "completed"
				found = true
				break
			}
		}
		if !found {
			return errors.New("REMINDER_NOT_FOUND")
		}
		updatedReminders = note.Reminders
		note.UserEdited = true
		return nil
	}, h.cfg.Location)

	if err != nil {
		if err.Error() == "REMINDER_NOT_FOUND" {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "REMINDER_NOT_FOUND", "message": "待办提醒项不存在"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}

	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(updatedNote, h.cfg.Location)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "待办已标记完成",
		"note_id":    noteID,
		"rem_id":     remID,
		"updated_at": updatedNote.UpdatedAt,
		"reminders":  updatedReminders,
	})
}

// CancelReminder 取消单条待办提醒
func (h *NotesHandler) CancelReminder(c *gin.Context) {
	email := c.GetString("user_email")
	noteID := c.Param("id")
	remID := c.Param("rem_id")

	if err := storage.ValidateNoteID(noteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "笔记不存在"}})
		return
	}

	var updatedReminders []storage.Reminder
	updatedNote, err := h.mdStore.MutateNoteAtomic(email, noteID, func(note *storage.Note) error {
		found := false
		for i := range note.Reminders {
			if note.Reminders[i].ID == remID {
				note.Reminders[i].Status = "cancelled"
				found = true
				break
			}
		}
		if !found {
			return errors.New("REMINDER_NOT_FOUND")
		}
		updatedReminders = note.Reminders
		note.UserEdited = true
		return nil
	}, h.cfg.Location)

	if err != nil {
		if err.Error() == "REMINDER_NOT_FOUND" {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "REMINDER_NOT_FOUND", "message": "待办提醒项不存在"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "STORAGE_ERROR", "message": err.Error()}})
		return
	}

	if idx, err := h.indexMgr.GetUserIndex(email); err == nil {
		_ = idx.IndexNote(updatedNote, h.cfg.Location)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "待办已取消",
		"note_id":    noteID,
		"rem_id":     remID,
		"updated_at": updatedNote.UpdatedAt,
		"reminders":  updatedReminders,
	})
}
