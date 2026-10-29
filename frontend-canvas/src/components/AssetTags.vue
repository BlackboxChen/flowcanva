<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Asset } from '../types'
import { useAssets } from '../composables/useAssets'
const props = defineProps<{ asset: Asset }>()
const { setTags } = useAssets()
const value = ref('')
const status = ref('')
watch(() => props.asset.tags, raw => {
  try { value.value = JSON.parse(raw || '[]').join(', ') } catch { value.value = raw || '' }
}, { immediate: true })
async function save() {
  try {
    await setTags(props.asset.id, value.value.split(/[,，\n]/))
    status.value = '已保存'
  } catch { status.value = '保存失败，请重试' }
}
</script>
<template>
  <div class="text-xs text-white/70" @click.stop>
    <input v-model="value" aria-label="编辑资产标签" placeholder="标签，逗号分隔" class="w-full bg-neutral-700 rounded p-1" />
    <button @click="save">保存标签</button> <span role="status">{{ status }}</span>
  </div>
</template>
