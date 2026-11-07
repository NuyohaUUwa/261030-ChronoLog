import axios from 'axios';

export interface Reminder {
  id: string;
  title: string;
  remind_at: string;
  status: 'pending' | 'completed' | 'cancelled';
  notification_status: 'pending' | 'sent' | 'failed';
  reason?: string;
}

export interface Note {
  id: string;
  created_at: string;
  updated_at: string;
  raw_content: string;
  processing_status: 'processing' | 'ready' | 'failed';
  failure_reason?: string;
  tags: string[];
  reminders: Reminder[];
  polished_content: string;
}

export interface TagCount {
  tag: string;
  count: number;
}

export interface CalendarReminderDate {
  date: string;
  count: number;
  has_pending: boolean;
}

export interface CalendarSummary {
  active_dates: string[];
  reminder_dates: CalendarReminderDate[];
}

const apiClient = axios.create({
  baseURL: '/api/v1',
  timeout: 15000,
  headers: {
    'Content-Type': 'application/json',
  },
});

apiClient.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user_email');
      localStorage.removeItem('is_admin');
      if (window.location.pathname !== '/login') {
        const redirectPath = window.location.pathname + window.location.search;
        window.location.href = `/login?redirect=${encodeURIComponent(redirectPath)}`;
      }
    }
    return Promise.reject(error);
  }
);

export default {
  // 认证
  register(email: string, password: string, adminKey?: string) {
    return apiClient.post('/auth/register', { email, password, admin_key: adminKey || undefined });
  },
  login(email: string, password: string) {
    return apiClient.post('/auth/login', { email, password });
  },
  getMe() {
    return apiClient.get('/auth/me');
  },
  updateSettings(data: { ntfy_topic: string }) {
    return apiClient.put('/auth/settings', data);
  },

  // 笔记
  createNote(rawContent: string) {
    return apiClient.post<{ id: string; status: string }>('/notes', { raw_content: rawContent });
  },
  listNotes(params: { date?: string; tag?: string; keyword?: string; page?: number; limit?: number }) {
    return apiClient.get<{ items: Note[]; total: number; page: number; limit: number }>('/notes', { params });
  },
  getNote(id: string) {
    return apiClient.get<Note>(`/notes/${id}`);
  },
  updateNote(id: string, data: { polished_content?: string; tags?: string[]; reminders?: Reminder[]; updated_at: string }) {
    return apiClient.put<Note>(`/notes/${id}`, data);
  },
  deleteNote(id: string) {
    return apiClient.delete(`/notes/${id}`);
  },
  retryNote(id: string) {
    return apiClient.post<{ id: string; status: string }>(`/notes/${id}/retry`);
  },
  completeReminder(noteId: string, remId: string) {
    return apiClient.post<{ message: string; note_id: string; rem_id: string; updated_at: string; reminders: Reminder[] }>(`/notes/${noteId}/reminders/${remId}/complete`);
  },
  cancelReminder(noteId: string, remId: string) {
    return apiClient.post<{ message: string; note_id: string; rem_id: string; updated_at: string; reminders: Reminder[] }>(`/notes/${noteId}/reminders/${remId}/cancel`);
  },

  // 日历与标签
  getCalendarSummary(year?: number, month?: number) {
    return apiClient.get<CalendarSummary>('/calendar/summary', { params: { year, month } });
  },
  getTags() {
    return apiClient.get<{ tags: TagCount[] }>('/tags');
  },

  // 提示词（管理员）
  getPrompts() {
    return apiClient.get<{ yaml: string }>('/prompts');
  },
  updatePrompts(yamlContent: string) {
    return apiClient.put('/prompts', { yaml: yamlContent });
  },
  testPrompt(rawContent: string) {
    return apiClient.post('/prompts/test', { raw_content: rawContent });
  },
};
