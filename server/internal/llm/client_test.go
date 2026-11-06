package llm

import (
	"strings"
	"testing"
)

func TestParseAIResponse_StrictValidation(t *testing.T) {
	// 1. 测试 null 或空响应拦截
	if _, err := ParseAIResponse("null"); err == nil {
		t.Error("预期 null 响应报错，但解析成功")
	}
	if _, err := ParseAIResponse(""); err == nil {
		t.Error("预期空响应报错，但解析成功")
	}
	if _, err := ParseAIResponse("   "); err == nil {
		t.Error("预期纯空白响应报错，但解析成功")
	}

	// 2. 缺少 polished_content 报错
	invalidJSON := `{"tags": ["工作"]}`
	if _, err := ParseAIResponse(invalidJSON); err == nil {
		t.Error("缺少 polished_content 应当报错")
	}

	// 3. 错误信息脱敏：不泄露包含敏感 canary 的原始字符串
	canaryInput := `{"invalid_json": REVIEW_PRIVATE_CANARY}`
	_, err := ParseAIResponse(canaryInput)
	if err == nil {
		t.Fatal("非法 JSON 应当报错")
	}
	if strings.Contains(err.Error(), "REVIEW_PRIVATE_CANARY") {
		t.Fatalf("错误信息泄露了未脱敏的原文敏感信息: %s", err.Error())
	}

	// 4. 合法响应与服务端强制状态覆盖 (防模型篡改完成状态)
	validJSON := `{
		"polished_content": "### 会议纪要\n- 讨论了方案",
		"tags": ["工作/方案", "工作/方案", "  "],
		"reminders": [
			{
				"id": "model_evil_id",
				"title": "提交报告",
				"remind_at": "2026-11-08T10:00:00+08:00",
				"status": "completed",
				"notification_status": "sent",
				"reason": "明天截止"
			}
		]
	}`

	res, err := ParseAIResponse(validJSON)
	if err != nil {
		t.Fatalf("合法 JSON 解析失败: %v", err)
	}

	// 验证 tags 去重且去除空白
	if len(res.Tags) != 1 || res.Tags[0] != "工作/方案" {
		t.Errorf("Tags 去重或清洗失败: %v", res.Tags)
	}

	// 验证合法提醒被保留；非法日期由 TestRejectInvalidFields 验证拒绝
	if len(res.Reminders) != 1 {
		t.Fatalf("预期只保留 1 条合法提醒，实际得到 %d 条", len(res.Reminders))
	}

	rem := res.Reminders[0]
	// 验证服务端强制生成 ID，不信任大模型的 ID
	if rem.ID == "model_evil_id" || !strings.HasPrefix(rem.ID, "rem_") {
		t.Errorf("大模型指定的 ID 应被服务端重写覆盖, 得到: %s", rem.ID)
	}

	// 验证服务端强制 pending 状态，不允许大模型指定 completed 或 sent
	if rem.Status != "pending" {
		t.Errorf("提醒初始状态必须为 pending, 得到: %s", rem.Status)
	}
	if rem.NotificationStatus != "pending" {
		t.Errorf("推送初始状态必须为 pending, 得到: %s", rem.NotificationStatus)
	}
}
