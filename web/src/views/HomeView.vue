<template>
  <div class="max-w-7xl mx-auto px-4 py-6">
    <!-- 桌面端三栏全景布局 -->
    <div class="grid grid-cols-1 lg:grid-cols-12 gap-6">
      <!-- 左栏 (280px / 3 cols): 双模日历 + 标签导航 -->
      <aside class="hidden lg:block lg:col-span-4 space-y-5">
        <DualCalendar />
        <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-4">
          <div class="flex items-center justify-between mb-3">
            <h4 class="text-xs font-semibold text-slate-700 flex items-center space-x-1.5">
              <TagIcon class="w-3.5 h-3.5 text-brand-600" />
              <span>常用分类标签</span>
            </h4>
            <span class="text-[10px] text-slate-400">共 {{ calendarStore.tags.length }} 个</span>
          </div>
          <div class="flex flex-wrap gap-1.5 max-h-48 overflow-y-auto">
            <button
              v-for="t in calendarStore.tags"
              :key="t.tag"
              @click="toggleTag(t.tag)"
              class="px-2.5 py-1 rounded-lg text-xs font-medium transition-all"
              :class="notesStore.selectedTag === t.tag ? 'bg-brand-600 text-white shadow-sm' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'"
            >
              #{{ t.tag }} ({{ t.count }})
            </button>
            <span v-if="calendarStore.tags.length === 0" class="text-xs text-slate-300">
              暂无标签，发布带分类笔记后自动呈现
            </span>
          </div>
        </div>
      </aside>

      <!-- 中栏 (主工作流 / 5 cols 或移动端全宽): 快录 + 笔记流 -->
      <main class="lg:col-span-8 xl:col-span-5 space-y-5">
        <!-- 顶部常驻极速快录条 -->
        <QuickCapture />

        <!-- 移动端标签筛选横条 -->
        <div class="lg:hidden">
          <TagFilterBar />
        </div>

        <!-- 当前活动过滤器横幅 -->
        <div
          v-if="notesStore.selectedDate || notesStore.selectedTag || notesStore.searchKeyword"
          class="flex items-center justify-between bg-brand-50/80 border border-brand-200/60 px-4 py-2 rounded-xl text-xs text-brand-800"
        >
          <div class="flex items-center space-x-2">
            <Filter class="w-3.5 h-3.5 text-brand-600" />
            <span>当前筛选: </span>
            <span v-if="notesStore.selectedDate" class="font-semibold">{{ notesStore.selectedDate }}</span>
            <span v-if="notesStore.selectedTag" class="font-semibold">#{{ notesStore.selectedTag }}</span>
            <span v-if="notesStore.searchKeyword" class="font-semibold">"{{ notesStore.searchKeyword }}"</span>
          </div>
          <button
            @click="notesStore.clearFilters"
            class="text-xs text-brand-700 hover:text-brand-900 underline font-medium"
          >
            重置筛选
          </button>
        </div>

        <!-- 结构化卡片流 -->
        <div v-if="notesStore.loading && notesStore.notes.length === 0" class="py-12 text-center text-slate-400 text-sm">
          <div class="w-6 h-6 border-2 border-brand-500 border-t-transparent rounded-full animate-spin mx-auto mb-2"></div>
          正在加载足迹笔记...
        </div>

        <!-- 异常提示横幅 (防白屏或误导暂无笔记) -->
        <div
          v-else-if="notesStore.errorMessage"
          class="bg-red-50 border border-red-200/80 rounded-2xl p-6 text-center space-y-3"
        >
          <AlertTriangle class="w-8 h-8 text-red-500 mx-auto" />
          <div class="text-sm font-medium text-red-800">{{ notesStore.errorMessage }}</div>
          <button
            @click="notesStore.fetchNotes"
            class="px-4 py-1.5 bg-red-600 hover:bg-red-700 text-white text-xs font-medium rounded-lg transition-colors inline-flex items-center space-x-1.5 shadow-sm"
          >
            <RotateCcw class="w-3.5 h-3.5" />
            <span>重新加载</span>
          </button>
        </div>

        <div v-else-if="notesStore.notes.length === 0" class="bg-white rounded-2xl border border-slate-200/80 p-8 text-center space-y-2">
          <Sparkles class="w-8 h-8 text-slate-300 mx-auto" />
          <h4 class="text-sm font-semibold text-slate-700">暂无足迹记录</h4>
          <p class="text-xs text-slate-400">在上方输入框敲下一句话，立即开启您的智能知识库</p>
        </div>

        <div v-else class="space-y-4">
          <NoteCard
            v-for="note in notesStore.notes"
            :key="note.id"
            :note="note"
          />

          <!-- 加载更多分页控制 -->
          <div v-if="notesStore.notes.length < notesStore.total" class="pt-2 text-center">
            <button
              @click="notesStore.loadMore"
              :disabled="notesStore.loading"
              class="px-5 py-2.5 bg-white hover:bg-slate-50 border border-slate-200 text-slate-600 rounded-xl text-xs font-medium transition-all shadow-sm"
            >
              {{ notesStore.loading ? '正在加载...' : `加载更多 (已展示 ${notesStore.notes.length} / 共 ${notesStore.total} 条)` }}
            </button>
          </div>
        </div>
      </main>

      <!-- 右栏 (320px / 3 cols): 提示词调优看板 (管理员可见) 或 产品特性卡 -->
      <aside class="hidden xl:block xl:col-span-3 space-y-5">
        <PromptTester v-if="authStore.isAdmin" />
        <div v-else class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-5 space-y-3">
          <div class="flex items-center space-x-2 text-brand-600 font-semibold text-sm">
            <ShieldCheck class="w-4 h-4" />
            <span>纯文本平铺数据保障</span>
          </div>
          <p class="text-xs text-slate-500 leading-relaxed">
            您的所有记录均实时以 <strong>Markdown + YAML FrontMatter</strong> 平铺存储于独立目录中，不依赖任何闭源数据库，支持随时无损迁移。
          </p>
          <div class="pt-2 border-t border-slate-100 text-[11px] text-slate-400">
            支持 Ctrl+Enter 快捷秒发随手记。
          </div>
        </div>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { Tag as TagIcon, Filter, Sparkles, ShieldCheck, AlertTriangle, RotateCcw } from 'lucide-vue-next';
import { useNotesStore } from '../stores/notes';
import { useCalendarStore } from '../stores/calendar';
import { useAuthStore } from '../stores/auth';
import DualCalendar from '../components/DualCalendar.vue';
import QuickCapture from '../components/QuickCapture.vue';
import NoteCard from '../components/NoteCard.vue';
import TagFilterBar from '../components/TagFilterBar.vue';
import PromptTester from '../components/PromptTester.vue';

const notesStore = useNotesStore();
const calendarStore = useCalendarStore();
const authStore = useAuthStore();

onMounted(() => {
  notesStore.fetchNotes();
  calendarStore.fetchTags();
});

function toggleTag(tag: string) {
  if (notesStore.selectedTag === tag) {
    notesStore.setSelectedTag('');
  } else {
    notesStore.setSelectedTag(tag);
  }
}
</script>
