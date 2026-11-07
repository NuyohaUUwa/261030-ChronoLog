package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRemotePayloadAndSafeErrors(t *testing.T) {
	for _, scenario := range []string{"valid", "http", "envelope", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer dummy" {
					t.Error("request contract")
				}
				var body chatCompletionRequest
				json.NewDecoder(r.Body).Decode(&body)
				if body.Temperature != nil || body.MaxTokens != 42 || len(body.Messages) != 4 {
					t.Error("model parameters or examples not sent")
				}
				switch scenario {
				case "http":
					w.WriteHeader(502)
					w.Write([]byte("PRIVATE_CANARY"))
				case "envelope":
					w.Write([]byte(`{"error":{"message":"PRIVATE_CANARY"}}`))
				case "oversized":
					w.Write([]byte(strings.Repeat("x", (1<<20)+1)))
				default:
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"polished_content":"ok","tags":[],"reminders":[]}`}}}})
				}
			}))
			defer srv.Close()
			c := NewRemoteClient(srv.URL+"/v1", "dummy", "fixture", time.Second)
			_, err := c.ProcessNote(context.Background(), &PromptPayload{SystemPrompt: "s", UserPrompt: "u", Temperature: 0, MaxTokens: 42, FewShotMessages: []ChatMessage{{Role: "user", Content: "eg"}, {Role: "assistant", Content: "answer"}}})
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || strings.Contains(err.Error(), "PRIVATE_CANARY") {
				t.Fatal("unsafe error or accepted invalid response")
			}
		})
	}
}

func TestRejectInvalidFields(t *testing.T) {
	for _, s := range []string{`null`, `{}`, `{"polished_content":""}`, `{"polished_content":"ok","tags":[],"reminders":[{"title":"x","remind_at":"2026-11-07T10:00:00"}]}`, `{"polished_content":"ok","tags":[],"reminders":[{"title":"x","remind_at":"not-date"}]}`} {
		if _, err := ParseAIResponse(s); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}

func TestDeepSeekThinkTagAndMultiLevelTags(t *testing.T) {
	sample := `<think>
用户想记录明天的会议，并加上多层级标签。
</think>
` + "```json\n" + `{
  "polished_content": "### 方案讨论会\n- 明天下午开会讨论新架构。",
  "tags": ["工作/项目/架构", "个人/待办"],
  "reminders": [
    {
      "title": "方案讨论会",
      "remind_at": "2026-11-08T15:00:00+08:00",
      "reason": "用户提及明天下午开会"
    }
  ]
}` + "\n```"

	res, err := ParseAIResponse(sample)
	if err != nil {
		t.Fatalf("ParseAIResponse failed for DeepSeek sample: %v", err)
	}
	if len(res.Tags) != 2 || res.Tags[0] != "工作/项目/架构" {
		t.Fatalf("unexpected tags: %+v", res.Tags)
	}
	if len(res.Reminders) != 1 || res.Reminders[0].Status != "pending" || res.Reminders[0].NotificationStatus != "pending" {
		t.Fatalf("unexpected reminders: %+v", res.Reminders)
	}
}
