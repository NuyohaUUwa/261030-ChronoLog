<template>
  <div
    :id="'note-' + note.id"
    class="bg-white rounded-2xl border border-slate-200/80 shadow-sm hover:shadow-md transition-all duration-300 overflow-hidden"
  >
    <!-- 卡片头部元数据 -->
    <div class="px-5 py-3.5 bg-slate-50/60 border-b border-slate-100 flex items-center justify-between flex-wrap gap-2">
      <div class="flex items-center space-x-2 text-xs text-slate-500 font-medium">
        <Clock class="w-3.5 h-3.5 text-slate-400" />
        <span>{{ formatTime(note.created_at) }}</span>
      </div>

      <div class="flex items-center space-x-2">
        <!-- 提醒徽标与一键完成/取消 (W02) -->
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
              class="ml-1 text-slate-400 hover:text-emerald-600 transition-colors"
            >
              <CheckCircle class="w-3.5 h-3.5" />
            </button>
            <button
              @click.stop="handleCancelReminder(rem.id)"
              title="点击取消待办"
              class="text-slate-400 hover:text-red-500 transition-colors"
            >
              <XCircle class="w-3.5 h-3.5" />
            </button>
          </div>
          <div
            v-else-if="rem.status === 'completed'"
            class="flex items-center space-x-1 px-2.5 py-1 rounded-full text-xs font-medium bg-slate-100 text-slate-400 group/rem"
            title="已完成待办，点击可恢复为未完成"
          >
            <span class="line-through">{{ rem.title }}</span>
            <button
              @click.stop="handleReopenReminder(rem.id)"
              title="点击恢复为未完成"
              class="ml-1 text-slate-400 hover:text-brand-600 transition-colors flex items-center"
            >
              <RotateCcw class="w-3 h-3" />
            </button>
          </div>
          <div
            v-else-if="rem.status === 'cancelled'"
            class="flex items-center space-x-1 px-2.5 py-1 rounded-full text-xs font-medium bg-slate-100 text-slate-400 opacity-75 group/rem"
            title="已取消待办，点击可恢复为未完成"
          >
            <span class="line-through">{{ rem.title }} (已取消)</span>
            <button
              @click.stop="handleReopenReminder(rem.id)"
              title="点击恢复为未完成"
              class="ml-1 text-slate-400 hover:text-brand-600 transition-colors flex items-center"
            >
              <RotateCcw class="w-3 h-3" />
            </button>
          </div>
        </template>

        <!-- 编辑正文与标签操作 (W01) -->
        <button
          v-if="!isEditing"
          @click.stop="startEdit"
          title="二次编辑正文与标签"
          class="p-1 text-slate-400 hover:text-brand-600 rounded transition-colors"
        >
          <Pencil class="w-3.5 h-3.5" />
        </button>

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

    <!-- 卡片主体正文：二次编辑表单 (W01) -->
    <div v-if="isEditing" class="p-5 space-y-3 bg-slate-50/30">
      <div v-if="editError" class="px-3 py-1.5 rounded-lg bg-red-50 border border-red-200 text-xs text-red-600">
        {{ editError }}
      </div>
      <div>
        <label class="block text-xs font-medium text-slate-600 mb-1">正文内容 (支持 Markdown)</label>
        <textarea
          v-model="editContent"
          rows="5"
          class="w-full px-3.5 py-2.5 bg-white border border-slate-200 rounded-xl text-sm text-slate-700 focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition-all resize-y leading-relaxed"
          placeholder="输入笔记正文..."
        ></textarea>
      </div>
      <div>
        <label class="block text-xs font-medium text-slate-600 mb-1">标签分类 (逗号或空格分隔，如：工作/方案, 会议)</label>
        <input
          v-model="editTagsStr"
          type="text"
          class="w-full px-3.5 py-2 bg-white border border-slate-200 rounded-xl text-xs text-slate-700 focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition-all"
          placeholder="标签分类..."
        />
      </div>
      <div class="flex items-center justify-end space-x-2 pt-2">
        <button
          @click="cancelEdit"
          :disabled="saving"
          class="px-3 py-1.5 rounded-xl border border-slate-200 text-xs text-slate-600 hover:bg-slate-100 transition-colors"
        >
          取消
        </button>
        <button
          @click="saveEdit"
          :disabled="saving"
          class="px-4 py-1.5 rounded-xl bg-brand-600 hover:bg-brand-700 text-white text-xs font-medium disabled:opacity-50 transition-colors shadow-sm"
        >
          {{ saving ? '保存中...' : '保存修改' }}
        </button>
      </div>
    </div>

    <!-- 卡片主体正文：常规展示模式 -->
    <div v-else class="p-5">
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
import DOMPurify from 'dompurify';
import {
  Clock,
  Bell,
  CheckCircle,
  Trash2,
  Sparkles,
  AlertCircle,
  RotateCcw,
  Tag,
  Eye,
  ChevronDown,
  FileText,
  Pencil,
  XCircle,
} from 'lucide-vue-next';
import { Note } from '../api/client';
import { useNotesStore } from '../stores/notes';

const props = defineProps<{
  note: Note;
}>();

const notesStore = useNotesStore();
const showRaw = ref(false);

// 二次编辑状态 (W01, R3-03)
const isEditing = ref(false);
const editContent = ref('');
const editTagsStr = ref('');
const draftBaseUpdatedAt = ref('');
const saving = ref(false);
const editError = ref('');

function startEdit() {
  editContent.value = props.note.polished_content || props.note.raw_content;
  editTagsStr.value = (props.note.tags || []).join(', ');
  draftBaseUpdatedAt.value = props.note.updated_at; // 冻结进入编辑时的基线版本快照 (R3-03)
  editError.value = '';
  isEditing.value = true;
}

function cancelEdit() {
  isEditing.value = false;
  editError.value = '';
}

async function saveEdit() {
  saving.value = true;
  editError.value = '';
  try {
    const rawTags = editTagsStr.value
      .split(/[,，\s]+/)
      .map((t) => t.trim().replace(/^#/, ''))
      .filter((t) => t.length > 0);
    const uniqueTags = Array.from(new Set(rawTags));
    await notesStore.updateNote(
      props.note.id,
      editContent.value,
      uniqueTags,
      draftBaseUpdatedAt.value // 使用进入编辑时的快照版本，防并发覆盖 (R3-03)
    );
    isEditing.value = false;
  } catch (err: any) {
    if (err.response?.status === 409) {
      editError.value = '笔记已被后台或其他修改，草稿已妥善留存，请确认后重试';
    } else {
      editError.value = err.response?.data?.error?.message || '保存修改失败，请重试';
    }
  } finally {
    saving.value = false;
  }
}

const renderedPolishedContent = computed(() => {
  if (!props.note.polished_content) return '';
  const rawHtml = marked.parse(props.note.polished_content, { breaks: true, gfm: true }) as string;
  const safeDOM = DOMPurify.sanitize(rawHtml, {
    RETURN_DOM: true,
    ALLOWED_TAGS: [
      'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'p', 'b', 'i', 'strong', 'em', 'strike', 's', 'code', 'pre',
      'ul', 'ol', 'li', 'blockquote', 'hr', 'br', 'table', 'thead', 'tbody', 'tr', 'th', 'td', 'a', 'span', 'input'
    ],
    ALLOWED_ATTR: ['href', 'title', 'rel', 'class', 'type', 'checked', 'disabled'],
  }) as HTMLElement;
  safeDOM.querySelectorAll('input').forEach((input) => {
    input.type = 'checkbox';
    input.disabled = true;
  });
  return safeDOM.innerHTML;
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

function handleCancelReminder(remId: string) {
  notesStore.cancelReminder(props.note.id, remId);
}

function handleReopenReminder(remId: string) {
  notesStore.reopenReminder(props.note.id, remId);
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
