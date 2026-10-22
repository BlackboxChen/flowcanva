import { computed, watch, type Ref } from 'vue'

export interface WorkflowVariable {
  name: string
  value: string
  type: 'image' | 'video' | 'audio' | 'text'
  sourceNodeId?: string
}

export function useWorkflowReferences(inputs: Ref<any[]>, variables: Ref<WorkflowVariable[]>, persist: () => void) {
  const connectedMedia = computed(() => inputs.value.flatMap(input => {
    let data = input.data
    if (typeof data === 'string') { try { data = JSON.parse(data) } catch { return [] } }
    const media = data?.outputs
      ? data.outputs.filter((o: any) => o.type === 'image').map((o: any) => ({ url: o.value, label: data.name || '上游关键帧' }))
      : [{ url: data?.url || data?.dataUrl, label: data?.name || '上游参考图' }]
    return media.filter((o: any) => o.url).map((o: any) => ({ ...o, sourceNodeId: input.sourceNodeId }))
  }))
  watch([connectedMedia, variables], () => {
    let changed = false
    for (const variable of variables.value) {
      if (!variable.sourceNodeId) continue
      const source = connectedMedia.value.find(media => media.sourceNodeId === variable.sourceNodeId)
      if (source && variable.value !== source.url) { variable.value = source.url; changed = true }
    }
    if (changed) persist()
  }, { deep: true })
  function onVarChange() {
    for (const variable of variables.value) {
      const source = connectedMedia.value.find(media => media.sourceNodeId === variable.sourceNodeId)
      if (source && variable.value !== source.url) variable.sourceNodeId = undefined
    }
    persist()
  }
  return { connectedMedia, onVarChange }
}
