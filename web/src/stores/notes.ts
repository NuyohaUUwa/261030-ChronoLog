import { defineStore } from 'pinia';
import api, { Note } from '../api/client';

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
    selectedNote: null as Note | null,
    pollingMap: {} as Record<string, number>, // noteID -> timer ID
  }),
  actions: {
    async fetchNotes() {
      this.loading = true;
      try {
        const res = await api.listNotes({
          date: this.selectedDate || undefined,
          tag: this.selectedTag || undefined,
          keyword: this.searchKeyword || undefined,
          page: this.page,
          limit: this.limit,
        });
        this.notes = res.data.items || [];
        this.total = res.data.total || 0;

        // 对处于 processing 状态的笔记启动轻量轮询
        for (const note of this.notes) {
          if (note.processing_status === 'processing') {
            this.startPollingNote(note.id);
          }
        }
      } catch (err) {
        console.error('获取笔记列表失败:', err);
      } finally {
        this.loading = false;
      }
    },

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
      await api.completeReminder(noteId, remId);
      const note = this.notes.find((n) => n.id === noteId);
      if (note) {
        const rem = note.reminders.find((r) => r.id === remId);
        if (rem) {
          rem.status = 'completed';
        }
      }
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
