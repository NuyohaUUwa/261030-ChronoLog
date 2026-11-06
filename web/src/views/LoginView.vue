<template>
  <div class="min-h-screen flex items-center justify-center p-4 bg-gradient-to-br from-slate-50 via-emerald-50/20 to-slate-100">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-xl shadow-slate-200/50 border border-slate-200/80 p-8 space-y-6">
      <!-- 品牌标识 -->
      <div class="text-center space-y-2">
        <div class="w-12 h-12 mx-auto rounded-2xl bg-gradient-to-tr from-brand-600 to-emerald-400 flex items-center justify-center text-white shadow-lg shadow-brand-500/25">
          <Clock class="w-7 h-7" />
        </div>
        <h1 class="text-2xl font-bold tracking-tight text-slate-800">ChronoLog 时光足迹</h1>
        <p class="text-xs text-slate-400">个人智能知识库、每日足迹与双模日历系统</p>
      </div>

      <!-- 切换登录 / 注册 -->
      <div class="flex rounded-xl bg-slate-100 p-1">
        <button
          @click="isRegister = false"
          class="flex-1 py-1.5 rounded-lg text-xs font-semibold transition-all"
          :class="!isRegister ? 'bg-white text-slate-800 shadow-sm' : 'text-slate-500 hover:text-slate-800'"
        >
          登录
        </button>
        <button
          @click="isRegister = true"
          class="flex-1 py-1.5 rounded-lg text-xs font-semibold transition-all"
          :class="isRegister ? 'bg-white text-slate-800 shadow-sm' : 'text-slate-500 hover:text-slate-800'"
        >
          注册
        </button>
      </div>

      <!-- 表单 -->
      <form @submit.prevent="handleSubmit" class="space-y-4">
        <div>
          <label class="block text-xs font-medium text-slate-600 mb-1">邮箱地址</label>
          <div class="relative">
            <input
              v-model="email"
              type="email"
              required
              placeholder="user@example.com"
              class="w-full pl-10 pr-4 py-2.5 bg-slate-50 border border-slate-200 rounded-xl text-sm focus:bg-white focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition-all"
            />
            <Mail class="w-4 h-4 text-slate-400 absolute left-3.5 top-3" />
          </div>
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-600 mb-1">密码</label>
          <div class="relative">
            <input
              v-model="password"
              type="password"
              required
              minlength="6"
              placeholder="至少 6 位密码"
              class="w-full pl-10 pr-4 py-2.5 bg-slate-50 border border-slate-200 rounded-xl text-sm focus:bg-white focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition-all"
            />
            <Lock class="w-4 h-4 text-slate-400 absolute left-3.5 top-3" />
          </div>
        </div>

        <div v-if="errorMsg" class="p-3 rounded-xl bg-red-50 text-red-600 text-xs flex items-center space-x-2 border border-red-200/60">
          <AlertCircle class="w-4 h-4 shrink-0" />
          <span>{{ errorMsg }}</span>
        </div>

        <button
          type="submit"
          :disabled="authStore.loading"
          class="w-full py-2.5 rounded-xl bg-brand-600 hover:bg-brand-700 disabled:opacity-50 text-white font-medium text-sm shadow-md shadow-brand-500/20 transition-all flex items-center justify-center space-x-2"
        >
          <span>{{ authStore.loading ? '提交中...' : (isRegister ? '立即注册并登录' : '登 录') }}</span>
          <ArrowRight class="w-4 h-4" />
        </button>
      </form>

      <div class="text-center text-[11px] text-slate-400">
        所有笔记均平铺存储为纯文本 Markdown，数据 100% 归个人所有
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { Clock, Mail, Lock, AlertCircle, ArrowRight } from 'lucide-vue-next';
import { useAuthStore } from '../stores/auth';

const router = useRouter();
const authStore = useAuthStore();

const isRegister = ref(false);
const email = ref('');
const password = ref('');
const errorMsg = ref('');

async function handleSubmit() {
  errorMsg.value = '';
  try {
    if (isRegister.value) {
      await authStore.register(email.value, password.value);
    } else {
      await authStore.login(email.value, password.value);
    }
    router.push('/');
  } catch (err: unknown) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    errorMsg.value = (err as any)?.response?.data?.error?.message || '请求失败，请稍后重试';
  }
}
</script>
