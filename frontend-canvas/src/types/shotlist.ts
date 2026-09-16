// 分镜台节点 content 的 JSON 结构，与后端 shotlist_types.go 保持一致。

export interface ShotlistEV { id: string; text: string }
export interface ShotlistDLG { id: string; speaker: string; text: string }
export interface ShotlistLOCK { id: string; fact: string }

export interface ShotlistAsset {
  handle: string
  node_id: string
  name?: string
  role?: string
  use: string
  not_use: string
}

export interface ShotlistShot {
  idx: number
  t: string
  action: string
  composition: string
  camera: string
  performance: string
  dialogue: string
  sound: string
  transition: string
  storyboard_ref?: string
}

export interface ShotlistCoverage {
  EV: string[]
  DLG: string[]
}

export interface ShotlistStoryboard {
  mode: 'use' | 'skip'
  rows: number
  cols: number
  n: number
  node_id?: string
  review?: 'pending' | 'passed' | 'rejected'
}

export interface ShotlistIssue {
  level: 'ERROR' | 'WARNING'
  code: string
  where: string
  message: string
}

export interface ShotlistValidation {
  errors: ShotlistIssue[]
  warnings: ShotlistIssue[]
}

export interface ShotlistSegment {
  id: string
  title: string
  duration_s: number
  form: 'multi_shot' | 'one_take' | ''
  coverage: ShotlistCoverage
  shots: ShotlistShot[]
  storyboard?: ShotlistStoryboard
  prompt: string
  prompt_node_id?: string
  video_node_id?: string
  validation?: ShotlistValidation
}

export interface ShotlistDuration {
  target_s: number
  faithful_estimate_s: number
  verdict: 'ok' | 'insufficient' | ''
  note: string
}

export interface ShotlistSpatial {
  needed: boolean
  reason: string
  blocking: string
  axis: string
}

export interface ShotlistLedger {
  from: string
  to: string
  inherit: string[]
}

export interface ShotlistContent {
  version: string
  project: { title: string; target_platform: string; target_duration_s: number }
  state: string
  gates: Record<string, any>
  script: {
    source_node_id?: string
    version: string
    EV: ShotlistEV[]
    DLG: ShotlistDLG[]
    LOCK: ShotlistLOCK[]
  }
  assets: ShotlistAsset[]
  duration: ShotlistDuration
  spatial: ShotlistSpatial
  segments: ShotlistSegment[]
  ledger: ShotlistLedger[]
  notes: string
  /**
   * 已落地到画布的产物节点 id，键为产物键（`script` / `plan`）。
   * 工作段提示词节点不走这里，用 segment.prompt_node_id（与方案文档一致）。
   * 存 id 是为了「再点一次原地更新」而不是重复新建。
   */
  spawned?: Record<string, string>
  _loading?: boolean
  _step?: string
  _error?: string
}

// ─── 产物落地（分镜台 → 画布节点） ──────────────────────────────────

/** 分镜台请求画布创建/更新一个产物节点。 */
export interface ShotlistSpawnItem {
  /** 回写用的键：`script` / `plan` / 工作段 id。 */
  key: string
  type: 'text' | 'table'
  /** 节点 content 原文（table 为 JSON 字符串，text 为纯文本）。 */
  content: string
  /** 已落地过的节点 id；存在则原地更新，不新建。 */
  existingNodeId?: string
}

export interface ShotlistSpawnRequest {
  /** 以哪个节点为锚点定位新节点，同时作为连线起点。 */
  anchorNodeId: string
  items: ShotlistSpawnItem[]
}

export interface ShotlistStepInfo {
  key: string
  label: string
  gate: string
  kind: 'llm' | 'manual'
  resources: string[] | null
  description: string
}

// 状态机顺序，与后端 shotlistStateOrder 一致。
export const SHOTLIST_STATES = [
  'SOURCE_READY',
  'SCRIPT_LOCKED',
  'ASSET_CONFIRMED',
  'MODE_CONFIRMED',
  'SPATIAL_CONFIRMED',
  'PLAN_CONFIRMED',
  'STORYBOARD_CONFIRMED',
  'PROMPTS_VALIDATED',
  'DELIVERED',
] as const

export const SHOTLIST_STATE_LABELS: Record<string, string> = {
  SOURCE_READY: '待锁定剧本',
  SCRIPT_LOCKED: '剧本已锁定',
  ASSET_CONFIRMED: '资产已确认',
  MODE_CONFIRMED: '故事板方式已确认',
  SPATIAL_CONFIRMED: '空间调度已确认',
  PLAN_CONFIRMED: '分镜规划已确认',
  STORYBOARD_CONFIRMED: '故事板已确认',
  PROMPTS_VALIDATED: '提示词已校验',
  DELIVERED: '已交付',
}

/** 当前状态是否已达到或越过目标状态。 */
export function stateAtLeast(current: string, target: string): boolean {
  const cur = SHOTLIST_STATES.indexOf(current as any)
  const want = SHOTLIST_STATES.indexOf(target as any)
  if (cur < 0 || want < 0) return false
  return cur >= want
}

export function emptyShotlistContent(): ShotlistContent {
  return {
    version: '3.7',
    project: { title: '', target_platform: 'Seedance 2.5', target_duration_s: 60 },
    state: 'SOURCE_READY',
    gates: {},
    script: { version: 'v1', EV: [], DLG: [], LOCK: [] },
    assets: [],
    duration: { target_s: 0, faithful_estimate_s: 0, verdict: '', note: '' },
    spatial: { needed: false, reason: '', blocking: '', axis: '' },
    segments: [],
    ledger: [],
    notes: '',
    spawned: {},
  }
}

/** 容错解析节点 content，任何损坏都退回空结构，避免面板白屏。 */
export function parseShotlistContent(raw: string): ShotlistContent {
  const base = emptyShotlistContent()
  if (!raw) return base
  try {
    const parsed = JSON.parse(raw)
    return {
      ...base,
      ...parsed,
      project: { ...base.project, ...(parsed.project || {}) },
      script: { ...base.script, ...(parsed.script || {}) },
      duration: { ...base.duration, ...(parsed.duration || {}) },
      spatial: { ...base.spatial, ...(parsed.spatial || {}) },
      assets: Array.isArray(parsed.assets) ? parsed.assets : [],
      segments: Array.isArray(parsed.segments) ? parsed.segments : [],
      ledger: Array.isArray(parsed.ledger) ? parsed.ledger : [],
      gates: parsed.gates && typeof parsed.gates === 'object' ? parsed.gates : {},
      spawned: parsed.spawned && typeof parsed.spawned === 'object' ? parsed.spawned : {},
    }
  } catch {
    return base
  }
}
