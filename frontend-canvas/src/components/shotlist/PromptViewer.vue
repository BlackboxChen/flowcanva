<script setup lang="ts">
// 视频提示词查看与校验结果。提示词允许直接编辑，改完可再次校验。
import { ref } from 'vue'
import type { ShotlistSegment, ShotlistValidation } from '../../types/shotlist'

const props = defineProps<{
  segments: ShotlistSegment[]
  validation: ShotlistValidation
}>()

const emit = defineEmits<{
  (e: 'update', segments: ShotlistSegment[]): void
}>()

const copiedId = ref('')
const expanded = ref<Record<string, boolean>>({})

function updatePrompt(i: number, prompt: string) {
  emit('update', props.segments.map((s, idx) => (idx === i ? { ...s, prompt } : s)))
}

async function copy(seg: ShotlistSegment) {
  try {
    await navigator.clipboard.writeText(seg.prompt)
    copiedId.value = seg.id
    setTimeout(() => { if (copiedId.value === seg.id) copiedId.value = '' }, 1500)
  } catch {
    // 剪贴板不可用时用户仍可手动全选复制。
  }
}

function issuesFor(segId: string) {
  return {
    errors: props.validation.errors.filter(e => e.where === segId),
    warnings: props.validation.warnings.filter(w => w.where === segId),
  }
}
</script>

<template>
  <div class="space-y-2">
    <div v-if="segments.length === 0" class="text-[11px] text-white/30">还没有工作段，请先完成规划。</div>

    <div
      v-for="(seg, i) in segments"
      :key="seg.id"
      class="rounded-lg border border-white/10 bg-white/[0.03] overflow-hidden"
    >
      <div class="flex items-center gap-2 px-2.5 py-1.5">
        <span class="text-[11px] text-cyan-200 shrink-0">{{ seg.id }}</span>
        <span class="text-[11px] text-white/60 truncate flex-1">{{ seg.title }}</span>
        <span
          class="text-[10px] px-1.5 py-0.5 rounded shrink-0"
          :class="seg.prompt ? 'bg-emerald-400/15 text-emerald-200' : 'bg-white/10 text-white/40'"
        >
          {{ seg.prompt ? `${seg.prompt.length} 字` : '未生成' }}
        </span>
        <button
          v-if="seg.prompt"
          class="text-[10px] px-1.5 py-0.5 rounded bg-white/10 hover:bg-white/20 text-white/70 shrink-0"
          @click.stop="copy(seg)"
        >
          {{ copiedId === seg.id ? '已复制' : '复制' }}
        </button>
        <button
          v-if="seg.prompt"
          class="text-[10px] px-1.5 py-0.5 rounded bg-white/10 hover:bg-white/20 text-white/70 shrink-0"
          @click.stop="expanded[seg.id] = !expanded[seg.id]"
        >
          {{ expanded[seg.id] ? '收起' : '展开' }}
        </button>
      </div>

      <div v-for="issue in issuesFor(seg.id).errors" :key="issue.code + issue.message"
        class="mx-2.5 mb-1 rounded border border-red-400/30 bg-red-950/40 px-2 py-1 text-[10px] text-red-200">
        {{ issue.code }}｜{{ issue.message }}
      </div>
      <div v-for="issue in issuesFor(seg.id).warnings" :key="issue.code + issue.message"
        class="mx-2.5 mb-1 rounded border border-amber-400/30 bg-amber-950/30 px-2 py-1 text-[10px] text-amber-200">
        {{ issue.code }}｜{{ issue.message }}
      </div>

      <textarea
        v-if="expanded[seg.id]"
        :value="seg.prompt"
        rows="10"
        class="w-full bg-black/30 px-2.5 py-2 text-[11px] text-white/75 outline-none resize-y font-mono leading-relaxed"
        @input="updatePrompt(i, ($event.target as HTMLTextAreaElement).value)"
        @wheel.stop
      />
    </div>

    <div v-if="validation.errors.some(e => e.where === '全局')" class="space-y-1">
      <div
        v-for="issue in validation.errors.filter(e => e.where === '全局')"
        :key="issue.code + issue.message"
        class="rounded border border-red-400/30 bg-red-950/40 px-2 py-1 text-[10px] text-red-200"
      >
        全局｜{{ issue.code }}｜{{ issue.message }}
      </div>
    </div>
  </div>
</template>
