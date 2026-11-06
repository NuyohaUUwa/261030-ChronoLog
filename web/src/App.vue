<template>
  <div class="min-h-screen flex flex-col bg-slate-50 text-slate-800">
    <Navbar v-if="authStore.isAuthenticated" />

    <div class="flex-1 pb-20 md:pb-6">
      <router-view />
    </div>

    <!-- 移动端底部专属导航栏 -->
    <nav
      v-if="authStore.isAuthenticated"
      class="md:hidden fixed bottom-0 left-0 right-0 z-30 bg-white/95 backdrop-blur border-t border-slate-200 flex items-center justify-around py-2 px-1 safe-area-bottom shadow-lg"
    >
      <router-link
        to="/"
        class="flex flex-col items-center py-1 px-3 text-xs font-medium transition-colors"
        :class="$route.path === '/' ? 'text-brand-600' : 'text-slate-400 hover:text-slate-700'"
      >
        <BookOpen class="w-5 h-5 mb-0.5" />
        <span>足迹回顾</span>
      </router-link>

      <router-link
        to="/calendar"
        class="flex flex-col items-center py-1 px-3 text-xs font-medium transition-colors"
        :class="$route.path === '/calendar' ? 'text-brand-600' : 'text-slate-400 hover:text-slate-700'"
      >
        <CalendarIcon class="w-5 h-5 mb-0.5" />
        <span>双模日历</span>
      </router-link>

      <router-link
        to="/search"
        class="flex flex-col items-center py-1 px-3 text-xs font-medium transition-colors"
        :class="$route.path === '/search' ? 'text-brand-600' : 'text-slate-400 hover:text-slate-700'"
      >
        <Search class="w-5 h-5 mb-0.5" />
        <span>检索</span>
      </router-link>

      <router-link
        to="/settings"
        class="flex flex-col items-center py-1 px-3 text-xs font-medium transition-colors"
        :class="$route.path === '/settings' ? 'text-brand-600' : 'text-slate-400 hover:text-slate-700'"
      >
        <Settings class="w-5 h-5 mb-0.5" />
        <span>设置</span>
      </router-link>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { BookOpen, Calendar as CalendarIcon, Search, Settings } from 'lucide-vue-next';
import { useAuthStore } from './stores/auth';
import Navbar from './components/Navbar.vue';

const authStore = useAuthStore();
</script>
