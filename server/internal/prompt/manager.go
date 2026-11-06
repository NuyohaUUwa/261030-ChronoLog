package prompt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type ModelSettings struct {
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"`
}

type ReminderExample struct {
	Title    string `yaml:"title"`
	RemindAt string `yaml:"remind_at"`
	Reason   string `yaml:"reason"`
}

type ExampleOutput struct {
	PolishedContent string            `yaml:"polished_content"`
	Tags            []string          `yaml:"tags"`
	Reminders       []ReminderExample `yaml:"reminders"`
}

type FewShotExample struct {
	ReferenceTime string        `yaml:"reference_time"`
	Input         string        `yaml:"input"`
	Output        ExampleOutput `yaml:"output"`
}

type PromptConfig struct {
	Version                string           `yaml:"version"`
	ModelSettings          ModelSettings    `yaml:"model_settings"`
	SystemPrompt           string           `yaml:"system_prompt"`
	DynamicContextTemplate string           `yaml:"dynamic_context_template"`
	FewShotExamples        []FewShotExample `yaml:"few_shot_examples"`
	Constraints            string           `yaml:"constraints"`
}

type PromptManager struct {
	filePath string
	mu       sync.RWMutex
	cfg      *PromptConfig
}

func NewPromptManager(filePath string) (*PromptManager, error) {
	pm := &PromptManager{filePath: filePath}
	if err := pm.Reload(); err != nil {
		return nil, err
	}
	return pm, nil
}

// Reload 从磁盘重新加载提示词配置
func (p *PromptManager) Reload() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := os.ReadFile(p.filePath)
	if err != nil {
		return fmt.Errorf("读取提示词文件失败: %w", err)
	}

	var cfg PromptConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("解析提示词 YAML 失败: %w", err)
	}

	p.cfg = &cfg
	return nil
}

// GetRawYAML 读取原始 YAML 字符串
func (p *PromptManager) GetRawYAML() (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	data, err := os.ReadFile(p.filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UpdateRawYAML 校验并原子写入新的提示词 YAML
func (p *PromptManager) UpdateRawYAML(content string) error {
	// 先做反序列化校验
	var cfg PromptConfig
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return fmt.Errorf("提示词 YAML 格式不合法: %w", err)
	}

	// 校验必要字段
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		return fmt.Errorf("system_prompt 不能为空")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	dir := filepath.Dir(p.filePath)
	tmpPath := filepath.Join(dir, fmt.Sprintf(".prompts_%d.tmp", time.Now().UnixNano()))
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入提示词临时文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, p.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("替换提示词文件失败: %w", err)
	}

	p.cfg = &cfg
	return nil
}

// RenderPrompt 根据输入笔记、参考基准时间与现有标签渲染提示词
func (p *PromptManager) RenderPrompt(rawContent string, refTime time.Time, userTags []string) (systemPrompt, userPrompt string, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.cfg == nil {
		return "", "", fmt.Errorf("提示词配置未初始化")
	}

	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	weekdayStr := weekdays[refTime.Weekday()]

	// 渲染动态上下文
	contextText := p.cfg.DynamicContextTemplate
	contextText = strings.ReplaceAll(contextText, "{{CURRENT_DATETIME}}", refTime.Format(time.RFC3339))
	contextText = strings.ReplaceAll(contextText, "{{CURRENT_WEEKDAY}}", weekdayStr)

	tagsStr := "暂无"
	if len(userTags) > 0 {
		tagsStr = strings.Join(userTags, ", ")
	}
	contextText = strings.ReplaceAll(contextText, "{{USER_TAGS}}", tagsStr)
	contextText = strings.ReplaceAll(contextText, "{{RAW_CONTENT}}", rawContent)

	var userBuf bytes.Buffer
	userBuf.WriteString(contextText)
	userBuf.WriteString("\n\n")

	if strings.TrimSpace(p.cfg.Constraints) != "" {
		userBuf.WriteString(p.cfg.Constraints)
		userBuf.WriteString("\n")
	}

	return p.cfg.SystemPrompt, userBuf.String(), nil
}
