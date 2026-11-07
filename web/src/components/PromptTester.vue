<template>
  <div class="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-5 space-y-4">
    <div class="flex items-center justify-between border-b border-slate-100 pb-3">
      <div class="flex items-center space-x-2">
        <Sliders class="w-4 h-4 text-purple-600" />
        <h3 class="font-semibold text-sm text-slate-800">管理后台：大模型与提示词工程</h3>
      </div>
      <div class="flex items-center space-x-1">
        <button
          @click="activeTab = 'config'"
          class="px-2.5 py-1 rounded-lg text-xs font-medium transition-colors"
          :class="activeTab === 'config' ? 'bg-purple-100 text-purple-700' : 'text-slate-500 hover:bg-slate-100'"
        >
          AI 模型与 API
        </button>
        <button
          @click="activeTab = 'editor'"
          class="px-2.5 py-1 rounded-lg text-xs font-medium transition-colors"
          :class="activeTab === 'editor' ? 'bg-purple-100 text-purple-700' : 'text-slate-500 hover:bg-slate-100'"
        >
          YAML 提示词
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

    <!-- Tab 1: AI 模型与 API 密钥配置 -->
    <div v-if="activeTab === 'config'" class="space-y-4">
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label class="block text-xs font-medium text-slate-600 mb-1">运行模式 (Mode)</label>
          <div class="flex items-center space-x-3 mt-1.5">
            <label class="flex items-center space-x-1.5 text-xs text-slate-700 cursor-pointer">
              <input type="radio" value="remote" v-model="aiForm.mode" class="text-purple-600 focus:ring-purple-500" />
              <span>Remote (真实远程模型)</span>
            </label>
            <label class="flex items-center space-x-1.5 text-xs text-slate-700 cursor-pointer">
              <input type="radio" value="mock" v-model="aiForm.mode" class="text-purple-600 focus:ring-purple-500" />
              <span>Mock (本地模拟模式)</span>
            </label>
          </div>
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-600 mb-1">模型标识 (Model)</label>
          <input
            v-model="aiForm.model"
            type="text"
            placeholder="例如 deepseek-flash"
            class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono focus:bg-white focus:border-purple-500 focus:ring-2 focus:ring-purple-500/20 outline-none"
          />
        </div>

        <div class="md:col-span-2">
          <label class="block text-xs font-medium text-slate-600 mb-1">API 端点 (Base URL)</label>
          <input
            v-model="aiForm.base_url"
            type="text"
            placeholder="例如 https://api.deepseek.com"
            class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono focus:bg-white focus:border-purple-500 focus:ring-2 focus:ring-purple-500/20 outline-none"
          />
          <p class="text-[11px] text-slate-400 mt-1">兼容 OpenAI 规范端点，系统将自动拼接 /chat/completions</p>
        </div>

        <div class="md:col-span-2">
          <label class="block text-xs font-medium text-slate-600 mb-1">API 密钥 (API Key)</label>
          <div class="relative">
            <input
              v-model="aiForm.api_key"
              type="password"
              :placeholder="aiForm.has_api_key ? `已配置 (${aiForm.api_key_masked})，留空保持现有密钥` : '请输入 API Key，如 sk-...'"
              class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono focus:bg-white focus:border-purple-500 focus:ring-2 focus:ring-purple-500/20 outline-none"
            />
          </div>
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-600 mb-1">超时时长 (秒)</label>
          <input
            v-model.number="aiForm.timeout_seconds"
            type="number"
            min="5"
            max="300"
            class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono focus:bg-white focus:border-purple-500 focus:ring-2 focus:ring-purple-500/20 outline-none"
          />
        </div>
      </div>

      <!-- 连通性测试反馈 -->
      <div v-if="testConnResult" class="p-3 rounded-xl text-xs flex items-center justify-between" :class="testConnResult.status === 'ok' ? 'bg-emerald-50 text-emerald-700 border border-emerald-200' : 'bg-red-50 text-red-700 border border-red-200'">
        <span>{{ testConnResult.status === 'ok' ? `连通成功 (耗时 ${testConnResult.latency_ms}ms): ${testConnResult.reply}` : `连通失败: ${testConnResult.reply}` }}</span>
      </div>

      <div class="flex items-center justify-between pt-2 border-t border-slate-100">
        <button
          @click="testConnection"
          :disabled="testingConn"
          class="px-3.5 py-1.5 rounded-xl text-xs font-medium bg-slate-100 hover:bg-slate-200 text-slate-700 flex items-center space-x-1.5 transition-colors"
        >
          <Activity class="w-3.5 h-3.5 text-slate-500" />
          <span>{{ testingConn ? '正在测试连通性...' : '测试接口连通性' }}</span>
        </button>

        <button
          @click="saveAIConfig"
          :disabled="savingConfig"
          class="px-4 py-1.5 rounded-xl text-xs font-medium bg-purple-600 hover:bg-purple-700 disabled:opacity-40 text-white flex items-center space-x-1.5 transition-colors shadow-sm"
        >
          <Save class="w-3.5 h-3.5" />
          <span>{{ savingConfig ? '正在保存...' : '保存并热生效' }}</span>
        </button>
      </div>
    </div>

    <!-- Tab 2: YAML 显式配置编辑器 -->
    <div v-else-if="activeTab === 'editor'" class="space-y-3">
      <textarea
        v-model="promptYaml"
        rows="14"
        class="w-full font-mono text-xs p-3 rounded-xl bg-slate-900 text-slate-100 border border-slate-800 outline-none focus:ring-2 focus:ring-purple-500/50 leading-relaxed"
        placeholder="正在加载提示词配置..."
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

    <!-- Tab 3: 在线测试调优面板 -->
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
import { Sliders, Save, Play, Activity } from 'lucide-vue-next';
import api from '../api/client';

const activeTab = ref<'config' | 'editor' | 'tester'>('config');

// AI Config
const aiForm = ref({
  mode: 'remote',
  base_url: 'https://api.deepseek.com',
  model: 'deepseek-flash',
  api_key: '',
  timeout_seconds: 60,
  has_api_key: false,
  api_key_masked: '',
});
const savingConfig = ref(false);
const testingConn = ref(false);
const testConnResult = ref<{ status: string; latency_ms: number; reply: string } | null>(null);

// Prompt Editor
const promptYaml = ref('');
const saving = ref(false);
const testInput = ref('今天把李总的项目方案写完了，改了三遍PPT，明天下午三点得发法务审核');
const testing = ref(false);
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const testResult = ref<any>(null);

onMounted(async () => {
  try {
    const [promptRes, aiRes] = await Promise.all([
      api.getPrompts(),
      api.getAIConfig().catch(() => null),
    ]);
    promptYaml.value = promptRes.data.yaml;
    if (aiRes?.data) {
      aiForm.value.mode = aiRes.data.mode;
      aiForm.value.base_url = aiRes.data.base_url;
      aiForm.value.model = aiRes.data.model;
      aiForm.value.timeout_seconds = aiRes.data.timeout_seconds || 60;
      aiForm.value.has_api_key = aiRes.data.has_api_key;
      aiForm.value.api_key_masked = aiRes.data.api_key_masked;
    }
  } catch (err) {
    console.error('加载管理后台配置失败:', err);
  }
});

async function saveAIConfig() {
  savingConfig.value = true;
  testConnResult.value = null;
  try {
    const res = await api.updateAIConfig({
      mode: aiForm.value.mode,
      base_url: aiForm.value.base_url,
      api_key: aiForm.value.api_key.trim() || undefined,
      model: aiForm.value.model,
      timeout_seconds: aiForm.value.timeout_seconds,
    });
    aiForm.value.has_api_key = res.data.has_api_key;
    aiForm.value.api_key_masked = res.data.api_key_masked;
    aiForm.value.api_key = '';
    alert('AI 服务配置保存成功并已热生效！');
  } catch (err: unknown) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const msg = (err as any)?.response?.data?.error?.message || '保存失败';
    alert('保存配置失败: ' + msg);
  } finally {
    savingConfig.value = false;
  }
}

async function testConnection() {
  testingConn.value = true;
  testConnResult.value = null;
  try {
    const res = await api.testAIConfig({
      mode: aiForm.value.mode,
      base_url: aiForm.value.base_url,
      api_key: aiForm.value.api_key.trim() || undefined,
      model: aiForm.value.model,
    });
    testConnResult.value = res.data;
  } catch (err: unknown) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const msg = (err as any)?.response?.data?.error?.message || '测试连通性失败';
    testConnResult.value = {
      status: 'error',
      latency_ms: 0,
      reply: msg,
    };
  } finally {
    testingConn.value = false;
  }
}

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
