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
	"time"

	"chronolog/server/internal/storage"
)

type Result struct {
	PolishedContent string             `json:"polished_content"`
	Tags            []string           `json:"tags"`
	Reminders       []storage.Reminder `json:"reminders"`
}

type Client interface {
	ProcessNote(ctx context.Context, systemPrompt, userPrompt, rawContent string) (*Result, error)
}

// MockClient 极简 Mock 模式客户端：满足用户决定的“留空待填，空标签与空提醒”策略
type MockClient struct{}

func NewMockClient() *MockClient {
	return &MockClient{}
}

func (m *MockClient) ProcessNote(ctx context.Context, systemPrompt, userPrompt, rawContent string) (*Result, error) {
	// 如果用户输入带有 [fail] 前缀，模拟 AI 服务故障，以验证原稿保护与重试机制
	if strings.HasPrefix(strings.TrimSpace(rawContent), "[fail]") {
		return nil, errors.New("模拟错误: AI 处理失败，原始笔记已安全留存")
	}

	return &Result{
		PolishedContent: strings.TrimSpace(rawContent),
		Tags:            []string{},
		Reminders:       []storage.Reminder{},
	}, nil
}

// RemoteClient 调用符合 OpenAI 协议的远程大模型服务
type RemoteClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewRemoteClient(baseURL, apiKey, model string, timeout time.Duration) *RemoteClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &RemoteClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
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

func (r *RemoteClient) ProcessNote(ctx context.Context, systemPrompt, userPrompt, rawContent string) (*Result, error) {
	reqBody := chatCompletionRequest{
		Model: r.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("构造 AI 请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", r.baseURL)
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
		return nil, fmt.Errorf("请求 AI 服务失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 AI 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI 服务返回异常状态码 (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 AI 响应 JSON 失败: %w", err)
	}

	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return nil, fmt.Errorf("AI 服务报错: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, errors.New("AI 未返回任何候选响应")
	}

	rawReply := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	return ParseAIResponse(rawReply)
}

// ParseAIResponse 清理可能的 Markdown 围栏并解析纯 JSON 结果
func ParseAIResponse(raw string) (*Result, error) {
	cleaned := strings.TrimSpace(raw)
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

	var res Result
	if err := json.Unmarshal([]byte(cleaned), &res); err != nil {
		return nil, fmt.Errorf("大模型输出不是合法 JSON: %w (原始输出: %s)", err, raw)
	}

	if res.Tags == nil {
		res.Tags = []string{}
	}
	if res.Reminders == nil {
		res.Reminders = []storage.Reminder{}
	}

	// 校验并补充提醒字段
	for i := range res.Reminders {
		if res.Reminders[i].ID == "" {
			res.Reminders[i].ID = fmt.Sprintf("rem_%02d", i+1)
		}
		if res.Reminders[i].Status == "" {
			res.Reminders[i].Status = "pending"
		}
		if res.Reminders[i].NotificationStatus == "" {
			res.Reminders[i].NotificationStatus = "pending"
		}
	}

	return &res, nil
}
