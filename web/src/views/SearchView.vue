<template>
  <div class="max-w-4xl mx-auto px-4 py-6 space-y-6">
    <!-- 搜索框 -->
    <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-4 space-y-3">
      <div class="relative">
        <input
          v-model="keywordInput"
          @keydown.enter="handleSearch"
          type="text"
          placeholder="输入关键词进行 SQLite FTS5 全文秒级检索... (按 Enter 搜索)"
          class="w-full pl-10 pr-24 py-3 bg-slate-50 border border-slate-200 rounded-xl text-sm focus:bg-white focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition-all"
        />
        <Search class="w-4 h-4 text-slate-400 absolute left-3.5 top-3.5" />
        <button
          @click="handleSearch"
          class="absolute right-2 top-2 px-4 py-1.5 rounded-lg text-xs font-medium bg-brand-600 hover:bg-brand-700 text-white transition-colors"
        >
          检索
        </button>
      </div>

      <!-- 快速标签筛选 -->
      <TagFilterBar />
    </div>

    <!-- 结果列表 -->
    <div class="space-y-4">
      <div class="flex items-center justify-between text-xs text-slate-500 px-1">
        <span>检索结果 (共 {{ notesStore.total }} 条)</span>
        <button
          v-if="notesStore.searchKeyword || notesStore.selectedTag"
          @click="clearSearch"
          class="text-brand-600 hover:underline"
        >
          清空搜索条件
        </button>
      </div>

      <!-- 异常提示横幅 -->
      <div
        v-if="notesStore.errorMessage"
        class="bg-red-50 border border-red-200/80 rounded-2xl p-6 text-center space-y-3"
      >
        <AlertTriangle class="w-8 h-8 text-red-500 mx-auto" />
        <div class="text-sm font-medium text-red-800">{{ notesStore.errorMessage }}</div>
        <button
          @click="notesStore.retryFetch()"
          class="px-4 py-1.5 bg-red-600 hover:bg-red-700 text-white text-xs font-medium rounded-lg transition-colors inline-flex items-center space-x-1.5 shadow-sm"
        >
          <RotateCcw class="w-3.5 h-3.5" />
          <span>重新检索</span>
        </button>
      </div>

      <div v-if="notesStore.loading && notesStore.notes.length === 0" class="py-12 text-center text-slate-400 text-sm">
        正在检索足迹...
      </div>

      <div v-if="!notesStore.loading && !notesStore.errorMessage && notesStore.notes.length === 0" class="bg-white rounded-2xl border border-slate-200/80 p-8 text-center text-slate-400 text-sm">
        未找到匹配的笔记
      </div>

      <template v-if="notesStore.notes.length > 0">
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
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { Search, AlertTriangle, RotateCcw } from 'lucide-vue-next';
import { useNotesStore } from '../stores/notes';
import NoteCard from '../components/NoteCard.vue';
import TagFilterBar from '../components/TagFilterBar.vue';

const notesStore = useNotesStore();
const keywordInput = ref(notesStore.searchKeyword);

onMounted(() => {
  notesStore.fetchNotes();
});

function handleSearch() {
  notesStore.setSearchKeyword(keywordInput.value.trim());
}

function clearSearch() {
  keywordInput.value = '';
  notesStore.clearFilters();
}
</script>
