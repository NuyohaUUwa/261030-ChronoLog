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

      <div v-if="notesStore.loading" class="py-12 text-center text-slate-400 text-sm">
        正在检索足迹...
      </div>

      <div v-else-if="notesStore.notes.length === 0" class="bg-white rounded-2xl border border-slate-200/80 p-8 text-center text-slate-400 text-sm">
        未找到匹配的笔记
      </div>

      <NoteCard
        v-for="note in notesStore.notes"
        :key="note.id"
        :note="note"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { Search } from 'lucide-vue-next';
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
