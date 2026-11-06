import { defineStore } from 'pinia';
import api from '../api/client';

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    email: localStorage.getItem('user_email') || '',
    isAdmin: localStorage.getItem('is_admin') === 'true',
    ntfyTopic: '',
    loading: false,
  }),
  getters: {
    isAuthenticated: (state) => !!state.token,
  },
  actions: {
    async register(email: string, password: string) {
      this.loading = true;
      try {
        const res = await api.register(email, password);
        this.setAuthData(res.data);
        return res.data;
      } finally {
        this.loading = false;
      }
    },
    async login(email: string, password: string) {
      this.loading = true;
      try {
        const res = await api.login(email, password);
        this.setAuthData(res.data);
        return res.data;
      } finally {
        this.loading = false;
      }
    },
    setAuthData(data: { token: string; email: string; is_admin: boolean; ntfy_topic?: string }) {
      this.token = data.token;
      this.email = data.email;
      this.isAdmin = data.is_admin;
      this.ntfyTopic = data.ntfy_topic || '';
      localStorage.setItem('token', data.token);
      localStorage.setItem('user_email', data.email);
      localStorage.setItem('is_admin', data.is_admin ? 'true' : 'false');
    },
    async fetchMe() {
      if (!this.token) return;
      try {
        const res = await api.getMe();
        this.email = res.data.email;
        this.isAdmin = res.data.is_admin;
        this.ntfyTopic = res.data.ntfy_topic;
        localStorage.setItem('is_admin', res.data.is_admin ? 'true' : 'false');
      } catch (e) {
        console.error('获取个人信息失败', e);
      }
    },
    logout() {
      this.token = '';
      this.email = '';
      this.isAdmin = false;
      this.ntfyTopic = '';
      localStorage.removeItem('token');
      localStorage.removeItem('user_email');
      localStorage.removeItem('is_admin');
      window.location.href = '/login';
    },
  },
});
