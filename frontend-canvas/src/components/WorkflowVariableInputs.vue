<script setup lang="ts">
import { computed } from 'vue'
import type { WorkflowVariable } from '../composables/useWorkflowReferences'

const props = defineProps<{
  variables: WorkflowVariable[]
  media: { url: string; label: string }[]
}>()
const emit = defineEmits<{ (e: 'change'): void }>()
const shots = computed(() => props.variables.filter(v => /^shot_\d+$/.test(v.name)))
const otherInputs = computed(() => props.variables.filter(v => !/^shot_\d+$/.test(v.name)))
function shotLabel(name: string) { return `镜头 ${name.slice(5)} 提示词` }
</script>

<template>
  <div class="space-y-2">
    <section v-if="shots.length" aria-label="分镜提示词" class="space-y-2">
      <div class="text-[10px] text-yellow-300/80">分镜提示词 · {{ shots.length }} 镜（可编辑）</div>
      <label v-for="v in shots" :key="v.name" class="block space-y-1">
        <span class="text-[10px] text-yellow-300/70">{{ shotLabel(v.name) }} <span class="text-white/30">{{ v.name }}</span></span>
        <textarea v-model="v.value" :aria-label="shotLabel(v.name)" @input="emit('change')" @wheel.stop rows="4"
          class="w-full min-h-20 text-[11px] leading-4 bg-white/5 border border-white/10 rounded p-2 text-white/80 outline-none resize-y" />
      </label>
    </section>
    <div v-if="shots.length && otherInputs.length" class="text-[10px] text-white/40">参考图与其他输入</div>
    <div v-for="v in otherInputs" :key="v.name" class="flex items-center gap-1">
      <span :title="v.name" class="text-yellow-300/60 text-[9px] w-20 shrink-0 truncate">{{ v.name }}</span>
      <template v-if="v.type === 'image' || v.type === 'video' || v.type === 'audio'">
        <img v-if="v.type === 'image' && v.value" :src="v.value" class="h-6 w-6 rounded object-cover border border-white/10" />
        <select v-model="v.value" :aria-label="v.name" @change="emit('change')" class="flex-1 min-w-0 h-5 text-[8px] bg-white/5 border border-white/10 rounded px-1 text-white/50 outline-none">
          <option value="">(空)</option>
          <option v-for="item in media" :key="item.url" :value="item.url">{{ item.label }}</option>
        </select>
      </template>
      <textarea v-else v-model="v.value" :aria-label="v.name" @input="emit('change')" @wheel.stop rows="3"
        class="flex-1 min-w-0 text-[10px] bg-white/5 border border-white/10 rounded p-1 text-white/70 outline-none resize-y" />
    </div>
  </div>
</template>
