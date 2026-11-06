import { defineStore } from 'pinia';
import api, { CalendarReminderDate, TagCount } from '../api/client';

export const useCalendarStore = defineStore('calendar', {
  state: () => ({
    year: new Date().getFullYear(),
    month: new Date().getMonth() + 1,
    activeDates: [] as string[],
    reminderDates: [] as CalendarReminderDate[],
    tags: [] as TagCount[],
    loading: false,
  }),
  actions: {
    async fetchSummary(year?: number, month?: number) {
      if (year) this.year = year;
      if (month) this.month = month;
      this.loading = true;
      try {
        const res = await api.getCalendarSummary(this.year, this.month);
        this.activeDates = res.data.active_dates || [];
        this.reminderDates = res.data.reminder_dates || [];
      } catch (err) {
        console.error('获取日历摘要失败:', err);
      } finally {
        this.loading = false;
      }
    },
    async fetchTags() {
      try {
        const res = await api.getTags();
        this.tags = res.data.tags || [];
      } catch (err) {
        console.error('获取标签列表失败:', err);
      }
    },
  },
});
