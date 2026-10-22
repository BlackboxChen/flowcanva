import { ref } from 'vue'

export type SeedMode = 'workflow' | 'random' | 'fixed'
const seedKeys = new Set(['seed', 'noise_seed', 'random_seed'])

// Rewrite seed literals only: arrays can be ComfyUI node links and must stay intact.
function mapSeeds(value: any, seed: number, placeholdersOnly = false): any {
  if (Array.isArray(value)) return value.map(item => mapSeeds(item, seed, placeholdersOnly))
  if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([key, item]) => [
    key, seedKeys.has(key) && (typeof item === 'number' || typeof item === 'string') && (!placeholdersOnly || item === '{{random_seed}}') ? seed : mapSeeds(item, seed, placeholdersOnly),
  ]))
  return value
}

function firstSeed(graph: any): number | null {
  if (!graph || typeof graph !== 'object') return null
  for (const [key, value] of Object.entries(graph)) {
    if (seedKeys.has(key) && typeof value === 'number') return value
    const found = firstSeed(value)
    if (found !== null) return found
  }
  return null
}

export function useWorkflowSeed() {
  const seedMode = ref<SeedMode>('workflow')
  const fixedSeed = ref('0')
  const lastSeed = ref<number | null>(null)
  function loadSeed(data: Record<string, any>) {
    seedMode.value = ['random', 'fixed', 'workflow'].includes(data.seedMode) ? data.seedMode : String(data.workflowJson || '').includes('{{random_seed}}') ? 'random' : 'workflow'
    let fallback = 0
    try { fallback = firstSeed(JSON.parse(data.workflowJson || '{}')) ?? 0 } catch { /* invalid JSON is handled on execute */ }
    fixedSeed.value = String(data.fixedSeed ?? fallback)
    lastSeed.value = typeof data.lastSeed === 'number' ? data.lastSeed : null
  }
  function seedState() { return { seedMode: seedMode.value, fixedSeed: fixedSeed.value, lastSeed: lastSeed.value } }
  function prepareSeed(graph: any) {
    if (seedMode.value === 'workflow') return { workflow: mapSeeds(graph, Number(fixedSeed.value), true), seed: null }
    let seed = Number(fixedSeed.value)
    if (seedMode.value === 'random') {
      // A positive 31-bit integer is exact in JSON and compatible with both samplers.
      do { seed = (crypto.getRandomValues(new Uint32Array(1))[0]! & 0x7fffffff) || 1 } while (seed === lastSeed.value)
    }
    if (!Number.isSafeInteger(seed) || seed < 0 || fixedSeed.value.trim() === '' && seedMode.value === 'fixed') throw new Error('固定种子必须是非负安全整数')
    return { workflow: mapSeeds(graph, seed), seed }
  }
  return { seedMode, fixedSeed, lastSeed, loadSeed, seedState, prepareSeed }
}
