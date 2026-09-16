// 分镜台产物落地到画布：把 content 里的清单/表格/提示词转成真实节点，并回写节点 id。
//
// 只做「数据 → 节点载荷」的纯转换，不碰画布状态；真正的创建与连线由 App.vue 的
// spawnNode 完成。这样分镜台的流程逻辑与画布的节点操作互不耦合。

import type {
  ShotlistContent, ShotlistSegment, ShotlistSpawnItem,
} from '../types/shotlist'
import { parseShotlistContent } from '../types/shotlist'

/** 文字分镜表的列头，顺序即 SKILL.md 的分镜表字段顺序。 */
const PLAN_COLUMNS = [
  '工作段', '镜号', '时间', '画面描述', '构图', '机位', '表演', '对白', '声音', '转场',
]

/** 把 {cols, rows} 序列化成 table 节点的 content。 */
function tableContent(cols: string[], rows: string[][]): string {
  return JSON.stringify({ cols, colWidths: [], rows })
}

/** 单元格里的换行会把表格撑坏，统一压成单行。 */
function cell(value: string | undefined): string {
  return (value || '').replace(/\s+/g, ' ').trim()
}

/**
 * 剧本锁定产物 → 一张三列清单表。
 * EV/DLG/LOCK 合成一张表，避免落地三个节点；DLG 的说话人并进内容列。
 */
export function buildScriptSpawnItem(content: ShotlistContent): ShotlistSpawnItem | null {
  const { EV, DLG, LOCK } = content.script
  if (!EV.length && !DLG.length && !LOCK.length) return null

  const rows: string[][] = []
  for (const ev of EV) rows.push(['事件 EV', ev.id, cell(ev.text)])
  for (const dlg of DLG) {
    const text = dlg.speaker ? `${dlg.speaker}：${cell(dlg.text)}` : cell(dlg.text)
    rows.push(['台词 DLG', dlg.id, text])
  }
  for (const lk of LOCK) rows.push(['锁定 LOCK', lk.id, cell(lk.fact)])

  return {
    key: 'script',
    type: 'table',
    content: tableContent(['类型', '编号', '内容'], rows),
    existingNodeId: content.spawned?.script,
  }
}

/** 工作段规划产物 → 一张文字分镜表。 */
export function buildPlanSpawnItem(content: ShotlistContent): ShotlistSpawnItem | null {
  if (!content.segments.length) return null

  const rows: string[][] = []
  for (const seg of content.segments) {
    for (const shot of seg.shots) {
      rows.push([
        `${seg.id}｜${cell(seg.title)}`,
        String(shot.idx),
        cell(shot.t),
        cell(shot.action),
        cell(shot.composition),
        cell(shot.camera),
        cell(shot.performance),
        cell(shot.dialogue),
        cell(shot.sound),
        cell(shot.transition),
      ])
    }
  }
  if (!rows.length) return null

  return {
    key: 'plan',
    type: 'table',
    content: tableContent(PLAN_COLUMNS, rows),
    existingNodeId: content.spawned?.plan,
  }
}

/** 每个已生成提示词的工作段 → 一个文本节点。 */
export function buildPromptSpawnItems(content: ShotlistContent): ShotlistSpawnItem[] {
  return content.segments
    .filter(seg => seg.prompt.trim().length > 0)
    .map(seg => ({
      key: seg.id,
      type: 'text' as const,
      content: promptNodeText(seg),
      existingNodeId: seg.prompt_node_id,
    }))
}

/** 文本节点里带上标题，方便在画布上一眼认出是哪一段。 */
function promptNodeText(seg: ShotlistSegment): string {
  const title = seg.title ? `｜${seg.title}` : ''
  return `${seg.id}${title}（${seg.duration_s} 秒）\n\n${seg.prompt.trim()}`
}

/**
 * 把落地得到的节点 id 写回 content。
 * 返回可直接交给 persist() 的补丁；没有命中任何产物时返回 null。
 */
export function applySpawnMapping(
  content: ShotlistContent,
  mapping: Record<string, string>,
): Partial<ShotlistContent> | null {
  const entries = Object.entries(mapping)
  if (!entries.length) return null

  const spawned = { ...(content.spawned || {}) }
  let segments = content.segments
  let touched = false

  for (const [key, nodeId] of entries) {
    if (!nodeId) continue
    if (key === 'script' || key === 'plan') {
      spawned[key] = nodeId
      touched = true
      continue
    }
    // 工作段 id：写进对应 segment 的 prompt_node_id。
    if (segments.some(seg => seg.id === key)) {
      segments = segments.map(seg => (seg.id === key ? { ...seg, prompt_node_id: nodeId } : seg))
      touched = true
    }
  }
  if (!touched) return null
  return { spawned, segments }
}

/**
 * 画布侧用的便捷入口：直接吃节点 content 原文，吐出写回后的 content 原文。
 * 没有需要写入的映射时返回 null，调用方据此跳过保存。
 */
export function mergeSpawnMapping(
  rawContent: string,
  mapping: Record<string, string>,
): string | null {
  const patch = applySpawnMapping(parseShotlistContent(rawContent), mapping)
  if (!patch) return null
  return JSON.stringify({ ...parseShotlistContent(rawContent), ...patch })
}
