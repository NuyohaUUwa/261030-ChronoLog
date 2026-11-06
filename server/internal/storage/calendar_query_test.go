package storage

import (
	"testing"
	"time"
)

func TestReminderDateUsesTimezoneAndNotTitle(t *testing.T) {
	mgr := NewIndexManager(t.TempDir())
	idx, err := mgr.GetUserIndex("a@test.local")
	if err != nil {
		t.Fatal(err)
	}
	defer idx.db.Close()
	loc := time.FixedZone("CST", 8*3600)
	n := &Note{NoteFrontMatter: NoteFrontMatter{ID: "test", CreatedAt: "2026-11-01T10:00:00+08:00", UpdatedAt: "2026-11-01T10:00:00+08:00", Tags: []string{}, Reminders: []Reminder{{Title: "2026-11-09", RemindAt: "2026-11-06T18:00:00Z"}}}}
	if err := idx.IndexNote(n, loc); err != nil {
		t.Fatal(err)
	}
	for day, want := range map[string]int{"2026-11-07": 1, "2026-11-06": 0, "2026-11-09": 0} {
		_, total, err := idx.QueryNotes(NoteFilter{Date: day})
		if err != nil || total != want {
			t.Fatalf("%s: %d %v", day, total, err)
		}
	}
	n.Reminders = []Reminder{}
	n.PolishedContent = "外部修改"
	if err := idx.RebuildIndex([]*Note{n}, loc); err != nil {
		t.Fatal(err)
	}
	_, count, _ := idx.QueryNotes(NoteFilter{Date: "2026-11-07"})
	if count != 0 {
		t.Fatal("stale reminder date")
	}
	_, count, _ = idx.QueryNotes(NoteFilter{Keyword: "外部修改"})
	if count != 1 {
		t.Fatal("stale content")
	}
}
