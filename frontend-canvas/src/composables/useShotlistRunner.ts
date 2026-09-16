import { ref, computed } from 'vue'
import type { Node } from '../types'
import {
  parseShotlistContent, emptyShotlistContent,
  type ShotlistContent, type ShotlistSegment, type ShotlistStepInfo, type ShotlistSpawnRequest,
} from '../types/shotlist'
import {
  buildScriptSpawnItem, buildPlanSpawnItem, buildPromptSpawnItems,
} from './useShotlistSpawn'
import { runShotlistStep, validateShotlist, exportShotlist, fetchShotlistSteps } from '../api'
import type { ModelInfo } from './useNodeConfigs'

export interface PanelInput {
  edgeId: string
  sourceNodeId: string
  sourceNodeType: string
  sourceNodeName?: string
  data: any
}

export interface AssetCandidate {
  edgeId: string
  nodeId: string
  nodeType: string
  name: string
  url: string
}

/** 可落地到画布的产物种类。 */
export type SpawnKind = 'script' | 'plan' | 'prompts'

interface RunnerOptions {
  getNode: () => Node | null
  getInputs: () => PanelInput[]
  save: (content: string) => void
  resolveModel: () => { name: string; config: ModelInfo | null }
  /** 请求画布创建产物节点；创建完成后会回调 onSpawned。 */
  spawn: (req: ShotlistSpawnRequest) => void
}

/**
 * 分镜台的流程执行器。
 * 状态机推进严格遵循 SKILL.md：模型步骤只产出数据，状态推进由用户点确认触发。
 */
export function useShotlistRunner(opts: RunnerOptions) {
  const content = ref<ShotlistContent>(emptyShotlistContent())
  const steps = ref<ShotlistStepInfo[]>([])
  const loading = ref(false)
  const currentStep = ref('')
  const error = ref('')
  const progress = ref('')

  const state = computed(() => content.value.state)

  function load() {
    const node = opts.getNode()
    if (!node) return
    content.value = parseShotlistContent(node.content)
  }

  function persist(patch: Partial<ShotlistContent> = {}) {
    const next = { ...content.value, ...patch }
    content.value = next
    opts.save(JSON.stringify(next))
  }

  async function loadSteps() {
    if (steps.value.length > 0) return
    try {
      const res = await fetchShotlistSteps()
      steps.value = res.steps || []
    } catch {
      // 拿不到步骤定义时面板会退化为只读，不影响画布其他功能。
    }
  }

  /** 用户确认某个 Gate，推进状态机并记录确认时间。 */
  function confirmGate(gate: string, note = '') {
    persist({
      state: gate,
      gates: { ...content.value.gates, [gate]: { at: new Date().toISOString(), note } },
    })
  }

  // ─── 上游素材 ────────────────────────────────────────────────────

  /** 把连入的剧本表格/文本节点转成模型能读的正文。 */
  function extractScriptText(): string {
    const parts: string[] = []
    for (const input of opts.getInputs()) {
      if (input.sourceNodeType === 'table' && typeof input.data === 'string') {
        parts.push(tableToText(input.data))
      } else if (input.sourceNodeType === 'text' && typeof input.data === 'string') {
        parts.push(input.data)
      }
    }
    return parts.filter(Boolean).join('\n\n')
  }

  function tableToText(raw: string): string {
    try {
      const table = JSON.parse(raw)
      const cols: string[] = table.cols || []
      const rows: string[][] = table.rows || []
      const lines: string[] = []
      if (cols.length) lines.push(cols.filter(Boolean).join(' | '))
      for (const row of rows) {
        const cells = row.map(c => (c || '').replace(/\s+/g, ' ').trim()).filter(Boolean)
        if (cells.length) lines.push(cells.join(' | '))
      }
      return lines.join('\n')
    } catch {
      return raw
    }
  }

  function scriptSourceNodeId(): string {
    const hit = opts.getInputs().find(i => i.sourceNodeType === 'table' || i.sourceNodeType === 'text')
    return hit?.sourceNodeId || ''
  }

  /** 可作为视觉参考的连入素材。 */
  function collectAssetCandidates(): AssetCandidate[] {
    const out: AssetCandidate[] = []
    for (const input of opts.getInputs()) {
      if (input.sourceNodeType === 'asset' && input.data) {
        out.push({
          edgeId: input.edgeId, nodeId: input.sourceNodeId, nodeType: 'asset',
          name: input.data.name || '素材', url: input.data.url || input.data.dataUrl || '',
        })
      } else if (input.sourceNodeType === 'image' && input.data) {
        out.push({
          edgeId: input.edgeId, nodeId: input.sourceNodeId, nodeType: 'image',
          name: input.data.name || '参考图', url: input.data.dataUrl || '',
        })
      }
    }
    return out
  }

  // ─── 步骤执行 ────────────────────────────────────────────────────

  function buildStepInput(stepKey: string, extra: Record<string, any>) {
    const c = content.value
    const base = { project: c.project, script_text: extractScriptText(), script: c.script }
    switch (stepKey) {
      case 'lock_script':
        return { ...base, scope: extra.scope || '' }
      case 'assets':
        return {
          ...base,
          candidates: collectAssetCandidates().map(a => ({
            node_id: a.nodeId, name: a.name, node_type: a.nodeType,
          })),
        }
      case 'spatial':
        return { ...base, assets: c.assets }
      case 'plan':
        return { ...base, assets: c.assets, spatial: c.spatial }
      case 'prompts':
        return {
          ...base, assets: c.assets, spatial: c.spatial, ledger: c.ledger,
          segment: extra.segment, style_hint: extra.styleHint || '',
        }
      default:
        return base
    }
  }

  async function runLlmStep(stepKey: string, extra: Record<string, any> = {}) {
    if (loading.value) return null
    const step = steps.value.find(s => s.key === stepKey)
    const label = step?.label || stepKey

    loading.value = true
    currentStep.value = label
    error.value = ''
    persist({ _loading: true, _step: label, _error: '' })

    try {
      const { name: model, config } = opts.resolveModel()
      if (!model) throw new Error('请先选择模型')
      const res = await runShotlistStep({
        step: stepKey,
        input: buildStepInput(stepKey, extra),
        model,
        channel: config?.channel,
        base_url: config?.base_url,
        api_key: config?.api_key,
      })
      if (!res.parsed || !res.data) {
        throw new Error('模型没有返回可解析的 JSON，请重试或换一个模型')
      }
      applyStepResult(stepKey, res.data)
      return res.data
    } catch (e: any) {
      error.value = e?.message || String(e)
      persist({ _loading: false, _step: '', _error: error.value })
      return null
    } finally {
      loading.value = false
      currentStep.value = ''
    }
  }

  function applyStepResult(stepKey: string, data: any) {
    const clean = { _loading: false, _step: '', _error: '' }
    switch (stepKey) {
      case 'lock_script':
        persist({
          ...clean,
          script: { ...content.value.script, ...(data.script || {}), source_node_id: scriptSourceNodeId() },
          notes: data.notes || content.value.notes,
        })
        break
      case 'assets':
        persist({ ...clean, assets: Array.isArray(data.assets) ? data.assets : [] })
        break
      case 'spatial':
        persist({ ...clean, spatial: { ...content.value.spatial, ...(data.spatial || {}) } })
        break
      case 'plan':
        persist({
          ...clean,
          segments: (data.segments || []).map(normalizeSegment),
          ledger: Array.isArray(data.ledger) ? data.ledger : [],
          duration: { ...content.value.duration, ...(data.duration || {}) },
        })
        break
      case 'prompts': {
        const incoming: any[] = data.segments || []
        persist({
          ...clean,
          segments: content.value.segments.map(seg => {
            const hit = incoming.find(s => s.id === seg.id)
            return hit?.prompt ? { ...seg, prompt: hit.prompt } : seg
          }),
        })
        break
      }
    }
  }

  /** 逐段生成提示词。串行调用，避免并发把上游打满。 */
  async function runAllPrompts(styleHint: string) {
    const total = content.value.segments.length
    if (total === 0) return false
    for (let i = 0; i < total; i++) {
      const seg = content.value.segments[i]
      progress.value = `提示词 ${i + 1}/${total}`
      const ok = await runLlmStep('prompts', { segment: seg, styleHint })
      if (!ok) return false
    }
    progress.value = ''
    return true
  }

  async function revalidate() {
    try {
      const result = await validateShotlist(JSON.stringify(content.value))
      const segments = content.value.segments.map(seg => ({
        ...seg,
        validation: {
          errors: result.errors.filter(e => e.where === seg.id),
          warnings: result.warnings.filter(w => w.where === seg.id),
        },
      }))
      persist({ segments })
      return result
    } catch (e: any) {
      error.value = e?.message || String(e)
      return null
    }
  }

  async function exportMarkdown() {
    try {
      return await exportShotlist(JSON.stringify(content.value))
    } catch (e: any) {
      error.value = e?.message || String(e)
      return null
    }
  }

  // ─── 产物落地 ────────────────────────────────────────────────────

  /** 按产物种类取出待落地的节点载荷。 */
  function buildSpawnItems(kind: SpawnKind) {
    const c = content.value
    if (kind === 'script') {
      const item = buildScriptSpawnItem(c)
      return item ? [item] : []
    }
    if (kind === 'plan') {
      const item = buildPlanSpawnItem(c)
      return item ? [item] : []
    }
    return buildPromptSpawnItems(c)
  }

  /**
   * 把产物落地成画布上的真实节点并连线。
   * 已落地过的产物会原地更新（沿原本地 id），不会重复新建。
   * 落地后由画布把节点 id 写回本节点 content，面板重新加载即可看到。
   */
  function spawnToCanvas(kind: SpawnKind): boolean {
    const node = opts.getNode()
    if (!node) return false
    const items = buildSpawnItems(kind)
    if (!items.length) {
      error.value = kind === 'prompts' ? '还没有生成任何提示词' : '还没有可落地的产物'
      return false
    }
    error.value = ''
    opts.spawn({ anchorNodeId: node.id, items })
    return true
  }

  return {
    content, steps, loading, currentStep, error, progress, state,
    load, persist, loadSteps, confirmGate,
    extractScriptText, scriptSourceNodeId, collectAssetCandidates,
    runLlmStep, runAllPrompts, revalidate, exportMarkdown, spawnToCanvas,
  }
}

function normalizeSegment(raw: any): ShotlistSegment {
  return {
    id: raw?.id || '',
    title: raw?.title || '',
    duration_s: Number(raw?.duration_s) || 0,
    form: raw?.form === 'one_take' ? 'one_take' : 'multi_shot',
    coverage: { EV: raw?.coverage?.EV || [], DLG: raw?.coverage?.DLG || [] },
    shots: (raw?.shots || []).map((s: any, i: number) => ({
      idx: Number(s?.idx) || i + 1,
      t: s?.t || '',
      action: s?.action || '',
      composition: s?.composition || '',
      camera: s?.camera || '',
      performance: s?.performance || '',
      dialogue: s?.dialogue || '',
      sound: s?.sound || '',
      transition: s?.transition || '',
    })),
    // P0 只覆盖「不使用故事板」分支（SKILL.md 5A）。
    storyboard: raw?.storyboard || { mode: 'skip', rows: 0, cols: 0, n: 0 },
    prompt: raw?.prompt || '',
  }
}
