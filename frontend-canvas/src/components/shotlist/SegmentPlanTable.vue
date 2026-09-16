<script setup lang="ts">
// 工作段规划结果：时长判断 + 每段文字分镜表 + 跨段连续性账本。
import type { ShotlistSegment, ShotlistLedger, ShotlistDuration } from '../../types/shotlist'

const props = defineProps<{
  segments: ShotlistSegment[]
  ledger: ShotlistLedger[]
  duration: ShotlistDuration
}>()

const emit = defineEmits<{
  (e: 'update', segments: ShotlistSegment[]): void
}>()

function update(i: number, patch: Partial<ShotlistSegment>) {
  emit('update', props.segments.map((s, idx) => (idx === i ? { ...s, ...patch } : s)))
}

function formLabel(form: string) {
  if (form === 'one_take') return '一镜到底'
  if (form === 'multi_shot') return '多镜头'
  return '—'
}
</script>

<template>
  <div class="space-y-3">
    <div
      v-if="duration.target_s || duration.faithful_estimate_s"
      class="rounded-lg border px-2.5 py-2 text-[11px] leading-relaxed"
      :class="duration.verdict === 'insufficient'
        ? 'border-amber-400/40 bg-amber-400/[0.08] text-amber-200'
        : 'border-white/10 bg-white/[0.03] text-white/60'"
    >
      <div>目标 {{ duration.target_s }} 秒 · 忠实估算 {{ duration.faithful_estimate_s }} 秒</div>
      <div v-if="duration.note" class="mt-0.5 opacity-80">{{ duration.note }}</div>
    </div>

    <div
      v-for="(seg, i) in segments"
      :key="seg.id"
      class="rounded-lg border border-white/10 bg-white/[0.03] overflow-hidden"
    >
      <div class="flex items-center gap-2 px-2.5 py-1.5 bg-white/[0.03]">
        <span class="text-[11px] text-cyan-200 shrink-0">{{ seg.id }}</span>
        <input
          :value="seg.title"
          class="flex-1 min-w-0 bg-transparent text-[12px] text-white/85 outline-none border-b border-transparent focus:border-cyan-400/40"
          @input="update(i, { title: ($event.target as HTMLInputElement).value })"
        />
        <input
          :value="seg.duration_s"
          type="number"
          class="w-12 bg-white/5 border border-white/10 rounded px-1 py-0.5 text-[11px] text-white/70 text-center outline-none"
          @input="update(i, { duration_s: Number(($event.target as HTMLInputElement).value) || 0 })"
        />
        <span class="text-[10px] text-white/35 shrink-0">秒</span>
        <span class="text-[10px] px-1.5 py-0.5 rounded bg-white/10 text-white/50 shrink-0">{{ formLabel(seg.form) }}</span>
      </div>

      <div class="px-2.5 py-1 text-[10px] text-white/35">
        覆盖 EV：{{ seg.coverage.EV.join('、') || '—' }} ｜ DLG：{{ seg.coverage.DLG.join('、') || '—' }}
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-[10px] border-collapse">
          <thead>
            <tr class="text-white/40">
              <th class="text-left font-normal px-2 py-1 border-b border-white/10 whitespace-nowrap">镜号</th>
              <th class="text-left font-normal px-2 py-1 border-b border-white/10 whitespace-nowrap">时间</th>
              <th class="text-left font-normal px-2 py-1 border-b border-white/10">画面动作概述</th>
              <th class="text-left font-normal px-2 py-1 border-b border-white/10">机位</th>
              <th class="text-left font-normal px-2 py-1 border-b border-white/10">对白</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="shot in seg.shots" :key="shot.idx" class="align-top">
              <td class="px-2 py-1 border-b border-white/5 text-white/50 whitespace-nowrap">{{ shot.idx }}</td>
              <td class="px-2 py-1 border-b border-white/5 text-white/50 whitespace-nowrap">{{ shot.t }}</td>
              <td class="px-2 py-1 border-b border-white/5 text-white/70">{{ shot.action }}</td>
              <td class="px-2 py-1 border-b border-white/5 text-white/60">{{ shot.camera }}</td>
              <td class="px-2 py-1 border-b border-white/5 text-white/70">{{ shot.dialogue || '无' }}</td>
            </tr>
            <tr v-if="seg.shots.length === 0">
              <td colspan="5" class="px-2 py-2 text-center text-white/25">没有镜头</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="ledger.length > 0">
      <div class="text-[11px] text-white/40 mb-1">跨段连续性账本</div>
      <div v-for="(l, i) in ledger" :key="i" class="text-[10px] text-white/50 py-0.5">
        {{ l.from }} → {{ l.to }}：{{ l.inherit.join('、') || '—' }}
      </div>
    </div>
  </div>
</template>
