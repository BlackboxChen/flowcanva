import { ref, computed, reactive } from 'vue'
import type { Node, Edge } from '../types'
import { fetchCanvas, createCanvas, updateCanvas, createNode, updateNode, deleteNode } from '../api'

// 生成任务脱离页面后无法继续轮询，超过两小时仍未结束就视为已超时。
export const VIDEO_TASK_TIMEOUT_MS = 2 * 60 * 60 * 1000

function parseTimestamp(value: unknown): number {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value !== 'string' || !value.trim()) return NaN
  const parsed = Date.parse(value.includes('T') ? value : value.replace(' ', 'T'))
  return Number.isFinite(parsed) ? parsed : NaN
}

function staleVideoTaskContent(node: Node): string | null {
  if (node.node_type !== 'video') return null
  let data: any
  try { data = JSON.parse(node.content || '{}') } catch { return null }
  if (data._loading !== true) return null

  // 新任务使用明确的开始时间；历史任务回退到节点最后更新时间。
  const startedAt = parseTimestamp(data._loading_started_at)
  const fallbackAt = parseTimestamp(node.updated_at)
  const referenceAt = Number.isFinite(startedAt) ? startedAt : fallbackAt
  if (!Number.isFinite(referenceAt) || Date.now() - referenceAt < VIDEO_TASK_TIMEOUT_MS) return null

  return JSON.stringify({
    ...data,
    _loading: false,
    _loading_started_at: null,
    _error: '视频任务超时（超过2小时未完成）',
  })
}

export function useNodes() {
  const nodes = ref<Node[]>([])
  const selectedNodeId = ref<string | null>(null)
  const canvasId = ref<string | null>(null)
  const canvasName = ref('')
  const nodeZIndices = reactive<Record<string, number>>({})
  let zCounter = 0

  const selectedNode = computed(() =>
    nodes.value.find(n => n.id === selectedNodeId.value) ?? null
  )

  async function expireStaleVideoTasks() {
    if (!canvasId.value) return
    const stale = nodes.value
      .map(node => ({ node, content: staleVideoTaskContent(node) }))
      .filter((item): item is { node: Node; content: string } => Boolean(item.content))
    if (stale.length === 0) return

    await Promise.all(stale.map(async ({ node, content }) => {
      node.content = content
      try { await updateNode(canvasId.value!, node.id, { content }) } catch { /* 本地状态仍显示超时，下次继续尝试持久化 */ }
    }))
  }

  async function loadOrCreateCanvas(preferredId?: string) {
    const res = await fetch('/api/canvases?page_size=100')
    const data = await res.json()
    const canvases = data.items || data

    if (preferredId && canvases.some((c: any) => c.id === preferredId)) {
      canvasId.value = preferredId
    } else if (canvases.length > 0) {
      canvasId.value = canvases[0].id
    } else {
      const cv = await createCanvas('未命名画布')
      canvasId.value = cv.id
    }

    const cv = await fetchCanvas(canvasId.value!)
    canvasName.value = cv.name
    nodes.value = cv.nodes ?? []
    await expireStaleVideoTasks()
    for (const n of nodes.value) {
      if (!(n.id in nodeZIndices)) nodeZIndices[n.id] = ++zCounter
    }
    return cv.edges ?? []
  }

  async function renameCanvas(name: string) {
    if (!canvasId.value) return
    await updateCanvas(canvasId.value, name)
    canvasName.value = name
  }

  async function addNode(
    type: Node['node_type'],
    wx: number,
    wy: number,
  ) {
    if (!canvasId.value) return
    const defaults: Record<string, { w: number; h: number; color: string }> = {
      text: { w: 480, h: 270, color: '#FEF3C7' },
      image: { w: 400, h: 300, color: '#DBEAFE' },
      video: { w: 480, h: 320, color: '#EDE9FE' },
      table: { w: 960, h: 540, color: '#D1FAE5' },
      full_image: { w: 420, h: 280, color: '#FEE2E2' },
      agent: { w: 480, h: 270, color: '#E0E7FF' },
      workflow: { w: 360, h: 260, color: '#FCE7F3' },
      asset: { w: 320, h: 300, color: '#FEF9E7' },
      director: { w: 480, h: 340, color: '#E8F5E9' },
    }
    const d = defaults[type]
    const n = await createNode(canvasId.value, {
      node_type: type,
      x: wx - d.w / 2,
      y: wy - d.h / 2,
      width: d.w,
      height: d.h,
      content: '',
      config: '{}',
    })
    nodes.value.push(n)
    nodeZIndices[n.id] = ++zCounter
    selectedNodeId.value = n.id
    return n
  }

  async function moveNode(id: string, x: number, y: number) {
    const node = nodes.value.find(n => n.id === id)
    if (node) {
      node.x = x
      node.y = y
    }
    if (!canvasId.value) return
    await updateNode(canvasId.value, id, { x, y })
  }

  async function resizeNode(id: string, width: number, height: number, x: number, y: number) {
    const node = nodes.value.find(n => n.id === id)
    if (node) {
      node.width = width
      node.height = height
      node.x = x
      node.y = y
    }
    if (!canvasId.value) return
    await updateNode(canvasId.value, id, { width, height, x, y })
  }

  async function updateNodeContent(id: string, content: string) {
    const node = nodes.value.find(n => n.id === id)
    if (node) {
      node.content = content
    }
    if (!canvasId.value) return
    try { await updateNode(canvasId.value, id, { content }) } catch { /* ignore save errors during stream */ }
  }

  async function updateNodeConfig(id: string, config: string) {
    const node = nodes.value.find(n => n.id === id)
    if (node) node.config = config
    if (!canvasId.value) return
    await updateNode(canvasId.value, id, { config })
  }

  async function removeNode(id: string) {
    if (!canvasId.value) return
    await deleteNode(canvasId.value, id)
    nodes.value = nodes.value.filter(n => n.id !== id)
    if (selectedNodeId.value === id) {
      selectedNodeId.value = null
    }
  }

  function selectNode(id: string | null) {
    selectedNodeId.value = id
    if (id) nodeZIndices[id] = ++zCounter
  }

  return {
    nodes,
    selectedNodeId,
    selectedNode,
    canvasId,
    canvasName,
    nodeZIndices,
    loadOrCreateCanvas,
    renameCanvas,
    addNode,
    moveNode,
    resizeNode,
    updateNodeContent,
    updateNodeConfig,
    removeNode,
    selectNode,
    expireStaleVideoTasks,
  }
}
