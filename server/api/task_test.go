package api

import (
	"chronolog/server/internal/config"
	"chronolog/server/internal/llm"
	"chronolog/server/internal/prompt"
	"chronolog/server/internal/storage"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type controlledAI struct {
	started chan struct{}
	release chan struct{}
	fail    bool
}

func (a *controlledAI) ProcessNote(ctx context.Context, p *llm.PromptPayload) (*llm.Result, error) {
	close(a.started)
	select {
	case <-a.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if a.fail {
		return nil, errors.New("PRIVATE_CANARY")
	}
	return &llm.Result{PolishedContent: "AI正文", Tags: []string{"AI标签"}, Reminders: []storage.Reminder{{ID: "r", Title: "AI提醒"}}}, nil
}

func TestTaskAndEditIsolation(t *testing.T) {
	for _, scenario := range []string{"old-success", "old-failure", "user-clear"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			loc := time.FixedZone("CST", 8*3600)
			md := storage.NewMarkdownStore(dir)
			cfg := &config.Config{Location: loc, AI: config.AIConfig{TimeoutSeconds: 5}}
			p := filepath.Join(dir, "prompts.yaml")
			os.WriteFile(p, []byte("system_prompt: test\ndynamic_context_template: '{{RAW_CONTENT}}'\n"), 0600)
			pm, err := prompt.NewPromptManager(p)
			if err != nil {
				t.Fatal(err)
			}
			ai := &controlledAI{started: make(chan struct{}), release: make(chan struct{}), fail: scenario == "old-failure"}
			indexMgr := storage.NewIndexManager(dir)
			defer indexMgr.Close()
			h := NewNotesHandler(cfg, md, indexMgr, ai, pm)
			n, err := md.SaveNoteRaw("a@test.local", "原稿", loc)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() { defer close(done); h.ProcessNoteAsync("a@test.local", n.ID, n.CurrentTaskID) }()
			<-ai.started
			_, err = md.MutateNoteAtomic("a@test.local", n.ID, func(n *storage.Note) error {
				if scenario == "user-clear" {
					n.UserEdited = true
					n.PolishedContent = ""
					n.Tags = []string{}
					n.Reminders = []storage.Reminder{}
				} else {
					n.CurrentTaskID = "new-task"
					n.ProcessingStatus = "ready"
					n.PolishedContent = "新任务正文"
				}
				return nil
			}, loc)
			if err != nil {
				t.Fatal(err)
			}
			close(ai.release)
			<-done
			got, err := md.ReadNote("a@test.local", n.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "user-clear" {
				if got.PolishedContent != "" || len(got.Tags) != 0 || len(got.Reminders) != 0 {
					t.Fatal("cleared fields overwritten")
				}
			} else if got.ProcessingStatus != "ready" || got.PolishedContent != "新任务正文" {
				t.Fatal("old task overwrote new task")
			}
			if got.RawContent != "原稿" {
				t.Fatal("raw changed")
			}
		})
	}
}
