<template>
  <div class="max-w-4xl mx-auto px-4 py-6 space-y-6">
    <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-5">
      <DualCalendar />
    </div>

    <!-- 当日笔记流 -->
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-semibold text-slate-800 flex items-center space-x-2">
          <span>{{ notesStore.selectedDate ? `${notesStore.selectedDate} 的足迹记录` : '全部历史足迹记录' }}</span>
          <span class="text-xs text-slate-400 font-normal">({{ notesStore.notes.length }} 条)</span>
        </h3>
        <button
          v-if="notesStore.selectedDate"
          @click="notesStore.setSelectedDate('')"
          class="text-xs text-brand-600 hover:underline"
        >
          查看所有日期
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
          <span>重新加载</span>
        </button>
      </div>

      <div v-if="notesStore.loading && notesStore.notes.length === 0" class="py-12 text-center text-slate-400 text-sm">
        正在加载日历足迹...
      </div>

      <div v-if="!notesStore.loading && !notesStore.errorMessage && notesStore.notes.length === 0" class="bg-white rounded-2xl border border-slate-200/80 p-8 text-center text-slate-400 text-sm">
        所选日期暂无笔记打卡或待办提醒
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
import { onMounted } from 'vue';
import { AlertTriangle, RotateCcw } from 'lucide-vue-next';
import { useNotesStore } from '../stores/notes';
import DualCalendar from '../components/DualCalendar.vue';
import NoteCard from '../components/NoteCard.vue';

const notesStore = useNotesStore();

onMounted(() => {
  notesStore.fetchNotes();
});
</script>
