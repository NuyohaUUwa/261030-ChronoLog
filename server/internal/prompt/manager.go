package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"chronolog/server/internal/llm"
)

type ModelSettings struct {
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"`
}

type ReminderExample struct {
	Title    string `yaml:"title" json:"title"`
	RemindAt string `yaml:"remind_at" json:"remind_at"`
	Reason   string `yaml:"reason" json:"reason"`
}

type ExampleOutput struct {
	PolishedContent string            `yaml:"polished_content" json:"polished_content"`
	Tags            []string          `yaml:"tags" json:"tags"`
	Reminders       []ReminderExample `yaml:"reminders" json:"reminders"`
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

	cfg := PromptConfig{ModelSettings: ModelSettings{Temperature: 0.2, MaxTokens: 1500}}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("解析提示词 YAML 失败: %w", err)
	}

	if err := validatePrompt(&cfg); err != nil {
		return err
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
	cfg := PromptConfig{ModelSettings: ModelSettings{Temperature: 0.2, MaxTokens: 1500}}
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return fmt.Errorf("提示词 YAML 格式不合法: %w", err)
	}

	// 校验必要字段
	if err := validatePrompt(&cfg); err != nil {
		return err
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

func validatePrompt(cfg *PromptConfig) error {
	if strings.TrimSpace(cfg.SystemPrompt) == "" || !strings.Contains(cfg.DynamicContextTemplate, "{{RAW_CONTENT}}") {
		return fmt.Errorf("提示词必须包含 system_prompt 和原稿占位符")
	}
	if cfg.ModelSettings.Temperature < 0 || cfg.ModelSettings.Temperature > 2 || cfg.ModelSettings.MaxTokens < 1 || cfg.ModelSettings.MaxTokens > 16000 {
		return fmt.Errorf("模型参数范围错误")
	}
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

// BuildPayload 构建包含完整模型参数、Few-Shot 消息序列与渲染后上下文的 Payload
func (p *PromptManager) BuildPayload(rawContent string, refTime time.Time, userTags []string) (*llm.PromptPayload, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.cfg == nil {
		return nil, fmt.Errorf("提示词配置未初始化")
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

	// 构造 FewShot 消息对 (Role: user / assistant)
	var fewShotMessages []llm.ChatMessage
	for _, eg := range p.cfg.FewShotExamples {
		if strings.TrimSpace(eg.Input) == "" {
			continue
		}
		egUserText := eg.Input
		if eg.ReferenceTime != "" {
			egUserText = fmt.Sprintf("【参考时间】：%s\n【随手记输入】：\n\"\"\"\n%s\n\"\"\"", eg.ReferenceTime, eg.Input)
		}
		outJSON, err := json.Marshal(eg.Output)
		if err != nil {
			continue
		}
		fewShotMessages = append(fewShotMessages,
			llm.ChatMessage{Role: "user", Content: egUserText},
			llm.ChatMessage{Role: "assistant", Content: string(outJSON)},
		)
	}

	temp := p.cfg.ModelSettings.Temperature
	if temp < 0 || temp > 2 {
		temp = 0.2
	}
	maxTokens := p.cfg.ModelSettings.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1500
	}

	return &llm.PromptPayload{
		SystemPrompt:    p.cfg.SystemPrompt,
		UserPrompt:      userBuf.String(),
		RawContent:      rawContent,
		FewShotMessages: fewShotMessages,
		Temperature:     temp,
		MaxTokens:       maxTokens,
	}, nil
}
