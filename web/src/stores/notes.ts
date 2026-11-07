import { defineStore } from 'pinia';
import api, { Note } from '../api/client';
import { useCalendarStore } from './calendar';

export const useNotesStore = defineStore('notes', {
  state: () => ({
    notes: [] as Note[],
    total: 0,
    page: 1,
    limit: 20,
    selectedDate: '',
    selectedTag: '',
    searchKeyword: '',
    loading: false,
    errorMessage: '',
  lastFailedMore: false,
  requestId: 0,
    selectedNote: null as Note | null,
    pollingMap: {} as Record<string, number>, // noteID -> timer ID
  }),
  actions: {
    async fetchNotes(resetPage = true) {
    const requestId = ++this.requestId;
    this.lastFailedMore = false;
      if (resetPage) {
        this.page = 1;
      }
      this.loading = true;
      this.errorMessage = '';
      try {
        const res = await api.listNotes({
          date: this.selectedDate || undefined,
          tag: this.selectedTag || undefined,
          keyword: this.searchKeyword || undefined,
          page: this.page,
          limit: this.limit,
        });
    if (requestId !== this.requestId) return;
        this.notes = res.data.items || [];
        this.total = res.data.total || 0;

        // 对处于 processing 状态的笔记启动轻量轮询
        for (const note of this.notes) {
          if (note.processing_status === 'processing') {
            this.startPollingNote(note.id);
          }
        }
      } catch (err: any) {
    if (requestId !== this.requestId) return;
        console.error('获取笔记列表失败:', err);
        this.errorMessage = err.response?.data?.error?.message || err.message || '加载笔记列表失败，请检查网络或后端服务';
      } finally {
        if (requestId === this.requestId) this.loading = false;
      }
    },

    async loadMore() {
      if (this.loading || this.notes.length >= this.total) return;
      const targetPage = this.page + 1;
    const requestId = ++this.requestId;
    this.errorMessage = '';
    this.lastFailedMore = false;
      this.loading = true;
      try {
        const res = await api.listNotes({
          date: this.selectedDate || undefined,
          tag: this.selectedTag || undefined,
          keyword: this.searchKeyword || undefined,
          page: targetPage,
          limit: this.limit,
        });
    if (requestId !== this.requestId) return;
        const items = res.data.items || [];
    const ids = new Set(this.notes.map(n => n.id));
        this.notes.push(...items.filter(n => !ids.has(n.id)));
    for (const n of items) if (n.processing_status === 'processing') this.startPollingNote(n.id);
        this.total = res.data.total || 0;
        this.page = targetPage;
      } catch (err: any) {
    if (requestId !== this.requestId) return;
    this.lastFailedMore = true;
        console.error('加载更多失败:', err);
        this.errorMessage = err.response?.data?.error?.message || err.message || '加载更多笔记失败';
      } finally {
        if (requestId === this.requestId) this.loading = false;
      }
    },

  retryFetch() { return this.lastFailedMore ? this.loadMore() : this.fetchNotes(); },

    async createNote(rawContent: string) {
      const res = await api.createNote(rawContent);
      const newId = res.data.id;
      // 插入一条轻量本地临时条目
      const optimisticNote: Note = {
        id: newId,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        raw_content: rawContent,
        processing_status: 'processing',
        tags: [],
        reminders: [],
        polished_content: '',
      };
      this.notes.unshift(optimisticNote);
      this.total++;
      this.startPollingNote(newId);

      // 联动刷新日历打点
      const calendarStore = useCalendarStore();
      calendarStore.fetchSummary();

      return res.data;
    },

    startPollingNote(noteId: string) {
      if (this.pollingMap[noteId]) return;

      let count = 0;
      const maxCount = 20; // 最多轮询 20 次 (约 30 秒)
      const timer = window.setInterval(async () => {
        count++;
        try {
          const res = await api.getNote(noteId);
          const updated = res.data;
          const idx = this.notes.findIndex((n) => n.id === noteId);
          if (idx !== -1) {
            this.notes[idx] = updated;
          }
          if (this.selectedNote?.id === noteId) {
            this.selectedNote = updated;
          }

          if (updated.processing_status !== 'processing' || count >= maxCount) {
            this.stopPollingNote(noteId);
            // AI 处理完毕，刷新日历与标签
            const calendarStore = useCalendarStore();
            calendarStore.fetchSummary();
            calendarStore.fetchTags();
          }
        } catch {
          this.stopPollingNote(noteId);
        }
      }, 1500);

      this.pollingMap[noteId] = timer;
    },

    stopPollingNote(noteId: string) {
      if (this.pollingMap[noteId]) {
        clearInterval(this.pollingMap[noteId]);
        delete this.pollingMap[noteId];
      }
    },

    async deleteNote(id: string) {
      await api.deleteNote(id);
      this.notes = this.notes.filter((n) => n.id !== id);
      this.total = Math.max(0, this.total - 1);
      if (this.selectedNote?.id === id) {
        this.selectedNote = null;
      }
      this.stopPollingNote(id);

      // 联动刷新日历与标签
      const calendarStore = useCalendarStore();
      calendarStore.fetchSummary();
      calendarStore.fetchTags();
    },

    async retryNote(id: string) {
      await api.retryNote(id);
      const idx = this.notes.findIndex((n) => n.id === id);
      if (idx !== -1) {
        this.notes[idx].processing_status = 'processing';
      }
      this.startPollingNote(id);
    },

    async completeReminder(noteId: string, remId: string) {
      const res = await api.completeReminder(noteId, remId);
      const note = this.notes.find((n) => n.id === noteId);
      if (note) {
        if (res.data?.updated_at) {
          note.updated_at = res.data.updated_at;
        }
        if (res.data?.reminders) {
          note.reminders = res.data.reminders;
        } else {
          const rem = note.reminders.find((r) => r.id === remId);
          if (rem) {
            rem.status = 'completed';
          }
        }
      }

      // 联动刷新日历提醒圆点
      const calendarStore = useCalendarStore();
      calendarStore.fetchSummary();
    },

    async cancelReminder(noteId: string, remId: string) {
      const res = await api.cancelReminder(noteId, remId);
      const note = this.notes.find((n) => n.id === noteId);
      if (note) {
        if (res.data?.updated_at) {
          note.updated_at = res.data.updated_at;
        }
        if (res.data?.reminders) {
          note.reminders = res.data.reminders;
        } else {
          const rem = note.reminders.find((r) => r.id === remId);
          if (rem) {
            rem.status = 'cancelled';
          }
        }
      }

      // 联动刷新日历提醒圆点
      const calendarStore = useCalendarStore();
      calendarStore.fetchSummary();
    },

    async reopenReminder(noteId: string, remId: string) {
      const res = await api.reopenReminder(noteId, remId);
      const note = this.notes.find((n) => n.id === noteId);
      if (note) {
        if (res.data?.updated_at) {
          note.updated_at = res.data.updated_at;
        }
        if (res.data?.reminders) {
          note.reminders = res.data.reminders;
        } else {
          const rem = note.reminders.find((r) => r.id === remId);
          if (rem) {
            rem.status = 'pending';
          }
        }
      }

      // 联动刷新日历提醒圆点
      const calendarStore = useCalendarStore();
      calendarStore.fetchSummary();
    },

    async updateNote(id: string, polishedContent: string, tags: string[], updatedAt: string) {
      const res = await api.updateNote(id, {
        polished_content: polishedContent,
        tags: tags,
        updated_at: updatedAt,
      });
      const updated = res.data;
      const idx = this.notes.findIndex((n) => n.id === id);
      if (idx !== -1) {
        this.notes[idx] = updated;
      }
      if (this.selectedNote?.id === id) {
        this.selectedNote = updated;
      }
      // 联动刷新日历与标签
      const calendarStore = useCalendarStore();
      calendarStore.fetchSummary();
      calendarStore.fetchTags();
      return updated;
    },

    setSelectedDate(date: string) {
      this.selectedDate = date;
      this.page = 1;
      this.fetchNotes();
    },

    setSelectedTag(tag: string) {
      this.selectedTag = tag;
      this.page = 1;
      this.fetchNotes();
    },

    setSearchKeyword(keyword: string) {
      this.searchKeyword = keyword;
      this.page = 1;
      this.fetchNotes();
    },

    clearFilters() {
      this.selectedDate = '';
      this.selectedTag = '';
      this.searchKeyword = '';
      this.page = 1;
      this.fetchNotes();
    },
  },
});
