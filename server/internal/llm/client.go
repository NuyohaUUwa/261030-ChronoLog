package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"chronolog/server/internal/storage"
)

type Result struct {
	PolishedContent string             `json:"polished_content"`
	Tags            []string           `json:"tags"`
	Reminders       []storage.Reminder `json:"reminders"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type PromptPayload struct {
	SystemPrompt    string        `json:"system_prompt"`
	UserPrompt      string        `json:"user_prompt"`
	RawContent      string        `json:"raw_content"`
	FewShotMessages []ChatMessage `json:"few_shot_messages,omitempty"`
	Temperature     float64       `json:"temperature"`
	MaxTokens       int           `json:"max_tokens"`
}

type Client interface {
	ProcessNote(ctx context.Context, payload *PromptPayload) (*Result, error)
}

// MockClient 极简 Mock 模式客户端：满足用户决定的“留空待填，空标签与空提醒”策略
type MockClient struct{}

func NewMockClient() *MockClient {
	return &MockClient{}
}

func (m *MockClient) ProcessNote(ctx context.Context, payload *PromptPayload) (*Result, error) {
	// 如果用户输入带有 [fail] 前缀，模拟 AI 服务故障，以验证原稿保护与重试机制
	if strings.HasPrefix(strings.TrimSpace(payload.RawContent), "[fail]") {
		return nil, errors.New("模拟错误: AI 处理失败，原始笔记已安全留存")
	}

	return &Result{
		PolishedContent: strings.TrimSpace(payload.RawContent),
		Tags:            []string{},
		Reminders:       []storage.Reminder{},
	}, nil
}

// DynamicClient 支持并发安全地热切换底层的 Client 实例
type DynamicClient struct {
	mu     sync.RWMutex
	client Client
}

func NewDynamicClient(initial Client) *DynamicClient {
	return &DynamicClient{client: initial}
}

func (d *DynamicClient) Switch(newClient Client) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.client = newClient
}

func (d *DynamicClient) ProcessNote(ctx context.Context, payload *PromptPayload) (*Result, error) {
	d.mu.RLock()
	c := d.client
	d.mu.RUnlock()
	return c.ProcessNote(ctx, payload)
}

// RemoteClient 调用符合 OpenAI 协议的远程大模型服务
type RemoteClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	slots      chan struct{}
}

func NewRemoteClient(baseURL, apiKey, model string, timeout time.Duration) *RemoteClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &RemoteClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		slots:   make(chan struct{}, 4),
		httpClient: &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func isReasoningModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "flash") ||
		strings.Contains(lower, "reasoner") ||
		strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3")
}

func (r *RemoteClient) ProcessNote(ctx context.Context, payload *PromptPayload) (*Result, error) {
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	default:
		return nil, errors.New("AI 服务繁忙，请稍后重试")
	}
	var messages []ChatMessage
	if strings.TrimSpace(payload.SystemPrompt) != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: payload.SystemPrompt})
	}
	messages = append(messages, payload.FewShotMessages...)
	messages = append(messages, ChatMessage{Role: "user", Content: payload.UserPrompt})

	var tempPtr *float64
	if !isReasoningModel(r.model) && payload.Temperature > 0 && payload.Temperature <= 2 {
		temp := payload.Temperature
		tempPtr = &temp
	}
	maxTokens := payload.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1500
	}

	reqBody := chatCompletionRequest{
		Model:       r.model,
		Messages:    messages,
		Temperature: tempPtr,
		MaxTokens:   maxTokens,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("构造 AI 请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", r.baseURL)

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*500) * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if r.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+r.apiKey)
		}

		resp, err := r.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = errors.New("请求 AI 服务失败")
			continue
		}

		// 限制最多读取 1MB 响应内容，防止大响应耗尽内存
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		resp.Body.Close()
		if err != nil {
			lastErr = errors.New("读取 AI 响应失败")
			continue
		}
		if len(bodyBytes) > 1<<20 {
			return nil, errors.New("AI 响应超过大小限制")
		}

		if resp.StatusCode == http.StatusInternalServerError || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout {
			// 上游服务端暂时过载或并发抖动，记录并重试
			lastErr = fmt.Errorf("AI 服务临时异常 (HTTP %d)", resp.StatusCode)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			// 严密脱敏：绝不在错误消息中泄露上游响应原文、网络私密信息或 Token
			return nil, fmt.Errorf("AI 服务响应异常 (HTTP %d)", resp.StatusCode)
		}

		var chatResp chatCompletionResponse
		if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
			return nil, fmt.Errorf("解析 AI 响应 JSON 失败")
		}

		if chatResp.Error != nil && chatResp.Error.Message != "" {
			return nil, errors.New("AI 服务返回错误")
		}

		if len(chatResp.Choices) == 0 {
			return nil, errors.New("AI 未返回任何候选响应")
		}

		rawReply := strings.TrimSpace(chatResp.Choices[0].Message.Content)
		return ParseAIResponse(rawReply)
	}

	return nil, lastErr
}

type rawAIOutput struct {
	PolishedContent *string   `json:"polished_content"`
	Tags            *[]string `json:"tags"`
	Reminders       *[]struct {
		Title    string `json:"title"`
		RemindAt string `json:"remind_at"`
		Reason   string `json:"reason"`
	} `json:"reminders"`
}

// ParseAIResponse 清理可能的 Markdown 围栏并解析纯 JSON 结果
func ParseAIResponse(raw string) (*Result, error) {
	cleaned := strings.TrimSpace(raw)
	// 如果包含 <think>...</think> 思考过程标签（如 DeepSeek-R1 / 兼容推理模型），剔除思考过程保留最终 JSON
	if idx := strings.Index(cleaned, "</think>"); idx != -1 {
		cleaned = strings.TrimSpace(cleaned[idx+len("</think>"):])
	}

	// 去除可能携带的 ```json 或 ``` 围栏
	if strings.HasPrefix(cleaned, "```") {
		idx := strings.Index(cleaned, "\n")
		if idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		if strings.HasSuffix(cleaned, "```") {
			cleaned = cleaned[:len(cleaned)-3]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	if cleaned == "" || cleaned == "null" {
		return nil, fmt.Errorf("大模型返回空响应")
	}

	var out rawAIOutput
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		// 脱敏：不将原始 raw 拼接入错误字符串
		return nil, errors.New("大模型输出不是合法 JSON")
	}

	if out.PolishedContent == nil || strings.TrimSpace(*out.PolishedContent) == "" || out.Tags == nil || out.Reminders == nil {
		return nil, errors.New("大模型响应缺少有效正文、标签或提醒字段")
	}
	if len(*out.PolishedContent) > 200000 || len(*out.Tags) > 32 || len(*out.Reminders) > 64 {
		return nil, errors.New("AI 字段超过大小限制")
	}

	var tags []string
	if out.Tags != nil {
		seen := make(map[string]bool)
		for _, t := range *out.Tags {
			trimmed := strings.TrimSpace(t)
			if len(trimmed) > 200 {
				return nil, errors.New("标签过长")
			}
			if trimmed != "" && !seen[trimmed] {
				seen[trimmed] = true
				tags = append(tags, trimmed)
			}
		}
	}
	if tags == nil {
		tags = []string{}
	}

	var reminders []storage.Reminder
	if out.Reminders != nil {
		for _, r := range *out.Reminders {
			title := strings.TrimSpace(r.Title)
			remindAt := strings.TrimSpace(r.RemindAt)
			if title == "" || remindAt == "" {
				return nil, errors.New("提醒缺少标题或时间")
			}
			if len(title) > 1000 || len(r.Reason) > 2000 {
				return nil, errors.New("提醒字段过长")
			}
			// 严格校验必须为带时区偏移的合法 RFC 3339 格式
			parsedTime, err := time.Parse(time.RFC3339, remindAt)
			if err != nil {
				// 若不是带时区的标准 RFC 3339，跳过该非法项
				return nil, errors.New("提醒时间必须为带偏移量的 RFC3339")
			}

			reminders = append(reminders, storage.Reminder{
				ID:                 storage.GenerateReminderID(), // 强制由服务端生成全局唯一 ID
				Title:              title,
				RemindAt:           parsedTime.Format(time.RFC3339),
				Status:             "pending", // 强制初始化，禁止大模型指定 completed
				NotificationStatus: "pending", // 强制初始化，禁止大模型指定 sent
				Reason:             strings.TrimSpace(r.Reason),
			})
		}
	}
	if reminders == nil {
		reminders = []storage.Reminder{}
	}

	return &Result{
		PolishedContent: *out.PolishedContent,
		Tags:            tags,
		Reminders:       reminders,
	}, nil
}

// TestRawConnection 用于轻量级连通性测试
func TestRawConnection(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	reqBody := chatCompletionRequest{
		Model: model,
		Messages: []ChatMessage{
			{Role: "user", Content: "你好，请回复五个字：连通测试成功"},
		},
		MaxTokens: 50,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(baseURL, "/"))

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt*500) * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = errors.New("连接 AI 服务失败")
			continue
		}

		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
		resp.Body.Close()
		if err != nil {
			lastErr = errors.New("读取 AI 服务响应失败")
			continue
		}

		if resp.StatusCode == http.StatusInternalServerError || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout {
			lastErr = fmt.Errorf("AI 服务临时繁忙 (HTTP %d)", resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("AI 服务返回异常状态 (HTTP %d)", resp.StatusCode)
		}

		var chatResp chatCompletionResponse
		if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
			return "", errors.New("解析响应 JSON 失败")
		}
		if len(chatResp.Choices) == 0 {
			return "", errors.New("AI 服务未返回候选内容")
		}
		return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
	}
	return "", lastErr
}
