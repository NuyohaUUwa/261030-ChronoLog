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

      <div v-if="notesStore.notes.length === 0" class="bg-white rounded-2xl border border-slate-200/80 p-8 text-center text-slate-400 text-sm">
        所选日期暂无笔记打卡
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
import { onMounted } from 'vue';
import { useNotesStore } from '../stores/notes';
import DualCalendar from '../components/DualCalendar.vue';
import NoteCard from '../components/NoteCard.vue';

const notesStore = useNotesStore();

onMounted(() => {
  notesStore.fetchNotes();
});
</script>
