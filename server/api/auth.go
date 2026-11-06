package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"chronolog/server/internal/auth"
	"chronolog/server/internal/config"
	"chronolog/server/internal/storage"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type AuthHandler struct {
	cfg          *config.Config
	accountStore *storage.AccountStore
}

func NewAuthHandler(cfg *config.Config, accountStore *storage.AccountStore) *AuthHandler {
	return &AuthHandler{
		cfg:          cfg,
		accountStore: accountStore,
	}
}

type AuthRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	AdminKey string `json:"admin_key,omitempty"`
}

type AuthResponse struct {
	Token             string `json:"token"`
	Email             string `json:"email"`
	IsAdmin           bool   `json:"is_admin"`
	NtfyTopic         string `json:"ntfy_topic"`
	AIMode            string `json:"ai_mode,omitempty"`
	NotificationsMode string `json:"notifications_mode,omitempty"`
}

func generateRandomTopic() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("topic_%s", hex.EncodeToString(b))
}

// Register 处理用户注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "邮箱与密码不能为空"}})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !emailRegex.MatchString(email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_EMAIL", "message": "邮箱格式不正确"}})
		return
	}

	// 针对管理员候选邮箱增加初始化密钥校验，防止公开注册抢注管理员
	isAdminCandidate := h.cfg.IsAdmin(email)
	if isAdminCandidate {
		expectedKey := strings.TrimSpace(h.cfg.Auth.AdminSetupKey)
		if expectedKey == "" || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.AdminKey)), []byte(expectedKey)) != 1 {
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"code":    "ADMIN_KEY_REQUIRED",
					"message": "该邮箱属于系统管理员名单，注册时必须提供有效的管理员初始化密钥",
				},
			})
			return
		}
	}

	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "WEAK_PASSWORD", "message": "密码长度不能少于 6 位"}})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "SERVER_ERROR", "message": "密码加密失败"}})
		return
	}

	topic := generateRandomTopic()
	user, err := h.accountStore.CreateUser(email, hash, topic)
	if err != nil {
		if err == storage.ErrUserExists {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "USER_EXISTS", "message": "该邮箱已注册，请直接登录"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "SERVER_ERROR", "message": "创建用户失败: " + err.Error()}})
		return
	}

	isAdmin := h.cfg.IsAdmin(user.Email)
	token, err := auth.GenerateToken(user.Email, isAdmin, h.cfg.Auth.JWTSecret, h.cfg.Auth.TokenExpiryHours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "SERVER_ERROR", "message": "签发 Token 失败"}})
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		Token:             token,
		Email:             user.Email,
		IsAdmin:           isAdmin,
		NtfyTopic:         user.NtfyTopic,
		AIMode:            h.cfg.AI.Mode,
		NotificationsMode: h.cfg.Notifications.Mode,
	})
}

// Login 处理用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "邮箱与密码不能为空"}})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	user, err := h.accountStore.GetUserByEmail(email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "INVALID_CREDENTIALS", "message": "账号或密码错误"}})
		return
	}

	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "INVALID_CREDENTIALS", "message": "账号或密码错误"}})
		return
	}

	isAdmin := h.cfg.IsAdmin(user.Email)
	token, err := auth.GenerateToken(user.Email, isAdmin, h.cfg.Auth.JWTSecret, h.cfg.Auth.TokenExpiryHours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "SERVER_ERROR", "message": "签发 Token 失败"}})
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		Token:             token,
		Email:             user.Email,
		IsAdmin:           isAdmin,
		NtfyTopic:         user.NtfyTopic,
		AIMode:            h.cfg.AI.Mode,
		NotificationsMode: h.cfg.Notifications.Mode,
	})
}

// GetMe 获取当前登录用户信息
func (h *AuthHandler) GetMe(c *gin.Context) {
	email := c.GetString("user_email")
	user, err := h.accountStore.GetUserByEmail(email)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "USER_NOT_FOUND", "message": "用户不存在"}})
		return
	}

	isAdmin := h.cfg.IsAdmin(user.Email)
	c.JSON(http.StatusOK, gin.H{
		"email":              user.Email,
		"is_admin":           isAdmin,
		"ntfy_topic":         user.NtfyTopic,
		"created_at":         user.CreatedAt,
		"ai_mode":            h.cfg.AI.Mode,
		"notifications_mode": h.cfg.Notifications.Mode,
	})
}

// UpdateSettings 更新设置
func (h *AuthHandler) UpdateSettings(c *gin.Context) {
	email := c.GetString("user_email")
	var req struct {
		NtfyTopic string `json:"ntfy_topic"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "参数错误"}})
		return
	}

	topic := strings.TrimSpace(req.NtfyTopic)
	if topic == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_PARAMS", "message": "ntfy_topic 不能为空"}})
		return
	}

	if err := h.accountStore.UpdateUserTopic(email, topic); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "SERVER_ERROR", "message": "更新配置失败"}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功", "ntfy_topic": topic})
}
