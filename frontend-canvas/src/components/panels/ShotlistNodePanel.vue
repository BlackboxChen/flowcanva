<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { Node, Asset } from '../../types'
import { useNodeConfigs } from '../../composables/useNodeConfigs'
import { useShotlistRunner, type PanelInput } from '../../composables/useShotlistRunner'
import {
  SHOTLIST_STATE_LABELS, stateAtLeast,
  type ShotlistValidation, type ShotlistSpawnRequest,
} from '../../types/shotlist'
import GateCard from '../shotlist/GateCard.vue'
import ScriptLockList from '../shotlist/ScriptLockList.vue'
import AssetMappingList from '../shotlist/AssetMappingList.vue'
import SegmentPlanTable from '../shotlist/SegmentPlanTable.vue'
import PromptViewer from '../shotlist/PromptViewer.vue'

const props = defineProps<{
  node: Node | null
  panelStyle: Record<string, string>
  nodeInputs?: PanelInput[]
  assets?: Asset[]
}>()

const emit = defineEmits<{
  (e: 'save', content: string): void
  (e: 'remove-connected-edge', edgeId: string): void
  (e: 'spawn-node', req: ShotlistSpawnRequest): void
}>()

const { getModels, getModelConfig, getRoles, loadNodeConfigs } = useNodeConfigs()

const selectedModel = ref('')
const selectedRole = ref('')
const models = ref<string[]>([])
const validation = ref<ShotlistValidation>({ errors: [], warnings: [] })

const runner = useShotlistRunner({
  getNode: () => props.node,
  getInputs: () => props.nodeInputs ?? [],
  save: content => emit('save', content),
  resolveModel: () => ({ name: selectedModel.value, config: getModelConfig('shotlist', selectedModel.value) }),
  spawn: req => emit('spawn-node', req),
})

const {
  content, loading, currentStep, error, progress, state,
  load, persist, loadSteps, confirmGate,
  extractScriptText, collectAssetCandidates,
  runLlmStep, runAllPrompts, revalidate, exportMarkdown, spawnToCanvas,
} = runner

const roles = computed(() => getRoles('shotlist'))
const candidates = computed(() => collectAssetCandidates())
const hasScriptSource = computed(() => Boolean(extractScriptText().trim()))
const stateLabel = computed(() => SHOTLIST_STATE_LABELS[state.value] || state.value)
const hasLockList = computed(() =>
  content.value.script.EV.length > 0
  || content.value.script.DLG.length > 0
  || content.value.script.LOCK.length > 0,
)
const hasPrompt = computed(() => content.value.segments.some(s => s.prompt.trim().length > 0))

const styleHint = computed(() => {
  const role = roles.value.find(r => r.key === selectedRole.value)
  return role?.prompt || ''
})

// 七道 Gate，顺序与 SKILL.md 状态机一致。P0 只覆盖「不使用故事板」分支。
const FLOW = [
  { key: 'lock_script', gate: 'SCRIPT_LOCKED', title: '锁定剧本', manual: false },
  { key: 'assets', gate: 'ASSET_CONFIRMED', title: '资产映射', manual: false },
  { key: 'mode', gate: 'MODE_CONFIRMED', title: '故事板方式', manual: true },
  { key: 'spatial', gate: 'SPATIAL_CONFIRMED', title: '空间调度', manual: false },
  { key: 'plan', gate: 'PLAN_CONFIRMED', title: '工作段规划', manual: false },
  { key: 'prompts', gate: 'PROMPTS_VALIDATED', title: '视频提示词', manual: false },
  { key: 'export', gate: 'DELIVERED', title: '交付导出', manual: true },
]

const stepStates = computed(() =>
  FLOW.map((step, i) => {
    if (stateAtLeast(state.value, step.gate)) return 'done' as const
    const prevDone = i === 0 || stateAtLeast(state.value, FLOW[i - 1].gate)
    return prevDone ? ('ready' as const) : ('pending' as const)
  }),
)

function refreshModels() {
  const list = getModels('shotlist')
  if (list.length === 0) {
    // 节点配置还没加载完时先给一个可用默认值，加载完成后再刷新。
    models.value = ['deepseek-v4-pro']
    void loadNodeConfigs().then(() => {
      const retry = getModels('shotlist')
      if (retry.length > 0) {
        models.value = retry
        if (!retry.includes(selectedModel.value)) selectedModel.value = retry[0]
      }
    })
    return
  }
  models.value = list
  if (!list.includes(selectedModel.value)) selectedModel.value = list[0]
}

watch(
  () => props.node?.id,
  async () => {
    if (props.node?.node_type !== 'shotlist') return
    load()
    validation.value = { errors: [], warnings: [] }
    await loadSteps()
    refreshModels()
  },
  { immediate: true },
)

// ─── 各步骤动作 ────────────────────────────────────────────────────

async function doLockScript() {
  await runLlmStep('lock_script')
}

async function doAssets() {
  await runLlmStep('assets')
}

async function doSpatial() {
  await runLlmStep('spatial')
}

async function doPlan() {
  await runLlmStep('plan')
}

async function doPrompts() {
  await runAllPrompts(styleHint.value)
}

async function doValidate() {
  const result = await revalidate()
  if (!result) return
  validation.value = result
  if (result.errors.length === 0) confirmGate('PROMPTS_VALIDATED')
}

async function doExport() {
  const result = await exportMarkdown()
  if (!result) return
  const blob = new Blob([result.markdown], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = result.filename
  link.click()
  URL.revokeObjectURL(url)
  confirmGate('DELIVERED')
}

function updateScript(patch: { EV?: any; DLG?: any; LOCK?: any }) {
  persist({ script: { ...content.value.script, ...patch } })
}
</script>

<template>
  <div v-if="node && node.node_type === 'shotlist'" :style="panelStyle" class="pointer-events-auto" @pointerdown.stop>
    <div class="rounded-2xl border border-white/15 bg-neutral-900/95 backdrop-blur shadow-xl text-white px-3 py-2.5">
      <!-- 头部：状态 + 风格 + 模型 -->
      <div class="flex items-center gap-2 mb-2">
        <span class="text-[12px] font-medium text-white/85 shrink-0">分镜台</span>
        <span class="text-[10px] px-1.5 py-0.5 rounded-full bg-cyan-400/15 text-cyan-200 shrink-0">{{ stateLabel }}</span>
        <span v-if="loading" class="text-[10px] text-amber-200 truncate">
          {{ currentStep || '处理中' }}{{ progress ? ' · ' + progress : '' }}
        </span>
        <div class="ml-auto flex items-center gap-1.5 shrink-0">
          <select v-model="selectedRole" class="text-[10px] bg-white/5 border border-white/10 rounded px-1.5 py-1 outline-none max-w-[110px]">
            <option value="" class="bg-neutral-900">默认风格</option>
            <option v-for="r in roles" :key="r.key" :value="r.key" class="bg-neutral-900">{{ r.label }}</option>
          </select>
          <select v-model="selectedModel" class="text-[10px] bg-white/5 border border-white/10 rounded px-1.5 py-1 outline-none max-w-[170px]">
            <option v-for="m in models" :key="m" :value="m" class="bg-neutral-900">{{ m }}</option>
          </select>
        </div>
      </div>

      <div v-if="error" class="mb-2 rounded border border-red-400/30 bg-red-950/40 px-2 py-1 text-[10px] text-red-200">{{ error }}</div>
      <div v-if="!hasScriptSource" class="mb-2 rounded border border-amber-400/30 bg-amber-950/30 px-2 py-1 text-[10px] text-amber-200">
        还没有连入剧本。把「剧本」或「文本」节点连到分镜台左侧，或先粘贴剧本。
      </div>

      <!-- Gate 列表 -->
      <div class="max-h-[440px] overflow-y-auto pr-0.5 space-y-1.5" @wheel.stop>
        <!-- 1 锁定剧本 -->
        <GateCard :index="1" title="锁定剧本" :status="stepStates[0]"
          description="拆成 EV/DLG/LOCK 清单。台词逐字保留，不做任何改编。">
          <template #actions>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading || !hasScriptSource" @click.stop="doLockScript">生成</button>
            <button v-if="stepStates[0] === 'ready'" class="text-[10px] px-2 py-0.5 rounded bg-cyan-500/25 hover:bg-cyan-500/40 text-cyan-100"
              @click.stop="confirmGate('SCRIPT_LOCKED')">锁定</button>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading || !hasLockList" title="把清单落地成画布上的表格节点"
              @click.stop="spawnToCanvas('script')">落地表格</button>
          </template>
          <ScriptLockList v-if="content.script.EV.length || content.script.DLG.length || content.script.LOCK.length"
            :ev="content.script.EV" :dlg="content.script.DLG" :lock="content.script.LOCK" @update="updateScript" />
          <div v-else class="text-[11px] text-white/25">尚未生成清单</div>
        </GateCard>

        <!-- 2 资产映射 -->
        <GateCard :index="2" title="资产映射" :status="stepStates[1]"
          description="为连入的素材写清「参考什么 / 不参考什么」，两项都必须写。">
          <template #actions>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading" @click.stop="doAssets">生成</button>
            <button v-if="stepStates[1] === 'ready'" class="text-[10px] px-2 py-0.5 rounded bg-cyan-500/25 hover:bg-cyan-500/40 text-cyan-100"
              @click.stop="confirmGate('ASSET_CONFIRMED')">确认</button>
          </template>
          <AssetMappingList :assets="content.assets" :candidates="candidates" @update="a => persist({ assets: a })" />
        </GateCard>

        <!-- 3 故事板方式 -->
        <GateCard :index="3" title="故事板方式" :status="stepStates[2]"
          description="P0 仅支持「不使用故事板」，直接进入视频提示词。故事板分支留待后续版本。">
          <template #actions>
            <button v-if="stepStates[2] === 'ready'" class="text-[10px] px-2 py-0.5 rounded bg-cyan-500/25 hover:bg-cyan-500/40 text-cyan-100"
              @click.stop="confirmGate('MODE_CONFIRMED', '不使用故事板')">确认不使用</button>
            <span v-else-if="stepStates[2] === 'done'" class="text-[10px] text-emerald-200/80">不使用故事板</span>
          </template>
        </GateCard>

        <!-- 4 空间调度 -->
        <GateCard :index="4" title="空间调度" :status="stepStates[3]"
          description="判断多人关系、道具落点、遮挡、视线、运动轴线是否影响结果；不需要时给出理由。">
          <template #actions>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading" @click.stop="doSpatial">判断</button>
            <button v-if="stepStates[3] === 'ready'" class="text-[10px] px-2 py-0.5 rounded bg-cyan-500/25 hover:bg-cyan-500/40 text-cyan-100"
              @click.stop="confirmGate('SPATIAL_CONFIRMED')">确认</button>
          </template>
          <div v-if="content.spatial.reason || content.spatial.blocking" class="text-[11px] text-white/60 leading-relaxed space-y-1">
            <div :class="content.spatial.needed ? 'text-cyan-200' : 'text-white/45'">
              {{ content.spatial.needed ? '需要俯视调度图' : '无需俯视调度图' }}｜{{ content.spatial.reason }}
            </div>
            <div v-if="content.spatial.blocking" class="whitespace-pre-wrap">{{ content.spatial.blocking }}</div>
            <div v-if="content.spatial.axis" class="text-white/45 whitespace-pre-wrap">{{ content.spatial.axis }}</div>
          </div>
          <div v-else class="text-[11px] text-white/25">尚未判断</div>
        </GateCard>

        <!-- 5 工作段规划 -->
        <GateCard :index="5" title="工作段规划" :status="stepStates[4]"
          description="比较目标时长与忠实估算，拆分 SEG 工作段，产出文字分镜表与 EV/DLG 覆盖表。">
          <template #actions>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading" @click.stop="doPlan">生成</button>
            <button v-if="stepStates[4] === 'ready' && content.segments.length" class="text-[10px] px-2 py-0.5 rounded bg-cyan-500/25 hover:bg-cyan-500/40 text-cyan-100"
              @click.stop="confirmGate('PLAN_CONFIRMED')">确认规划</button>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading || content.segments.length === 0" title="把文字分镜表落地成画布上的表格节点"
              @click.stop="spawnToCanvas('plan')">落地分镜表</button>
          </template>
          <SegmentPlanTable :segments="content.segments" :ledger="content.ledger" :duration="content.duration"
            @update="s => persist({ segments: s })" />
        </GateCard>

        <!-- 6 视频提示词 -->
        <GateCard :index="6" title="视频提示词" :status="stepStates[5]"
          description="逐段产出 Seedance 2.5 提示词，然后跑结构校验；零 ERROR 才可确认。">
          <template #actions>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading || content.segments.length === 0" @click.stop="doPrompts">生成全部</button>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading || content.segments.length === 0" @click.stop="doValidate">校验</button>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading || !hasPrompt" title="每段提示词落地成一个文本节点，可复制给视频节点"
              @click.stop="spawnToCanvas('prompts')">落地提示词</button>
          </template>
          <div v-if="validation.errors.length || validation.warnings.length" class="mb-1.5 text-[10px]">
            <span class="text-red-300">{{ validation.errors.length }} 个错误</span>
            <span class="text-white/30"> · </span>
            <span class="text-amber-300">{{ validation.warnings.length }} 个警告</span>
          </div>
          <PromptViewer :segments="content.segments" :validation="validation"
            @update="s => persist({ segments: s })" />
        </GateCard>

        <!-- 7 交付导出 -->
        <GateCard :index="7" title="交付导出" :status="stepStates[6]"
          description="按 DELIVERY_FORMAT 渲染完整 Markdown 交付文件并下载。">
          <template #actions>
            <button class="text-[10px] px-2 py-0.5 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
              :disabled="loading || content.segments.length === 0" @click.stop="doExport">导出</button>
          </template>
        </GateCard>
      </div>
    </div>
  </div>
</template>
