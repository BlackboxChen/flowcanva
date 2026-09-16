<script setup lang="ts">
// EV / DLG / LOCK 三份清单。台词逐字保留是本技能的第一原则，所以允许直接修正。
import type { ShotlistEV, ShotlistDLG, ShotlistLOCK } from '../../types/shotlist'

const props = defineProps<{
  ev: ShotlistEV[]
  dlg: ShotlistDLG[]
  lock: ShotlistLOCK[]
}>()

const emit = defineEmits<{
  (e: 'update', patch: { EV?: ShotlistEV[]; DLG?: ShotlistDLG[]; LOCK?: ShotlistLOCK[] }): void
}>()

function updateEV(i: number, text: string) {
  emit('update', { EV: props.ev.map((x, idx) => (idx === i ? { ...x, text } : x)) })
}

function updateDLG(i: number, patch: Partial<ShotlistDLG>) {
  emit('update', { DLG: props.dlg.map((x, idx) => (idx === i ? { ...x, ...patch } : x)) })
}

function updateLOCK(i: number, fact: string) {
  emit('update', { LOCK: props.lock.map((x, idx) => (idx === i ? { ...x, fact } : x)) })
}
</script>

<template>
  <div class="space-y-3">
    <div>
      <div class="text-[11px] text-white/40 mb-1">事件 EV（{{ ev.length }}）</div>
      <div v-if="ev.length === 0" class="text-[11px] text-white/25">暂无</div>
      <div v-for="(item, i) in ev" :key="item.id" class="flex items-center gap-2 py-0.5">
        <span class="text-[10px] text-white/35 w-14 shrink-0">{{ item.id }}</span>
        <input
          :value="item.text"
          class="flex-1 min-w-0 bg-white/5 border border-white/10 rounded px-1.5 py-0.5 text-[11px] text-white/80 outline-none focus:border-cyan-400/40"
          @input="updateEV(i, ($event.target as HTMLInputElement).value)"
        />
      </div>
    </div>

    <div>
      <div class="text-[11px] text-white/40 mb-1">台词 DLG（{{ dlg.length }}）· 逐字保留，不得改写</div>
      <div v-if="dlg.length === 0" class="text-[11px] text-white/25">无台词</div>
      <div v-for="(item, i) in dlg" :key="item.id" class="flex items-center gap-2 py-0.5">
        <span class="text-[10px] text-white/35 w-14 shrink-0">{{ item.id }}</span>
        <input
          :value="item.speaker"
          class="w-20 shrink-0 bg-white/5 border border-white/10 rounded px-1.5 py-0.5 text-[11px] text-white/80 outline-none focus:border-cyan-400/40"
          @input="updateDLG(i, { speaker: ($event.target as HTMLInputElement).value })"
        />
        <input
          :value="item.text"
          class="flex-1 min-w-0 bg-white/5 border border-white/10 rounded px-1.5 py-0.5 text-[11px] text-white/90 outline-none focus:border-cyan-400/40"
          @input="updateDLG(i, { text: ($event.target as HTMLInputElement).value })"
        />
      </div>
    </div>

    <div>
      <div class="text-[11px] text-white/40 mb-1">锁定事实 LOCK（{{ lock.length }}）</div>
      <div v-if="lock.length === 0" class="text-[11px] text-white/25">暂无</div>
      <div v-for="(item, i) in lock" :key="item.id" class="flex items-center gap-2 py-0.5">
        <span class="text-[10px] text-white/35 w-14 shrink-0">{{ item.id }}</span>
        <input
          :value="item.fact"
          class="flex-1 min-w-0 bg-white/5 border border-white/10 rounded px-1.5 py-0.5 text-[11px] text-white/80 outline-none focus:border-cyan-400/40"
          @input="updateLOCK(i, ($event.target as HTMLInputElement).value)"
        />
      </div>
    </div>
  </div>
</template>
