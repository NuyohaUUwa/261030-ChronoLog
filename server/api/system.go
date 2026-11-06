package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"chronolog/server/internal/config"
	"chronolog/server/internal/storage"
)

type SystemHandler struct {
	cfg          *config.Config
	accountStore *storage.AccountStore
	mdStore      *storage.MarkdownStore
	indexMgr     *storage.IndexManager
}

func NewSystemHandler(cfg *config.Config, accountStore *storage.AccountStore, mdStore *storage.MarkdownStore, indexMgr *storage.IndexManager) *SystemHandler {
	return &SystemHandler{
		cfg:          cfg,
		accountStore: accountStore,
		mdStore:      mdStore,
		indexMgr:     indexMgr,
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
