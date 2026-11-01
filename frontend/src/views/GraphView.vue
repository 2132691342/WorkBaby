<script setup lang="ts">
// 关系图谱页：把「功能 → 工具」「技能 → 工具」这两类真实依赖画成图。
// 数据全部来自现有清单接口，不落库、不手工维护第二份真相。
import { computed, defineAsyncComponent, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '../stores/settings'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'

// 两张重图不在首屏：只有切到对应页签才下载引擎代码
const RelationGraph = defineAsyncComponent(() => import('../components/graph/RelationGraph.vue'))
const BpmnDiagram = defineAsyncComponent(() => import('../components/graph/BpmnDiagram.vue'))

const { t } = useI18n()
const settings = useSettingsStore()

type Tab = 'relation' | 'workflow'
const tab = ref<Tab>('relation')

/** 图里的一种节点：id / 文本 / 分组（决定配色）。 */
interface GraphNode {
  id: string
  label: string
  group: string
}
/** 一条连线。 */
interface GraphEdge {
  source: string
  target: string
}

const loading = computed(() => !settings.warmed)

/** 已启用的功能与它们的工具归属：功能 → 工具。 */
const featureGraph = computed(() => {
  const nodes: GraphNode[] = []
  const edges: GraphEdge[] = []
  const seenTools = new Set<string>()
  for (const f of settings.features) {
    if (!f.feature_enabled) continue
    nodes.push({ id: `f:${f.feature_code}`, label: f.feature_name, group: 'feature' })
    for (const tool of f.feature_tools) {
      const key = `t:${tool}`
      if (!seenTools.has(key)) {
        seenTools.add(key)
        nodes.push({ id: key, label: tool, group: 'tool' })
      }
      edges.push({ source: `f:${f.feature_code}`, target: key })
    }
  }
  return { nodes, edges }
})

/** 技能分组视图：技能按来源分组，用于看「哪来的技能最多」。 */
const skillGraph = computed(() => {
  const nodes: GraphNode[] = []
  const edges: GraphEdge[] = []
  const groups = new Map<string, string>()
  for (const g of [
    { code: 'builtin', label: t('graph.sourceBuiltin') },
    { code: 'user', label: t('graph.sourceUser') },
    { code: 'workspace', label: t('graph.sourceWorkspace') },
  ]) {
    groups.set(g.code, g.label)
    nodes.push({ id: `g:${g.code}`, label: g.label, group: 'feature' })
  }
  for (const s of settings.skills) {
    const groupKey = `g:${s.skill_source}`
    if (!groups.has(s.skill_source)) continue
    const id = `s:${s.skill_name}`
    nodes.push({ id, label: s.skill_name, group: 'skill' })
    edges.push({ source: groupKey, target: id })
  }
  return { nodes, edges }
})

/** 工具来源视图：内置 / MCP 两大类，各自挂了哪些工具。 */
const toolGraph = computed(() => {
  const nodes: GraphNode[] = []
  const edges: GraphEdge[] = []
  const cats = new Map<string, string>([
    ['builtin', t('graph.toolBuiltin')],
    ['mcp', t('graph.toolMcp')],
  ])
  for (const [code, label] of cats) {
    nodes.push({ id: `c:${code}`, label, group: 'feature' })
  }
  for (const tool of settings.tools) {
    const key = `tool:${tool.tool_name}`
    const cat = cats.has(tool.tool_source) ? tool.tool_source : 'builtin'
    nodes.push({ id: key, label: tool.tool_name, group: 'tool' })
    edges.push({ source: `c:${cat}`, target: key })
  }
  return { nodes, edges }
})

type View = 'feature' | 'skill' | 'tool'
const view = ref<View>('feature')

const relation = computed(() => {
  if (view.value === 'skill') return skillGraph.value
  if (view.value === 'tool') return toolGraph.value
  return featureGraph.value
})

/** 视图为空时说明清楚缺什么，不留一张空画布。 */
const relationEmpty = computed(() => relation.value.nodes.length === 0)

/**
 * 工作流图：用 BPMN 画「一轮对话」的内核循环。
 * 这是产品行为的可读说明，不是某次运行的实际轨迹。
 */
const WORKFLOW_XML = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"
                  xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
                  xmlns:dc="http://www.omg.org/spec/DD/20100524/DC"
                  xmlns:di="http://www.omg.org/spec/DD/20100524/DI"
                  id="Definitions_wb" targetNamespace="http://workbaby.local/bpmn">
  <bpmn:process id="Process_run" isExecutable="false">
    <bpmn:startEvent id="Start_user" name="用户发言">
      <bpmn:outgoing>F1</bpmn:outgoing>
    </bpmn:startEvent>
    <bpmn:sequenceFlow id="F1" sourceRef="Start_user" targetRef="Task_build" />
    <bpmn:task id="Task_build" name="组装上下文">
      <bpmn:incoming>F1</bpmn:incoming>
      <bpmn:outgoing>F2</bpmn:outgoing>
    </bpmn:task>
    <bpmn:sequenceFlow id="F2" sourceRef="Task_build" targetRef="Gw_compact" />
    <bpmn:exclusiveGateway id="Gw_compact" name="超出窗口？">
      <bpmn:incoming>F2</bpmn:incoming>
      <bpmn:outgoing>F3</bpmn:outgoing>
      <bpmn:outgoing>F4</bpmn:outgoing>
    </bpmn:exclusiveGateway>
    <bpmn:sequenceFlow id="F3" name="是" sourceRef="Gw_compact" targetRef="Task_compact" />
    <bpmn:task id="Task_compact" name="压缩历史">
      <bpmn:incoming>F3</bpmn:incoming>
      <bpmn:outgoing>F5</bpmn:outgoing>
    </bpmn:task>
    <bpmn:sequenceFlow id="F5" sourceRef="Task_compact" targetRef="Task_sample" />
    <bpmn:sequenceFlow id="F4" name="否" sourceRef="Gw_compact" targetRef="Task_sample" />
    <bpmn:task id="Task_sample" name="模型采样">
      <bpmn:incoming>F4</bpmn:incoming>
      <bpmn:incoming>F5</bpmn:incoming>
      <bpmn:outgoing>F6</bpmn:outgoing>
    </bpmn:task>
    <bpmn:sequenceFlow id="F6" sourceRef="Task_sample" targetRef="Gw_tool" />
    <bpmn:exclusiveGateway id="Gw_tool" name="要调工具？">
      <bpmn:incoming>F6</bpmn:incoming>
      <bpmn:outgoing>F7</bpmn:outgoing>
      <bpmn:outgoing>F8</bpmn:outgoing>
    </bpmn:exclusiveGateway>
    <bpmn:sequenceFlow id="F7" name="是" sourceRef="Gw_tool" targetRef="Task_tool" />
    <bpmn:task id="Task_tool" name="执行工具批">
      <bpmn:incoming>F7</bpmn:incoming>
      <bpmn:outgoing>F9</bpmn:outgoing>
    </bpmn:task>
    <bpmn:sequenceFlow id="F9" sourceRef="Task_tool" targetRef="Task_sample" />
    <bpmn:sequenceFlow id="F8" name="否" sourceRef="Gw_tool" targetRef="End_done" />
    <bpmn:endEvent id="End_done" name="定稿落盘">
      <bpmn:incoming>F8</bpmn:incoming>
    </bpmn:endEvent>
  </bpmn:process>
  <bpmndi:BPMNDiagram id="Diagram_wb">
    <bpmndi:BPMNPlane id="Plane_wb" bpmnElement="Process_run">
      <bpmndi:BPMNShape id="S_start" bpmnElement="Start_user">
        <dc:Bounds x="120" y="182" width="36" height="36" />
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="S_build" bpmnElement="Task_build">
        <dc:Bounds x="200" y="160" width="120" height="80" />
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="S_gwc" bpmnElement="Gw_compact">
        <dc:Bounds x="365" y="175" width="50" height="50" />
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="S_compact" bpmnElement="Task_compact">
        <dc:Bounds x="360" y="280" width="120" height="80" />
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="S_sample" bpmnElement="Task_sample">
        <dc:Bounds x="460" y="160" width="120" height="80" />
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="S_gwt" bpmnElement="Gw_tool">
        <dc:Bounds x="625" y="175" width="50" height="50" />
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="S_tool" bpmnElement="Task_tool">
        <dc:Bounds x="600" y="280" width="120" height="80" />
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="S_end" bpmnElement="End_done">
        <dc:Bounds x="720" y="182" width="36" height="36" />
      </bpmndi:BPMNShape>
      <bpmndi:BPMNEdge id="E_f1" bpmnElement="F1">
        <di:waypoint x="156" y="200" /><di:waypoint x="200" y="200" />
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge id="E_f2" bpmnElement="F2">
        <di:waypoint x="320" y="200" /><di:waypoint x="365" y="200" />
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge id="E_f3" bpmnElement="F3">
        <di:waypoint x="390" y="225" /><di:waypoint x="390" y="280" />
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge id="E_f5" bpmnElement="F5">
        <di:waypoint x="480" y="320" /><di:waypoint x="500" y="240" />
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge id="E_f4" bpmnElement="F4">
        <di:waypoint x="415" y="200" /><di:waypoint x="460" y="200" />
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge id="E_f6" bpmnElement="F6">
        <di:waypoint x="580" y="200" /><di:waypoint x="625" y="200" />
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge id="E_f7" bpmnElement="F7">
        <di:waypoint x="650" y="225" /><di:waypoint x="650" y="280" />
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge id="E_f9" bpmnElement="F9">
        <di:waypoint x="600" y="320" /><di:waypoint x="520" y="240" />
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge id="E_f8" bpmnElement="F8">
        <di:waypoint x="675" y="200" /><di:waypoint x="720" y="200" />
      </bpmndi:BPMNEdge>
    </bpmndi:BPMNPlane>
  </bpmndi:BPMNDiagram>
</bpmn:definitions>`

const views: { code: View; label: string }[] = [
  { code: 'feature', label: 'graph.viewFeature' },
  { code: 'skill', label: 'graph.viewSkill' },
  { code: 'tool', label: 'graph.viewTool' },
]

onMounted(() => {
  if (!settings.warmed) void settings.warmup()
})
</script>

<template>
  <div class="page">
    <PageHeader :title="t('graph.title')" :subtitle="t('graph.desc')">
      <template #actions>
        <el-radio-group v-model="tab" size="small">
          <el-radio-button value="relation">{{ t('graph.tabRelation') }}</el-radio-button>
          <el-radio-button value="workflow">{{ t('graph.tabWorkflow') }}</el-radio-button>
        </el-radio-group>
      </template>
    </PageHeader>

    <div class="graph-body">
      <Transition name="wb-fade" mode="out-in">
        <section v-if="tab === 'relation'" key="relation" class="graph-card">
          <div class="graph-bar">
            <el-radio-group v-model="view" size="small">
              <el-radio-button v-for="v in views" :key="v.code" :value="v.code">
                {{ t(v.label) }}
              </el-radio-button>
            </el-radio-group>
            <span class="graph-sp" />
            <span class="graph-stat">
              {{ t('graph.stat', { nodes: relation.nodes.length, edges: relation.edges.length }) }}
            </span>
          </div>

          <div v-if="loading" class="wb-skeleton graph-skel" />
          <div v-else-if="relationEmpty" class="graph-empty">
            <span class="wb-tile wb-tile--lg"><AppIcon name="brain" size="ic-xl" /></span>
            <b>{{ t('graph.emptyTitle') }}</b>
            <p>{{ t('graph.emptyHint') }}</p>
          </div>
          <RelationGraph
            v-else
            :key="view"
            :nodes="relation.nodes"
            :edges="relation.edges"
            :height="520"
          />
        </section>

        <section v-else key="workflow" class="graph-card">
          <div class="graph-bar">
            <span class="graph-note">{{ t('graph.workflowNote') }}</span>
          </div>
          <BpmnDiagram :xml="WORKFLOW_XML" :height="520" />
        </section>
      </Transition>
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.graph-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-4) var(--wb-sp-5) var(--wb-sp-5);
}
.graph-card {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-xl);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
}
.graph-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}
.graph-sp {
  flex: 1;
}
.graph-stat {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.graph-note {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.graph-skel {
  height: calc(520px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-lg);
}
.graph-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-12) var(--wb-sp-6);
  text-align: center;
}
.graph-empty b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.graph-empty p {
  max-width: calc(440px * var(--wb-vw-scale));
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
</style>
