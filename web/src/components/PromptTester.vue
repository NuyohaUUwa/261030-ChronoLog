<template>
  <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-5 space-y-4">
    <div class="flex items-center justify-between border-b border-slate-100 pb-3">
      <div class="flex items-center space-x-2">
        <Sliders class="w-4 h-4 text-purple-600" />
        <h3 class="font-semibold text-sm text-slate-800">显式提示词工程与在线调优</h3>
      </div>
      <div class="flex items-center space-x-1">
        <button
          @click="activeTab = 'editor'"
          class="px-2.5 py-1 rounded-lg text-xs font-medium transition-colors"
          :class="activeTab === 'editor' ? 'bg-purple-100 text-purple-700' : 'text-slate-500 hover:bg-slate-100'"
        >
          YAML 配置
        </button>
        <button
          @click="activeTab = 'tester'"
          class="px-2.5 py-1 rounded-lg text-xs font-medium transition-colors"
          :class="activeTab === 'tester' ? 'bg-purple-100 text-purple-700' : 'text-slate-500 hover:bg-slate-100'"
        >
          效果实测
        </button>
      </div>
    </div>

    <!-- Tab 1: YAML 显式配置编辑器 -->
    <div v-if="activeTab === 'editor'" class="space-y-3">
      <textarea
        v-model="promptYaml"
        rows="14"
        class="w-full font-mono text-xs p-3 rounded-xl bg-slate-900 text-slate-100 border border-slate-800 outline-none focus:ring-2 focus:ring-purple-500/50 leading-relaxed"
        placeholder="正在加载 prompts.local.yaml..."
      ></textarea>
      <div class="flex items-center justify-between">
        <span class="text-xs text-slate-400">修改后将热生效，无需重启后端服务</span>
        <button
          @click="savePrompt"
          :disabled="saving"
          class="px-4 py-1.5 rounded-xl text-xs font-medium bg-purple-600 hover:bg-purple-700 disabled:opacity-40 text-white flex items-center space-x-1.5 transition-colors shadow-sm"
        >
          <Save class="w-3.5 h-3.5" />
          <span>{{ saving ? '保存中...' : '保存并生效' }}</span>
        </button>
      </div>
    </div>

    <!-- Tab 2: 在线测试调优面板 -->
    <div v-else class="space-y-3">
      <div>
        <label class="block text-xs font-medium text-slate-500 mb-1">测试随笔输入</label>
        <textarea
          v-model="testInput"
          rows="3"
          class="w-full text-xs p-3 rounded-xl border border-slate-200 outline-none focus:border-purple-500 focus:ring-2 focus:ring-purple-500/20"
          placeholder="例如：今天把李总的项目方案写完了，明天下午三点得把终稿发给法务"
        ></textarea>
      </div>

      <div class="flex justify-end">
        <button
          @click="runTest"
          :disabled="testing || !testInput.trim()"
          class="px-4 py-1.5 rounded-xl text-xs font-medium bg-purple-600 hover:bg-purple-700 disabled:opacity-40 text-white flex items-center space-x-1.5 transition-colors shadow-sm"
        >
          <Play class="w-3.5 h-3.5" />
          <span>{{ testing ? '调用中...' : '运行大模型推理' }}</span>
        </button>
      </div>

      <div v-if="testResult" class="space-y-2 pt-2 border-t border-slate-100">
        <div class="text-xs font-semibold text-slate-700">推理返回结果 (结构化 JSON)：</div>
        <pre class="bg-slate-900 text-emerald-400 p-3 rounded-xl text-xs font-mono overflow-x-auto">{{ JSON.stringify(testResult, null, 2) }}</pre>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { Sliders, Save, Play } from 'lucide-vue-next';
import api from '../api/client';

const activeTab = ref<'editor' | 'tester'>('editor');
const promptYaml = ref('');
const saving = ref(false);
const testInput = ref('今天把李总的项目方案写完了，改了三遍PPT，明天下午三点得发法务审核');
const testing = ref(false);
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const testResult = ref<any>(null);

onMounted(async () => {
  try {
    const res = await api.getPrompts();
    promptYaml.value = res.data.yaml;
  } catch (err) {
    console.error('加载提示词配置失败:', err);
  }
});

async function savePrompt() {
  if (!promptYaml.value.trim() || saving.value) return;
  saving.value = true;
  try {
    await api.updatePrompts(promptYaml.value);
    alert('提示词配置保存成功并已热生效！');
  } catch (err: unknown) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const msg = (err as any)?.response?.data?.error?.message || '保存失败';
    alert('保存失败: ' + msg);
  } finally {
    saving.value = false;
  }
}

async function runTest() {
  if (!testInput.value.trim() || testing.value) return;
  testing.value = true;
  testResult.value = null;
  try {
    const res = await api.testPrompt(testInput.value);
    testResult.value = res.data;
  } catch (err: unknown) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const msg = (err as any)?.response?.data?.error?.message || '测试失败';
    alert('测试失败: ' + msg);
  } finally {
    testing.value = false;
  }
}
</script>
