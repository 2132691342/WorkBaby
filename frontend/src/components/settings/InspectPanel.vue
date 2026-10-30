<script setup lang="ts">
// 内省面板：当前装配一屏看全 —— 工具 / 技能 / 命令 / 功能 / 系统提示词快照。
// 数据是只读快照（/inspect），改开关请去对应面板。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import type { CommandVO, FeatureVO, SkillVO, ToolVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import CodeEditor from '../common/CodeEditor.vue'
import SettingsSection from './SettingsSection.vue'

const tools = ref<ToolVO[]>([])
const skills = ref<SkillVO[]>([])
const commands = ref<CommandVO[]>([])
const features = ref<FeatureVO[]>([])
const prompt = ref('')
const loading = ref(true)
const err = ref('')
const keyword = ref('')

const hit = (name: string, desc: string) => {
  const k = keyword.value.trim().toLowerCase()
  return !k || name.toLowerCase().includes(k) || desc.toLowerCase().includes(k)
}
const toolRows = computed(() => tools.value.filter((x) => hit(x.tool_name, x.tool_description)))
const skillRows = computed(() => skills.value.filter((x) => hit(x.skill_name, x.skill_description)))
const commandRows = computed(() => commands.value.filter((x) => hit(x.command_name, x.command_description)))

async function load() {
  loading.value = true
  err.value = ''
  try {
    const snap = await api.inspect()
    tools.value = (snap.resp_tools as unknown as ToolVO[]) || []
    skills.value = (snap.resp_skills as unknown as SkillVO[]) || []
    commands.value = (snap.resp_commands as unknown as CommandVO[]) || []
    features.value = (snap.resp_features as unknown as FeatureVO[]) || []
    prompt.value = snap.resp_system_prompt || ''
  } catch (e) {
    err.value = (e as Error)?.message || ''
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection icon="search" title="内省" desc="当前装配快照：助手此刻能看到哪些工具、技能、命令与系统提示词。">
      <template #actions>
        <el-button size="small" :loading="loading" @click="load">
          <AppIcon v-if="!loading" name="refresh" size="ic-xs" />
          <span class="ml-1">刷新</span>
        </el-button>
      </template>

      <div v-if="err" class="wb-card empty-box">
        <b>加载失败</b>
        <p>{{ err }}</p>
        <el-button size="small" @click="load">重试</el-button>
      </div>

      <template v-else>
        <div class="bar">
          <el-input v-model="keyword" size="small" class="kw" placeholder="按名字或描述过滤" clearable />
          <span class="cnt wb-mono">
            {{ toolRows.length }} 工具 · {{ skillRows.length }} 技能 · {{ commandRows.length }} 命令
          </span>
        </div>

        <div class="grid">
          <section class="wb-card sec">
            <header class="sec-head"><AppIcon name="wrench" size="ic-xs" /> 工具</header>
            <ul class="rows">
              <li v-for="x in toolRows" :key="x.tool_name" class="row">
                <span class="nm wb-mono">{{ x.tool_name }}</span>
                <el-tag size="small" :type="x.tool_source === 'mcp' ? 'warning' : 'info'">{{ x.tool_source }}</el-tag>
                <el-tag v-if="!x.tool_enabled" size="small" type="danger">停用</el-tag>
                <span class="ds">{{ x.tool_description }}</span>
              </li>
            </ul>
          </section>

          <section class="wb-card sec">
            <header class="sec-head"><AppIcon name="book" size="ic-xs" /> 技能</header>
            <ul class="rows">
              <li v-for="x in skillRows" :key="x.skill_name" class="row">
                <span class="nm wb-mono">{{ x.skill_name }}</span>
                <el-tag size="small" type="info">{{ x.skill_source }}</el-tag>
                <el-tag v-if="!x.skill_enabled" size="small" type="danger">停用</el-tag>
                <span class="ds">{{ x.skill_description }}</span>
              </li>
            </ul>
          </section>

          <section class="wb-card sec">
            <header class="sec-head"><AppIcon name="terminal" size="ic-xs" /> 斜杠命令</header>
            <ul class="rows">
              <li v-for="x in commandRows" :key="x.command_name" class="row">
                <span class="nm wb-mono">/{{ x.command_name }}</span>
                <span class="ds">{{ x.command_description }}</span>
              </li>
            </ul>
          </section>

          <section class="wb-card sec">
            <header class="sec-head"><AppIcon name="zap" size="ic-xs" /> 功能</header>
            <ul class="rows">
              <li v-for="x in features" :key="x.feature_code" class="row">
                <span class="nm">{{ x.feature_name }}</span>
                <el-tag size="small" :type="x.feature_enabled ? 'success' : 'info'">
                  {{ x.feature_enabled ? '启用' : '停用' }}
                </el-tag>
                <span class="ds">{{ x.feature_description }}</span>
              </li>
            </ul>
          </section>
        </div>

        <section class="wb-card prompt-sec">
          <header class="sec-head"><AppIcon name="doc" size="ic-xs" /> 系统提示词（只读快照）</header>
          <CodeEditor v-model="prompt" lang="markdown" :min-height="220" :readonly="true" />
        </section>
      </template>
    </SettingsSection>
  </div>
</template>

<style scoped>
.ml-1 {
  margin-left: var(--wb-sp-1);
}
.bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}
.kw {
  width: calc(260px * var(--wb-vw-scale));
}
.cnt {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(calc(320px * var(--wb-vw-scale)), 1fr));
  gap: var(--wb-sp-4);
}
.sec {
  overflow: hidden;
}
.sec-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
  border-bottom: 1px solid var(--wb-border);
}
.rows {
  max-height: calc(280px * var(--wb-vw-scale));
  overflow-y: auto;
  padding: var(--wb-sp-1) var(--wb-sp-2);
}
.row {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-1) 0;
  font-size: var(--wb-fs-2xs);
}
.nm {
  font-weight: 700;
  color: var(--wb-ink);
  flex: none;
}
.row .ds {
  flex: 1;
  min-width: 0;
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.prompt-sec {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3);
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8);
  text-align: center;
}
</style>
