package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"chronolog/server/internal/config"
	"chronolog/server/internal/storage"
)

type CalendarHandler struct {
	cfg      *config.Config
	indexMgr *storage.IndexManager
}

func NewCalendarHandler(cfg *config.Config, indexMgr *storage.IndexManager) *CalendarHandler {
	return &CalendarHandler{
		cfg:      cfg,
		indexMgr: indexMgr,
	}
}

// GetSummary 获取双模日历月度摘要（历史活跃打卡日与未来提醒日）
func (h *CalendarHandler) GetSummary(c *gin.Context) {
	email := c.GetString("user_email")
	idx, err := h.indexMgr.GetUserIndex(email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INDEX_ERROR", "message": err.Error()}})
		return
	}

	now := time.Now().In(h.cfg.Location)
	year, _ := strconv.Atoi(c.DefaultQuery("year", strconv.Itoa(now.Year())))
	month, _ := strconv.Atoi(c.DefaultQuery("month", strconv.Itoa(int(now.Month()))))

	if month < 1 || month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "月份必须在 1-12 之间"}})
		return
	}

	summary, err := idx.GetCalendarSummary(year, month, h.cfg.Location)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "QUERY_ERROR", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// GetTags 获取当前用户的所有常用标签库及引用频次
func (h *CalendarHandler) GetTags(c *gin.Context) {
	email := c.GetString("user_email")
	idx, err := h.indexMgr.GetUserIndex(email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INDEX_ERROR", "message": err.Error()}})
		return
	}

	tags, err := idx.GetUserTags()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "QUERY_ERROR", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tags": tags})
}
