<template>
  <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-4">
    <!-- 日历头部 -->
    <div class="flex items-center justify-between mb-4">
      <div class="flex items-center space-x-2">
        <CalendarIcon class="w-4 h-4 text-brand-600" />
        <h3 class="text-sm font-semibold text-slate-800">
          {{ calendarStore.year }} 年 {{ calendarStore.month }} 月
        </h3>
      </div>
      <div class="flex items-center space-x-1">
        <button
          @click="prevMonth"
          class="p-1 rounded-lg hover:bg-slate-100 text-slate-500 hover:text-slate-800 transition-colors"
          title="上个月"
        >
          <ChevronLeft class="w-4 h-4" />
        </button>
        <button
          @click="resetToday"
          class="px-2 py-0.5 rounded text-xs text-slate-500 hover:bg-slate-100 transition-colors"
          title="回到今天"
        >
          今天
        </button>
        <button
          @click="nextMonth"
          class="p-1 rounded-lg hover:bg-slate-100 text-slate-500 hover:text-slate-800 transition-colors"
          title="下个月"
        >
          <ChevronRight class="w-4 h-4" />
        </button>
      </div>
    </div>

    <!-- 星期表头 -->
    <div class="grid grid-cols-7 gap-1 text-center text-[11px] font-medium text-slate-400 mb-2">
      <span>日</span>
      <span>一</span>
      <span>二</span>
      <span>三</span>
      <span>四</span>
      <span>五</span>
      <span>六</span>
    </div>

    <!-- 日期网格 -->
    <div class="grid grid-cols-7 gap-1">
      <div
        v-for="(day, idx) in daysInMonth"
        :key="idx"
        @click="day.dateStr && selectDate(day.dateStr)"
        class="relative aspect-square flex flex-col items-center justify-center rounded-xl text-xs transition-all cursor-pointer select-none"
        :class="[
          !day.dateStr ? 'opacity-0 cursor-default' : '',
          day.isToday ? 'font-bold text-brand-600 ring-1 ring-brand-400/50' : 'text-slate-700 hover:bg-slate-100',
          notesStore.selectedDate === day.dateStr ? 'bg-brand-600 text-white font-semibold hover:bg-brand-700 shadow-sm' : '',
        ]"
      >
        <span>{{ day.dayNum }}</span>

        <!-- 标记指示器 -->
        <div class="flex items-center space-x-0.5 mt-0.5 h-1.5">
          <!-- 历史足迹打卡绿点 -->
          <span
            v-if="day.hasNote"
            class="w-1.5 h-1.5 rounded-full"
            :class="notesStore.selectedDate === day.dateStr ? 'bg-white' : 'bg-emerald-500'"
            title="当日有记录"
          ></span>

          <!-- 未来提醒待办橙点 -->
          <span
            v-if="day.hasReminder"
            class="w-1.5 h-1.5 rounded-full"
            :class="notesStore.selectedDate === day.dateStr ? 'bg-amber-300' : 'bg-amber-500'"
            title="当日有待办提醒"
          ></span>
        </div>
      </div>
    </div>

    <!-- 图例 -->
    <div class="mt-4 pt-3 border-t border-slate-100 flex items-center justify-between text-[11px] text-slate-400">
      <div class="flex items-center space-x-1.5">
        <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
        <span>打卡足迹</span>
      </div>
      <div class="flex items-center space-x-1.5">
        <span class="w-2 h-2 rounded-full bg-amber-500"></span>
        <span>备忘提醒</span>
      </div>
      <button
        v-if="notesStore.selectedDate"
        @click="notesStore.clearFilters"
        class="text-brand-600 hover:underline"
      >
        清除过滤
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { Calendar as CalendarIcon, ChevronLeft, ChevronRight } from 'lucide-vue-next';
import { useCalendarStore } from '../stores/calendar';
import { useNotesStore } from '../stores/notes';

const calendarStore = useCalendarStore();
const notesStore = useNotesStore();

onMounted(() => {
  calendarStore.fetchSummary();
});

function prevMonth() {
  let y = calendarStore.year;
  let m = calendarStore.month - 1;
  if (m < 1) {
    m = 12;
    y--;
  }
  calendarStore.fetchSummary(y, m);
}

function nextMonth() {
  let y = calendarStore.year;
  let m = calendarStore.month + 1;
  if (m > 12) {
    m = 1;
    y++;
  }
  calendarStore.fetchSummary(y, m);
}

function resetToday() {
  const now = new Date();
  calendarStore.fetchSummary(now.getFullYear(), now.getMonth() + 1);
  const todayStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
  notesStore.setSelectedDate(todayStr);
}

function selectDate(dateStr: string) {
  if (notesStore.selectedDate === dateStr) {
    notesStore.setSelectedDate('');
  } else {
    notesStore.setSelectedDate(dateStr);
  }
}

const daysInMonth = computed(() => {
  const y = calendarStore.year;
  const m = calendarStore.month;
  const firstDay = new Date(y, m - 1, 1).getDay(); // 0 is Sunday
  const lastDate = new Date(y, m, 0).getDate();
  const today = new Date();
  const todayStr = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`;

  const days = [];
  // 填充月初空白
  for (let i = 0; i < firstDay; i++) {
    days.push({ dayNum: '', dateStr: '', hasNote: false, hasReminder: false, isToday: false });
  }

  // 填充当月天数
  for (let d = 1; d <= lastDate; d++) {
    const dStr = `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
    const hasNote = calendarStore.activeDates.includes(dStr);
    const remItem = calendarStore.reminderDates.find((r) => r.date === dStr);
    const hasReminder = !!remItem && remItem.has_pending;
    const isToday = dStr === todayStr;

    days.push({
      dayNum: d,
      dateStr: dStr,
      hasNote,
      hasReminder,
      isToday,
    });
  }

  return days;
});
</script>
