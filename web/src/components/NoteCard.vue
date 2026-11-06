<template>
  <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm hover:shadow-md transition-all duration-200 overflow-hidden">
    <!-- 卡片头部元数据 -->
    <div class="px-5 py-3.5 bg-slate-50/60 border-b border-slate-100 flex items-center justify-between flex-wrap gap-2">
      <div class="flex items-center space-x-2 text-xs text-slate-500 font-medium">
        <Clock class="w-3.5 h-3.5 text-slate-400" />
        <span>{{ formatTime(note.created_at) }}</span>
      </div>

      <div class="flex items-center space-x-2">
        <!-- 提醒徽标 -->
        <template v-for="rem in note.reminders" :key="rem.id">
          <div
            v-if="rem.status === 'pending'"
            class="flex items-center space-x-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-amber-50 text-amber-700 border border-amber-200/60"
            :title="rem.reason || '到期提醒'"
          >
            <Bell class="w-3 h-3 text-amber-500 animate-pulse" />
            <span>{{ rem.title }} · {{ formatRemindTime(rem.remind_at) }}</span>
            <button
              @click.stop="handleCompleteReminder(rem.id)"
              title="点击标记完成"
              class="ml-1 hover:text-emerald-600 transition-colors"
            >
              <CheckCircle class="w-3.5 h-3.5" />
            </button>
          </div>
          <div
            v-else-if="rem.status === 'completed'"
            class="flex items-center space-x-1 px-2.5 py-1 rounded-full text-xs font-medium bg-slate-100 text-slate-400 line-through"
          >
            <Check class="w-3 h-3 text-emerald-500" />
            <span>{{ rem.title }}</span>
          </div>
        </template>

        <!-- 删除操作 -->
        <button
          @click.stop="handleDelete"
          title="删除笔记"
          class="p-1 text-slate-400 hover:text-red-500 rounded transition-colors"
        >
          <Trash2 class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>

    <!-- 卡片主体正文 -->
    <div class="p-5">
      <!-- AI 状态展示 -->
      <div v-if="note.processing_status === 'processing'" class="mb-3 flex items-center space-x-2 text-xs text-brand-600 bg-brand-50/80 px-3 py-1.5 rounded-lg border border-brand-200/50">
        <Sparkles class="w-3.5 h-3.5 animate-spin" />
        <span>AI 正在结构化润色与抽取备忘中...</span>
      </div>

      <div v-else-if="note.processing_status === 'failed'" class="mb-3 flex items-center justify-between text-xs text-red-600 bg-red-50 px-3 py-1.5 rounded-lg border border-red-200/50">
        <div class="flex items-center space-x-1.5">
          <AlertCircle class="w-3.5 h-3.5" />
          <span>{{ note.failure_reason || 'AI 处理失败，原稿已妥善留存' }}</span>
        </div>
        <button
          @click.stop="handleRetry"
          class="text-xs font-medium text-red-700 underline hover:text-red-800 flex items-center space-x-1"
        >
          <RotateCcw class="w-3 h-3" />
          <span>重试</span>
        </button>
      </div>

      <!-- 润色正文 -->
      <div
        v-if="note.polished_content"
        class="prose prose-sm max-w-none text-slate-700 leading-relaxed break-words"
        v-html="renderedPolishedContent"
      ></div>
      <div
        v-else
        class="text-slate-600 text-sm leading-relaxed whitespace-pre-line"
      >{{ note.raw_content }}</div>
    </div>

    <!-- 卡片底部：标签与原稿折叠抽屉 -->
    <div class="px-5 py-3 bg-slate-50/40 border-t border-slate-100 flex items-center justify-between flex-wrap gap-2">
      <!-- 标签胶囊 -->
      <div class="flex items-center flex-wrap gap-1.5">
        <span
          v-for="tag in note.tags"
          :key="tag"
          @click.stop="handleTagClick(tag)"
          class="inline-flex items-center space-x-1 px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-100 text-slate-600 hover:bg-brand-50 hover:text-brand-600 cursor-pointer transition-colors"
        >
          <Tag class="w-3 h-3 opacity-60" />
          <span>#{{ tag }}</span>
        </span>
        <span v-if="!note.tags || note.tags.length === 0" class="text-xs text-slate-300">无标签</span>
      </div>

      <!-- 原稿折叠开关 -->
      <button
        @click="showRaw = !showRaw"
        class="text-xs text-slate-400 hover:text-slate-600 flex items-center space-x-1 px-2 py-1 rounded hover:bg-slate-100 transition-colors"
      >
        <Eye class="w-3.5 h-3.5" />
        <span>{{ showRaw ? '收起原稿' : '查看原稿' }}</span>
        <ChevronDown class="w-3 h-3 transition-transform" :class="{ 'rotate-180': showRaw }" />
      </button>
    </div>

    <!-- 原稿抽屉 -->
    <div v-if="showRaw" class="px-5 py-3.5 bg-slate-100/70 border-t border-slate-200/60 text-xs text-slate-600">
      <div class="font-medium text-slate-400 mb-1 flex items-center space-x-1">
        <FileText class="w-3.5 h-3.5" />
        <span>随手记原始输入 (未润色)</span>
      </div>
      <blockquote class="pl-3 border-l-2 border-slate-300 text-slate-700 whitespace-pre-wrap font-mono">
        {{ note.raw_content }}
      </blockquote>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { marked } from 'marked';
import { Clock, Bell, CheckCircle, Check, Trash2, Sparkles, AlertCircle, RotateCcw, Tag, Eye, ChevronDown, FileText } from 'lucide-vue-next';
import { Note } from '../api/client';
import { useNotesStore } from '../stores/notes';

const props = defineProps<{
  note: Note;
}>();

const notesStore = useNotesStore();
const showRaw = ref(false);

const renderedPolishedContent = computed(() => {
  if (!props.note.polished_content) return '';
  return marked.parse(props.note.polished_content, { breaks: true, gfm: true }) as string;
});

function formatTime(isoStr: string): string {
  try {
    const d = new Date(isoStr);
    const now = new Date();
    const isToday = d.toDateString() === now.toDateString();
    const hours = String(d.getHours()).padStart(2, '0');
    const mins = String(d.getMinutes()).padStart(2, '0');
    if (isToday) {
      return `今天 ${hours}:${mins}`;
    }
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const date = String(d.getDate()).padStart(2, '0');
    return `${month}-${date} ${hours}:${mins}`;
  } catch {
    return isoStr;
  }
}

function formatRemindTime(isoStr: string): string {
  try {
    const d = new Date(isoStr);
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const date = String(d.getDate()).padStart(2, '0');
    const hours = String(d.getHours()).padStart(2, '0');
    const mins = String(d.getMinutes()).padStart(2, '0');
    return `${month}-${date} ${hours}:${mins}`;
  } catch {
    return isoStr;
  }
}

function handleTagClick(tag: string) {
  notesStore.setSelectedTag(tag);
}

function handleCompleteReminder(remId: string) {
  notesStore.completeReminder(props.note.id, remId);
}

function handleDelete() {
  if (confirm('确认删除这条笔记吗？')) {
    notesStore.deleteNote(props.note.id);
  }
}

function handleRetry() {
  notesStore.retryNote(props.note.id);
}
</script>
