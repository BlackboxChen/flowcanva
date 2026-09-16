<script setup lang="ts">
// 资产映射：把画布上的连线翻译成 @图片N 句柄，并写清参考/不参考边界。
import type { ShotlistAsset } from '../../types/shotlist'
import type { AssetCandidate } from '../../composables/useShotlistRunner'

const props = defineProps<{
  assets: ShotlistAsset[]
  candidates: AssetCandidate[]
}>()

const emit = defineEmits<{
  (e: 'update', assets: ShotlistAsset[]): void
}>()

function update(i: number, patch: Partial<ShotlistAsset>) {
  emit('update', props.assets.map((a, idx) => (idx === i ? { ...a, ...patch } : a)))
}

function candidateFor(nodeId: string): AssetCandidate | undefined {
  return props.candidates.find(c => c.nodeId === nodeId)
}
</script>

<template>
  <div class="space-y-2">
    <div v-if="assets.length === 0" class="text-[11px] text-white/30 leading-relaxed">
      未使用任何资产图。后续提示词会完全省略素材句柄与占位，一致性风险更高。
    </div>

    <div
      v-for="(asset, i) in assets"
      :key="asset.node_id || i"
      class="rounded-lg border border-white/10 bg-white/[0.03] p-2"
    >
      <div class="flex items-center gap-2">
        <img
          v-if="candidateFor(asset.node_id)?.url"
          :src="candidateFor(asset.node_id)!.url"
          class="w-8 h-8 rounded object-cover border border-white/10 shrink-0"
          alt=""
        />
        <div v-else class="w-8 h-8 rounded bg-white/5 border border-white/10 shrink-0" />
        <span class="text-[11px] text-cyan-200 shrink-0">{{ asset.handle }}</span>
        <span class="text-[11px] text-white/60 truncate">{{ asset.name || candidateFor(asset.node_id)?.name || asset.node_id }}</span>
        <span v-if="asset.role" class="ml-auto text-[10px] text-white/30 shrink-0">{{ asset.role }}</span>
      </div>
      <div class="mt-1.5 space-y-1">
        <div class="flex items-start gap-1.5">
          <span class="text-[10px] text-emerald-300/70 w-8 shrink-0 pt-1">参考</span>
          <textarea
            :value="asset.use"
            rows="1"
            class="flex-1 min-w-0 bg-white/5 border border-white/10 rounded px-1.5 py-1 text-[11px] text-white/80 outline-none resize-y focus:border-cyan-400/40"
            @input="update(i, { use: ($event.target as HTMLTextAreaElement).value })"
          />
        </div>
        <div class="flex items-start gap-1.5">
          <span class="text-[10px] text-rose-300/70 w-8 shrink-0 pt-1">不参考</span>
          <textarea
            :value="asset.not_use"
            rows="1"
            class="flex-1 min-w-0 bg-white/5 border border-white/10 rounded px-1.5 py-1 text-[11px] text-white/80 outline-none resize-y focus:border-cyan-400/40"
            @input="update(i, { not_use: ($event.target as HTMLTextAreaElement).value })"
          />
        </div>
      </div>
    </div>

    <div v-if="candidates.length > 0" class="text-[10px] text-white/30 pt-1">
      已连入 {{ candidates.length }} 项素材：
      <span v-for="c in candidates" :key="c.nodeId" class="mr-1.5">{{ c.name }}</span>
    </div>
  </div>
</template>
