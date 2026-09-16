<script setup lang="ts">
// 分镜台每个 Gate 的通用外壳：序号、标题、状态色、右侧操作区。
defineProps<{
  index: number
  title: string
  description?: string
  status: 'pending' | 'ready' | 'done'
  disabled?: boolean
}>()
</script>

<template>
  <div
    class="rounded-xl border px-3 py-2.5 transition-colors"
    :class="[
      status === 'done' ? 'border-emerald-400/30 bg-emerald-400/[0.06]' : '',
      status === 'ready' ? 'border-cyan-400/40 bg-cyan-400/[0.06]' : '',
      status === 'pending' ? 'border-white/10 bg-white/[0.03]' : '',
      disabled ? 'opacity-50 pointer-events-none' : '',
    ]"
  >
    <div class="flex items-center gap-2">
      <span
        class="w-5 h-5 rounded-full flex items-center justify-center text-[10px] shrink-0"
        :class="status === 'done' ? 'bg-emerald-400/20 text-emerald-200' : 'bg-white/10 text-white/50'"
      >
        {{ status === 'done' ? '✓' : index }}
      </span>
      <span class="text-[13px] font-medium text-white/85">{{ title }}</span>
      <slot name="badge" />
      <div class="ml-auto flex items-center gap-1.5">
        <slot name="actions" />
      </div>
    </div>
    <p v-if="description" class="mt-1.5 ml-7 text-[11px] text-white/40 leading-relaxed">{{ description }}</p>
    <div class="mt-2 ml-7">
      <slot />
    </div>
  </div>
</template>
