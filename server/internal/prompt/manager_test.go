package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPayloadUsesModelSettingsAndExampleJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p.yaml")
	os.WriteFile(p, []byte(`system_prompt: test
dynamic_context_template: '{{CURRENT_DATETIME}} {{RAW_CONTENT}}'
model_settings:
  temperature: 0
  max_tokens: 42
few_shot_examples:
  - reference_time: '2026-11-01T10:00:00+08:00'
    input: example
    output:
      polished_content: answer
      tags: []
      reminders: []
`), 0600)
	pm, err := NewPromptManager(p)
	if err != nil {
		t.Fatal(err)
	}
	v, err := pm.BuildPayload("raw", time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Temperature != 0 || v.MaxTokens != 42 || len(v.FewShotMessages) != 2 || !strings.Contains(v.FewShotMessages[1].Content, `"polished_content"`) || !strings.Contains(v.FewShotMessages[0].Content, "2026-11-01") {
		t.Fatal("wrong payload")
	}
}
