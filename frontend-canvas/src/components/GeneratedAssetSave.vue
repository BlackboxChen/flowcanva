<script setup lang="ts">
import { computed, ref } from 'vue'
import type { WorkflowOutput } from '../types/workflow'
import { uploadAsset, updateAsset, deleteAsset } from '../api'
import { useAssets, ASSET_CATEGORIES } from '../composables/useAssets'

const props = defineProps<{ outputs: WorkflowOutput[]; canvasId: string; nodeId: string; name: string }>()
const { loadAssets } = useAssets()
const media = computed(() => props.outputs.filter(o => o.type !== 'text' && o.value))
const selected = ref(0)
const tags = ref('')
const category = ref('物品')
const busy = ref(false)
const message = ref('')
async function saveAsset() {
  const output = media.value[selected.value]
  if (!output) return
  busy.value = true
  message.value = ''
  try {
    const url = new URL(output.value, location.origin)
    if (url.origin !== location.origin || !url.pathname.startsWith('/uploads/')) {
      throw new Error('结果尚未持久化到本地，请先查询任务结果。')
    }
    const response = await fetch(url)
    if (!response.ok) throw new Error('读取生成结果失败')
    const blob = await response.blob()
    const ext = url.pathname.split('.').pop() || 'png'
    const asset = await uploadAsset(new File([blob], `${props.name || '生成资产'}-${output.name}.${ext}`, { type: blob.type }))
    const labels = [...new Set([
      ...tags.value.split(/[,，\n]/).map(t => t.trim()).filter(Boolean),
      `画布:${props.canvasId}`, `节点:${props.nodeId}`,
    ])]
    try {
      await updateAsset(asset.id, { category: category.value, tags: JSON.stringify(labels) })
    } catch (error) {
      await deleteAsset(asset.id)
      throw error
    }
    await loadAssets(true)
    message.value = '已保存到资产库（独立副本，可跨画布复用）'
  } catch (error) {
    message.value = error instanceof Error ? error.message : '保存失败，请重试'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <details v-if="media.length" class="border border-white/20 rounded p-2 text-xs text-white/80" @pointerdown.stop @wheel.stop>
    <summary>生成结果 → 按标签保存到资产库</summary>
    <div class="flex flex-col gap-2 mt-2">
      <select v-model="selected" aria-label="保存哪个生成结果" class="bg-neutral-800">
        <option v-for="(output, i) in media" :key="i" :value="i">{{ output.name }}（{{ output.type }}）</option>
      </select>
      <select v-model="category" aria-label="资产分类" class="bg-neutral-800">
        <option v-for="c in ASSET_CATEGORIES.filter(c => c !== '全部')" :key="c">{{ c }}</option>
      </select>
      <input v-model="tags" aria-label="资产标签" placeholder="标签，逗号分隔：项目:打铁花,章节:第二章,角色:周铁山" class="bg-neutral-800 p-1" />
      <span>自动附加来源画布与节点标签；标签用于筛选，不是访问权限。</span>
      <button :disabled="busy" class="bg-cyan-900 rounded p-1" @click.stop="saveAsset">{{ busy ? '保存中…' : '保存选中结果到资产库' }}</button>
      <span role="status">{{ message }}</span>
    </div>
  </details>
</template>
