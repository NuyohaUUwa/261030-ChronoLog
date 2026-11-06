package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Storage       StorageConfig       `yaml:"storage"`
	App           AppConfig           `yaml:"app"`
	Auth          AuthConfig          `yaml:"auth"`
	AI            AIConfig            `yaml:"ai"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Prompts       PromptsConfig       `yaml:"prompts"`

	// 计算字段
	Location *time.Location `yaml:"-"`
	RootDir  string         `yaml:"-"`
}

type ServerConfig struct {
	Address string `yaml:"address"`
}

type StorageConfig struct {
	DataDir string `yaml:"data_dir"`
}

type AppConfig struct {
	Timezone string `yaml:"timezone"`
}

type AuthConfig struct {
	JWTSecret        string   `yaml:"jwt_secret"`
	TokenExpiryHours int      `yaml:"token_expiry_hours"`
	AdminEmails      []string `yaml:"admin_emails"`
	AdminSetupKey    string   `yaml:"admin_setup_key"`
}

type AIConfig struct {
	Mode           string `yaml:"mode"` // mock | remote
	BaseURL        string `yaml:"base_url"`
	APIKey         string `yaml:"api_key"`
	Model          string `yaml:"model"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type NotificationsConfig struct {
	Mode                 string `yaml:"mode"` // mock | remote
	NtfyServer           string `yaml:"ntfy_server"`
	CheckIntervalSeconds int    `yaml:"check_interval_seconds"`
}

type PromptsConfig struct {
	File string `yaml:"file"`
}

// LoadConfig 从配置文件与环境变量加载配置
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Address: "127.0.0.1:8080",
		},
		Storage: StorageConfig{
			DataDir: "data-dev",
		},
		App: AppConfig{
			Timezone: "Asia/Shanghai",
		},
		Auth: AuthConfig{
			TokenExpiryHours: 720,
			AdminEmails:      []string{"434322037@qq.com"},
		},
		AI: AIConfig{
			Mode:           "mock",
			BaseURL:        "http://127.0.0.1:3000/v1",
			TimeoutSeconds: 30,
		},
		Notifications: NotificationsConfig{
			Mode:                 "mock",
			NtfyServer:           "https://ntfy.sh",
			CheckIntervalSeconds: 60,
		},
		Prompts: PromptsConfig{
			File: "config/prompts.local.yaml",
		},
	}

	// 查找配置文件
	configPath := os.Getenv("CHRONOLOG_CONFIG")
	if configPath == "" {
		candidates := []string{
			"config/config.local.yaml",
			"../config/config.local.yaml",
			"config/config.yaml",
			"../config/config.yaml",
			"config/config.example.yaml",
			"../config/config.example.yaml",
		}
		for _, cand := range candidates {
			if _, err := os.Stat(cand); err == nil {
				configPath = cand
				break
			}
		}
	}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败 (%s): %w", configPath, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件 YAML 失败: %w", err)
		}
	}

	// 环境变量覆盖 (Env > YAML > Default)
	if val := os.Getenv("HTTP_ADDR"); val != "" {
		cfg.Server.Address = val
	}
	if val := os.Getenv("DATA_DIR"); val != "" {
		cfg.Storage.DataDir = val
	}
	if val := os.Getenv("APP_TIMEZONE"); val != "" {
		cfg.App.Timezone = val
	}
	if val := os.Getenv("JWT_SECRET"); val != "" {
		cfg.Auth.JWTSecret = val
	}
	if val := os.Getenv("ADMIN_EMAILS"); val != "" {
		cfg.Auth.AdminEmails = parseCommaSeparated(val)
	}
	if val := os.Getenv("ADMIN_SETUP_KEY"); val != "" {
		cfg.Auth.AdminSetupKey = strings.TrimSpace(val)
	}
	if val := os.Getenv("AI_MODE"); val != "" {
		cfg.AI.Mode = strings.ToLower(val)
	}
	if val := os.Getenv("AI_API_BASE"); val != "" {
		cfg.AI.BaseURL = val
	}
	if val := os.Getenv("AI_API_KEY"); val != "" {
		cfg.AI.APIKey = val
	}
	if val := os.Getenv("AI_MODEL"); val != "" {
		cfg.AI.Model = val
	}
	if val := os.Getenv("NOTIFICATION_MODE"); val != "" {
		cfg.Notifications.Mode = strings.ToLower(val)
	}
	if val := os.Getenv("PROMPTS_FILE"); val != "" {
		cfg.Prompts.File = val
	}

	// 校验必需项
	if strings.TrimSpace(cfg.Auth.JWTSecret) == "" {
		return nil, fmt.Errorf("安全错误: JWT_SECRET 必须配置，且不能为空")
	}
	if strings.Contains(cfg.Auth.JWTSecret, "replace-with") || strings.Contains(cfg.Auth.JWTSecret, "placeholder") {
		return nil, fmt.Errorf("安全错误: 不能使用占位符作为 JWT_SECRET")
	}

	// 时区加载
	cfg.AI.Mode = strings.ToLower(strings.TrimSpace(cfg.AI.Mode))
	cfg.Notifications.Mode = strings.ToLower(strings.TrimSpace(cfg.Notifications.Mode))
	if cfg.AI.Mode != "mock" && cfg.AI.Mode != "remote" {
		return nil, fmt.Errorf("AI_MODE 必须是 mock 或 remote")
	}
	if cfg.Notifications.Mode != "mock" && cfg.Notifications.Mode != "remote" {
		return nil, fmt.Errorf("NOTIFICATION_MODE 必须是 mock 或 remote")
	}
	if cfg.AI.TimeoutSeconds <= 0 {
		return nil, fmt.Errorf("AI 超时必须大于零")
	}
	if cfg.AI.Mode == "remote" {
		u, parseErr := url.Parse(cfg.AI.BaseURL)
		if parseErr != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("AI 地址必须为无凭据及查询参数的 HTTP(S) 地址")
		}
		if strings.TrimSpace(cfg.AI.Model) == "" {
			return nil, fmt.Errorf("remote 模式必须配置 AI_MODEL")
		}
	}
	loc, err := time.LoadLocation(cfg.App.Timezone)
	if err != nil {
		return nil, fmt.Errorf("无效的时区配置 (%s): %w", cfg.App.Timezone, err)
	}
	cfg.Location = loc

	// 将相对路径规范化
	if !filepath.IsAbs(cfg.Storage.DataDir) {
		cwd, _ := os.Getwd()
		// 如果在 server 目录下运行，将上级项目根目录作为基准
		if filepath.Base(cwd) == "server" {
			cfg.Storage.DataDir = filepath.Join(filepath.Dir(cwd), cfg.Storage.DataDir)
		} else {
			cfg.Storage.DataDir = filepath.Join(cwd, cfg.Storage.DataDir)
		}
	}

	if !filepath.IsAbs(cfg.Prompts.File) {
		cwd, _ := os.Getwd()
		if filepath.Base(cwd) == "server" {
			cfg.Prompts.File = filepath.Join(filepath.Dir(cwd), cfg.Prompts.File)
		} else {
			cfg.Prompts.File = filepath.Join(cwd, cfg.Prompts.File)
		}
	}

	return cfg, nil
}

// IsAdmin 检查指定邮箱是否在管理员列表中
func (c *Config) IsAdmin(email string) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	for _, admin := range c.Auth.AdminEmails {
		if strings.ToLower(strings.TrimSpace(admin)) == normalized {
			return true
		}
	}
	return false
}

func parseCommaSeparated(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
