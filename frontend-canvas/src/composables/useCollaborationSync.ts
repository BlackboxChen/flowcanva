import { onUnmounted, ref, type Ref } from 'vue'
import type { Node, Edge, Canvas } from '../types'
import { nodeBases, savingNodes, saveProblems, rememberNode } from './collaborationState'

export function useCollaborationSync(nodes: Ref<Node[]>, edges: Ref<Edge[]>, canvasId: Ref<string | null>, selected: Ref<string | null>) {
  const message = ref('协作同步启动中')
  let busy = false
  async function sync() {
    if (!canvasId.value || busy || document.hidden) return
    // Don't move the canvas under a drag or active editor.
    busy = true
    try {
      const res = await fetch(`/api/canvases/${canvasId.value}`)
      if (!res.ok) throw new Error('同步失败')
      const remote: Canvas = await res.json()
      const ids = new Set(remote.nodes.map(n => n.id))
      for (const incoming of remote.nodes) {
        const local = nodes.value.find(n => n.id === incoming.id)
        if (savingNodes.has(incoming.id) || saveProblems[incoming.id]) continue
        if (local && selected.value === incoming.id) {
          if (nodeBases.get(incoming.id)?.content !== incoming.content) message.value = '当前节点有远端更新；编辑内容已保留，可备份草稿后加载最新版本'
          continue
        }
        if (local) Object.assign(local, incoming)
        else nodes.value.push(incoming)
        rememberNode(incoming)
      }
      nodes.value = nodes.value.filter(n => ids.has(n.id) || selected.value === n.id || savingNodes.has(n.id) || saveProblems[n.id])
      if (!savingNodes.size && !pointerDown) edges.value = remote.edges || []
      if (!selected.value) message.value = '协作同步正常（每 5 秒）'
    } catch { message.value = '同步暂不可用；本地编辑仍保留' }
    finally { busy = false }
  }
  async function reloadSelected() {
    const local = nodes.value.find(n => n.id === selected.value)
    if (!local || savingNodes.has(local.id)) return
    const blob = new Blob([JSON.stringify(local, null, 2)], {type:'application/json'})
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a'); a.href = url; a.download = `${local.id}-draft.json`; a.click(); URL.revokeObjectURL(url)
    const res = await fetch(`/api/canvases/${canvasId.value}`)
    if (!res.ok) { message.value = '草稿已导出，加载远端失败'; return }
    const remote: Canvas = await res.json()
    const current = remote.nodes.find(n => n.id === local.id)
    selected.value = null
    delete saveProblems[local.id]
    if (current) { Object.assign(local,current); rememberNode(current) }
    else nodes.value = nodes.value.filter(n => n.id !== local.id)
    message.value = '草稿已导出，远端版本已加载；重新点击节点可编辑'
  }
  let pointerDown = false
  const down = () => { pointerDown = true }
  const up = () => { pointerDown = false }
  window.addEventListener('pointerdown',down); window.addEventListener('pointerup',up)
  const timer = setInterval(() => { if (!pointerDown) void sync() },5000)
  onUnmounted(() => {clearInterval(timer);window.removeEventListener('pointerdown',down);window.removeEventListener('pointerup',up)})
  return { syncMessage: message, reloadSelected, saveProblems }
}
