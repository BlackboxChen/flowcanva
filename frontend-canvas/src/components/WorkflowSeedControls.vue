<script setup lang="ts">
import type { SeedMode } from '../composables/useWorkflowSeed'
defineProps<{ mode: SeedMode; fixedSeed: string; lastSeed: number | null }>()
const emit = defineEmits<{
  (event: 'update:mode', value: SeedMode): void
  (event: 'update:fixedSeed', value: string): void
  (event: 'change'): void
}>()
function changeMode(event: Event) {
  emit('update:mode', (event.target as HTMLSelectElement).value as SeedMode)
  emit('change')
}
function changeFixed(event: Event) {
  emit('update:fixedSeed', (event.target as HTMLInputElement).value)
  emit('change')
}
</script>

<template>
  <div class="space-y-1 rounded border border-cyan-500/20 p-1.5 text-[10px] text-white/70">
    <label class="flex items-center gap-2">种子模式
      <select :value="mode" aria-label="种子模式" class="rounded bg-neutral-800 px-2 py-1" @change="changeMode">
        <option value="random">每次执行随机</option>
        <option value="fixed">固定种子</option>
        <option value="workflow">工作流原值</option>
      </select>
    </label>
    <div v-if="mode === 'random'" class="text-cyan-300/80">随机模式已启用：提交时替换 seed，不使用 JSON 原种子。</div>
    <input v-if="mode === 'fixed'" :value="fixedSeed" aria-label="固定种子" inputmode="numeric" class="w-full rounded bg-neutral-800 px-2 py-1" @change="changeFixed" />
    <div v-if="lastSeed !== null" class="break-all text-cyan-300/80">上次提交种子：{{ lastSeed }}</div>
    <div v-else class="text-white/40">新任务提交后记录实际种子；查询任务不会换种子。</div>
  </div>
</template>
