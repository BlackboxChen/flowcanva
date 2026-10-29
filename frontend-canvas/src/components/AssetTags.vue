<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Asset } from '../types'
import { updateAsset } from '../api'
import { useAssets } from '../composables/useAssets'
const props = defineProps<{ asset: Asset }>()
const { setTags } = useAssets()
const value = ref('')
const status = ref('')
const project = ref(props.asset.project || '')
watch(() => props.asset.tags, raw => {
  try { value.value = JSON.parse(raw || '[]').join(', ') } catch { value.value = raw || '' }
}, { immediate: true })
async function save() {
  try {
    await setTags(props.asset.id, value.value.split(/[,，\n]/))
    await updateAsset(props.asset.id, {project: project.value.trim()})
    props.asset.project = project.value.trim()
    status.value = '已保存'
  } catch { status.value = '保存失败，请重试' }
}
</script>
<template>
  <div class="text-xs text-white/70" @click.stop>
    <input v-model="value" aria-label="编辑资产标签" placeholder="标签，逗号分隔" class="w-full bg-neutral-700 rounded p-1" />
    <input v-model="project" aria-label="资产所属项目" placeholder="所属项目（空为公共）" class="w-full bg-neutral-700 rounded p-1 mt-1" />
    <button @click="save">保存标签/项目</button> <span role="status">{{ status }}</span>
  </div>
</template>
