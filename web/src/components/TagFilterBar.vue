<template>
  <div class="flex items-center space-x-1.5 overflow-x-auto py-1 scrollbar-none">
    <button
      @click="notesStore.setSelectedTag('')"
      class="px-3 py-1 rounded-full text-xs font-medium shrink-0 transition-colors"
      :class="!notesStore.selectedTag ? 'bg-brand-600 text-white shadow-sm' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'"
    >
      全部
    </button>

    <button
      v-for="item in calendarStore.tags"
      :key="item.tag"
      @click="toggleTag(item.tag)"
      class="px-3 py-1 rounded-full text-xs font-medium shrink-0 flex items-center space-x-1 transition-colors"
      :class="notesStore.selectedTag === item.tag ? 'bg-brand-600 text-white shadow-sm' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'"
    >
      <span>#{{ item.tag }}</span>
      <span
        class="text-[10px] px-1.5 py-0.2 rounded-full"
        :class="notesStore.selectedTag === item.tag ? 'bg-brand-700 text-brand-100' : 'bg-slate-200 text-slate-500'"
      >
        {{ item.count }}
      </span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { useCalendarStore } from '../stores/calendar';
import { useNotesStore } from '../stores/notes';

const calendarStore = useCalendarStore();
const notesStore = useNotesStore();

onMounted(() => {
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
