package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUserIndex_FTS5AndCalendarSummary(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "chronolog_index_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	mgr := NewIndexManager(tempDir)
	email := "bob@example.com"
	loc, _ := time.LoadLocation("Asia/Shanghai")

	idx, err := mgr.GetUserIndex(email)
	if err != nil {
		t.Fatalf("GetUserIndex 失败: %v", err)
	}

	note1 := &Note{
		NoteFrontMatter: NoteFrontMatter{
			ID:               "20261106_100000_1111",
			CreatedAt:        "2026-11-06T10:00:00+08:00",
			UpdatedAt:        "2026-11-06T10:00:00+08:00",
			RawContent:       "今天给李总发了报价单",
			ProcessingStatus: "ready",
			Tags:             []string{"工作/商务", "项目/李总"},
			Reminders: []Reminder{
				{
					ID:                 "rem_01",
					Title:              "跟进李总报价单反馈",
					RemindAt:           "2026-11-07T14:00:00+08:00",
					Status:             "pending",
					NotificationStatus: "pending",
				},
			},
		},
		PolishedContent: "### 今日商务进展\n- 发送李总报价单",
	}

	note2 := &Note{
		NoteFrontMatter: NoteFrontMatter{
			ID:               "20261106_150000_2222",
			CreatedAt:        "2026-11-06T15:00:00+08:00",
			UpdatedAt:        "2026-11-06T15:00:00+08:00",
			RawContent:       "晚上去健身房跑步五公里",
			ProcessingStatus: "ready",
			Tags:             []string{"生活/健康"},
			Reminders:        []Reminder{},
		},
		PolishedContent: "今日跑步 5 公里打卡",
	}

	if err := idx.IndexNote(note1, loc); err != nil {
		t.Fatalf("IndexNote 1 失败: %v", err)
	}
	if err := idx.IndexNote(note2, loc); err != nil {
		t.Fatalf("IndexNote 2 失败: %v", err)
	}

	// 1. 全文检索 FTS5 测试
	notes, total, err := idx.QueryNotes(NoteFilter{Keyword: "报价单"})
	if err != nil {
		t.Fatalf("FTS5 检索失败: %v", err)
	}
	if total != 1 || len(notes) != 1 || notes[0].ID != note1.ID {
		t.Fatalf("FTS5 预期检索到 1 条笔记, 实际: %d", total)
	}

	// 1.1 短词检索测试 (1-2 字符)
	shortNotes, shortTotal, err := idx.QueryNotes(NoteFilter{Keyword: "报价"})
	if err != nil || shortTotal != 1 || len(shortNotes) != 1 {
		t.Fatalf("短词检索失败: %v, total: %d", err, shortTotal)
	}

	// 2. 标签检索测试
	tagNotes, totalTag, err := idx.QueryNotes(NoteFilter{Tag: "工作/商务"})
	if err != nil || totalTag != 1 {
		t.Fatalf("Tag 检索失败: %v, total: %d", err, totalTag)
	}
	if tagNotes[0].ID != note1.ID {
		t.Fatalf("Tag 检索匹配错误")
	}

	// 3. 日历聚合测试
	summary, err := idx.GetCalendarSummary(2026, 11, loc)
	if err != nil {
		t.Fatalf("GetCalendarSummary 失败: %v", err)
	}

	if len(summary.ActiveDates) != 1 || summary.ActiveDates[0] != "2026-11-06" {
		t.Fatalf("活跃打卡日期预期包含 2026-11-06, 得到: %v", summary.ActiveDates)
	}

	if len(summary.ReminderDates) != 1 || summary.ReminderDates[0].Date != "2026-11-07" {
		t.Fatalf("提醒日期预期包含 2026-11-07, 得到: %v", summary.ReminderDates)
	}
	if !summary.ReminderDates[0].HasPending {
		t.Fatalf("提醒日期应标注 has_pending=true")
	}

	// 4. 用户常用标签统计
	tagCounts, err := idx.GetUserTags()
	if err != nil {
		t.Fatalf("GetUserTags 失败: %v", err)
	}
	if len(tagCounts) != 3 {
		t.Fatalf("预期 3 个不重复标签, 得到: %d", len(tagCounts))
	}

	// 5. 索引重建验证（先删除物理文件索引，然后从内存对象全量恢复）
	if err := idx.RebuildIndex([]*Note{note1, note2}, loc); err != nil {
		t.Fatalf("RebuildIndex 失败: %v", err)
	}
	rebuiltNotes, rebuiltTotal, err := idx.QueryNotes(NoteFilter{})
	if err != nil || rebuiltTotal != 2 {
		t.Fatalf("重建索引后查询数量不符合预期: %v, %d", err, rebuiltTotal)
	}
	if len(rebuiltNotes) != 2 {
		t.Fatalf("重建索引返回的笔记切片长度不符合预期: %d", len(rebuiltNotes))
	}
	_ = filepath.Base(tempDir)
}
