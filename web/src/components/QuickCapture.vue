<template>
  <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-4 hover:shadow-md transition-shadow">
    <div class="relative">
      <textarea
        v-model="content"
        @keydown.ctrl.enter.prevent="handleSubmit"
        @keydown.meta.enter.prevent="handleSubmit"
        rows="3"
        class="w-full bg-slate-50/50 hover:bg-slate-50 focus:bg-white text-slate-800 text-sm rounded-xl p-3.5 border border-slate-200 focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition-all resize-none placeholder:text-slate-400"
        placeholder="随手记下当下的工作进展、生活灵感或待办备忘... (按 Ctrl + Enter 秒发)"
      ></textarea>
    </div>

    <div class="mt-3 flex items-center justify-between">
      <span class="text-[11px] text-slate-400">
        💡 提交后毫秒级落盘原稿，AI 异步抽取结构化标签与待办
      </span>

      <button
        @click="handleSubmit"
        :disabled="submitting || !content.trim()"
        class="px-4 py-2 rounded-xl text-sm font-medium bg-brand-600 hover:bg-brand-700 disabled:opacity-40 disabled:hover:bg-brand-600 text-white flex items-center space-x-1.5 shadow-sm transition-all"
      >
        <Send class="w-3.5 h-3.5" :class="{ 'animate-pulse': submitting }" />
        <span>{{ submitting ? '保存中...' : '快速记下' }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { Send } from 'lucide-vue-next';
import { useNotesStore } from '../stores/notes';

const content = ref('');
const submitting = ref(false);
const notesStore = useNotesStore();

async function handleSubmit() {
  if (!content.value.trim() || submitting.value) return;

  const textToSave = content.value;
  submitting.value = true;
  try {
    await notesStore.createNote(textToSave);
    content.value = '';
  } catch (err) {
    alert('保存笔记失败，请检查网络或后端服务');
    console.error(err);
  } finally {
    submitting.value = false;
  }
}
</script>
