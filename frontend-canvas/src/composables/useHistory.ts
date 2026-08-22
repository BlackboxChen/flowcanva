import { ref, computed, type Ref } from 'vue'
import type { Node, Edge } from '../types'

interface Snapshot {
  nodes: Node[]
  edges: Edge[]
}

const MAX_HISTORY_ENTRIES = 30

export function useHistory(nodes: Ref<Node[]>, edges: Ref<Edge[]>) {
  const undoStack = ref<Snapshot[]>([])
  const redoStack = ref<Snapshot[]>([])

  const canUndo = computed(() => undoStack.value.length > 0)
  const canRedo = computed(() => redoStack.value.length > 0)

  function clone(n: Node[], e: Edge[]): Snapshot {
    return {
      nodes: JSON.parse(JSON.stringify(n)),
      edges: JSON.parse(JSON.stringify(e)),
    }
  }

  function push() {
    undoStack.value.push(clone(nodes.value, edges.value))
    if (undoStack.value.length > MAX_HISTORY_ENTRIES) {
      undoStack.value.splice(0, undoStack.value.length - MAX_HISTORY_ENTRIES)
    }
    redoStack.value = []
  }

  function undo() {
    const snapshot = undoStack.value.pop()
    if (!snapshot) return
    redoStack.value.push(clone(nodes.value, edges.value))
    if (redoStack.value.length > MAX_HISTORY_ENTRIES) {
      redoStack.value.splice(0, redoStack.value.length - MAX_HISTORY_ENTRIES)
    }
    nodes.value = snapshot.nodes
    edges.value = snapshot.edges
  }

  function redo() {
    const snapshot = redoStack.value.pop()
    if (!snapshot) return
    undoStack.value.push(clone(nodes.value, edges.value))
    if (undoStack.value.length > MAX_HISTORY_ENTRIES) {
      undoStack.value.splice(0, undoStack.value.length - MAX_HISTORY_ENTRIES)
    }
    nodes.value = snapshot.nodes
    edges.value = snapshot.edges
  }

  return { canUndo, canRedo, push, undo, redo }
}
