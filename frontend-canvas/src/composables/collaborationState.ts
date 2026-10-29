import { reactive } from 'vue'
import type { Node } from '../types'

export const nodeBases = new Map<string, Node>()
export const savingNodes = new Set<string>()
export const saveProblems = reactive<Record<string, string>>({})
export function rememberNode(node: Node) { nodeBases.set(node.id, { ...node }) }
