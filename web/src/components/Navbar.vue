<template>
  <header class="sticky top-0 z-30 bg-white/90 backdrop-blur border-b border-slate-200">
    <div class="max-w-7xl mx-auto px-4 h-16 flex items-center justify-between">
      <!-- 品牌 Logo -->
      <router-link to="/" class="flex items-center space-x-2 group">
        <div class="w-9 h-9 rounded-xl bg-gradient-to-tr from-brand-600 to-emerald-400 flex items-center justify-center text-white shadow-md shadow-brand-500/20 group-hover:scale-105 transition-transform">
          <Clock class="w-5 h-5" />
        </div>
        <div class="flex flex-col">
          <span class="font-bold text-lg text-slate-800 tracking-tight leading-none">ChronoLog</span>
          <span class="text-[10px] text-slate-400 font-medium tracking-wider">时光足迹</span>
        </div>
      </router-link>

      <!-- 桌面端导航 -->
      <nav class="hidden md:flex items-center space-x-1">
        <router-link
          to="/"
          class="px-3 py-2 rounded-lg text-sm font-medium transition-colors"
          :class="$route.path === '/' ? 'text-brand-600 bg-brand-50' : 'text-slate-600 hover:text-slate-900 hover:bg-slate-100'"
        >
          <div class="flex items-center space-x-1.5">
            <BookOpen class="w-4 h-4" />
            <span>回顾工作台</span>
          </div>
        </router-link>

        <router-link
          to="/calendar"
          class="px-3 py-2 rounded-lg text-sm font-medium transition-colors"
          :class="$route.path === '/calendar' ? 'text-brand-600 bg-brand-50' : 'text-slate-600 hover:text-slate-900 hover:bg-slate-100'"
        >
          <div class="flex items-center space-x-1.5">
            <CalendarIcon class="w-4 h-4" />
            <span>双模日历</span>
          </div>
        </router-link>

        <router-link
          to="/search"
          class="px-3 py-2 rounded-lg text-sm font-medium transition-colors"
          :class="$route.path === '/search' ? 'text-brand-600 bg-brand-50' : 'text-slate-600 hover:text-slate-900 hover:bg-slate-100'"
        >
          <div class="flex items-center space-x-1.5">
            <Search class="w-4 h-4" />
            <span>检索</span>
          </div>
        </router-link>

        <router-link
          to="/settings"
          class="px-3 py-2 rounded-lg text-sm font-medium transition-colors"
          :class="$route.path === '/settings' ? 'text-brand-600 bg-brand-50' : 'text-slate-600 hover:text-slate-900 hover:bg-slate-100'"
        >
          <div class="flex items-center space-x-1.5">
            <Settings class="w-4 h-4" />
            <span>设置</span>
          </div>
        </router-link>
      </nav>

      <!-- 用户信息与登出 -->
      <div class="flex items-center space-x-3">
        <div class="hidden sm:flex items-center space-x-2 text-xs">
          <span class="text-slate-500 font-mono">{{ authStore.email }}</span>
          <span v-if="authStore.isAdmin" class="px-2 py-0.5 rounded-full bg-purple-100 text-purple-700 font-medium">管理员</span>
        </div>
        <button
          @click="authStore.logout"
          title="退出登录"
          class="p-2 text-slate-400 hover:text-red-500 hover:bg-red-50 rounded-lg transition-colors"
        >
          <LogOut class="w-4 h-4" />
        </button>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { Clock, BookOpen, Calendar as CalendarIcon, Search, Settings, LogOut } from 'lucide-vue-next';
import { useAuthStore } from '../stores/auth';

const authStore = useAuthStore();
</script>
