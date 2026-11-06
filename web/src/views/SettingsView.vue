<template>
  <div class="max-w-3xl mx-auto px-4 py-6 space-y-6">
    <!-- 账号基本信息 -->
    <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-6 space-y-4">
      <div class="flex items-center space-x-3">
        <div class="w-10 h-10 rounded-xl bg-brand-50 text-brand-600 flex items-center justify-center font-bold">
          <User class="w-5 h-5" />
        </div>
        <div>
          <h3 class="text-sm font-semibold text-slate-800">{{ authStore.email }}</h3>
          <p class="text-xs text-slate-400">
            角色权限: {{ authStore.isAdmin ? '系统管理员 (具备提示词调优权限)' : '普通用户' }}
          </p>
        </div>
      </div>
    </div>

    <!-- 通知渠道设置 -->
    <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-6 space-y-4">
      <div class="flex items-center space-x-2 border-b border-slate-100 pb-3">
        <Bell class="w-4 h-4 text-brand-600" />
        <h3 class="text-sm font-semibold text-slate-800">到期备忘通知设置 (Ntfy 推送)</h3>
      </div>

      <p class="text-xs text-slate-500 leading-relaxed">
        {{ authStore.notificationsMode === 'mock' ? '当前为模拟通知，不会向手机发送消息。' : '当前使用远程通知渠道，消息送达取决于服务器配置及手机订阅。' }}
      </p>

      <div class="space-y-3">
        <div>
          <label class="block text-xs font-medium text-slate-600 mb-1">Ntfy 订阅频道 Topic</label>
          <div class="flex items-center space-x-2">
            <input
              v-model="ntfyTopic"
              type="text"
              class="flex-1 px-3.5 py-2.5 bg-slate-50 border border-slate-200 rounded-xl text-sm focus:bg-white focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition-all font-mono"
            />
            <button
              @click="saveTopic"
              :disabled="savingTopic"
              class="px-4 py-2.5 rounded-xl text-sm font-medium bg-brand-600 hover:bg-brand-700 disabled:opacity-40 text-white transition-colors"
            >
              {{ savingTopic ? '保存中...' : '保存' }}
            </button>
          </div>
        </div>

        <div class="p-3 bg-slate-50 rounded-xl text-[11px] text-slate-500 space-y-1">
          <div>订阅主题: <code class="font-mono text-brand-600">{{ ntfyTopic }}</code></div>
          <div>通知服务地址由服务器配置；通知点击定位笔记功能尚未实现。</div>
        </div>
      </div>
    </div>

    <!-- 管理员专属提示词面板 -->
    <div v-if="authStore.isAdmin">
      <PromptTester />
    </div>

    <!-- 退出登录 -->
    <div class="pt-4 flex justify-end">
      <button
        @click="authStore.logout"
        class="px-5 py-2.5 rounded-xl border border-red-200 text-red-600 hover:bg-red-50 text-sm font-medium transition-colors"
      >
        退出登录
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { User, Bell } from 'lucide-vue-next';
import { useAuthStore } from '../stores/auth';
import api from '../api/client';
import PromptTester from '../components/PromptTester.vue';

const authStore = useAuthStore();
const ntfyTopic = ref('');
const savingTopic = ref(false);

onMounted(async () => {
  await authStore.fetchMe();
  ntfyTopic.value = authStore.ntfyTopic;
});

async function saveTopic() {
  const topic = ntfyTopic.value.trim();
  if (!topic || savingTopic.value) return;

  savingTopic.value = true;
  try {
    await api.updateSettings({ ntfy_topic: topic });
    authStore.ntfyTopic = topic;
    alert('通知设置更新成功！');
  } catch (err) {
    console.error(err);
    alert('更新通知设置失败');
  } finally {
    savingTopic.value = false;
  }
}
</script>
