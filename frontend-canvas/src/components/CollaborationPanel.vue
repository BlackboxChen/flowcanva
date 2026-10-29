<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { Node } from '../types'
import { waitForNodeSaves } from '../api'
const props = defineProps<{canvasId: string; nodes: Node[]; syncMessage: string; saveProblems: Record<string,string>; selectedId: string | null}>()
defineEmits<{(e:'reload'):void}>()
const open = ref(false)
const chosen = ref<string[]>([])
const tasks = ref<any[]>([])
const message = ref('')
const busy = ref(false)
const limits = ref<Record<string,number>>({})
function content(n: Node) { try { return JSON.parse(n.content || '{}') } catch { return {} } }
const workflows = computed(() => props.nodes.filter(n => n.node_type === 'workflow'))
const servers = computed(() => [...new Set(workflows.value.map(n => content(n).serverUrl).filter(Boolean))] as string[])
const labels: Record<string,string> = {pending:'等待依赖/服务空闲',submitting:'提交中',running:'运行中',review:'等待人工审核',done:'完成',failed:'失败',blocked:'已阻止',cancelled:'已停止调度'}
async function request(url: string, method='GET', body?: any) {
  const response = await fetch(url,{method,headers:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)})
  if (response.status===204) return
  const data = await response.json()
  if (!response.ok) throw new Error(data.error || '请求失败')
  return data
}
async function refresh() {
  if (!open.value) return
  try { tasks.value = await request(`/api/canvases/${props.canvasId}/batch`) } catch(e) { message.value = String(e) }
}
watch(open,async value => {
  if (!value) return
  await refresh()
  try { const settings = await request('/api/collaboration/settings'); limits.value = settings.limits } catch(e) { message.value=String(e) }
})
async function start() {
  busy.value=true
  try {
    if (Object.keys(props.saveProblems).length) throw new Error('请先处理保存冲突再开始批次')
    await Promise.all(chosen.value.map(waitForNodeSaves))
    await request(`/api/canvases/${props.canvasId}/batch`,'POST',{node_ids:chosen.value})
    message.value='已建立持久化批次；关闭页面后后端继续调度。';await refresh()
  } catch(e) { message.value=String(e) } finally { busy.value=false }
}
async function backup() {
  busy.value=true
  try {const result=await request('/api/collaboration/backup','POST');message.value=`备份完成：${result.path}`}catch(e){message.value=String(e)}finally{busy.value=false}
}
async function stop() {
  try { await request(`/api/canvases/${props.canvasId}/batch`,'DELETE');message.value='停止后续节点；已提交任务继续执行';await refresh() } catch(e){message.value=String(e)}
}
async function setLimit(server:string,event:Event) {
  try { const max=Number((event.target as HTMLInputElement).value);await request('/api/collaboration/settings','PUT',{server,max_active:max});limits.value[server]=max } catch(e) {message.value=String(e)}
}
const timer=setInterval(refresh,4000);onUnmounted(()=>clearInterval(timer))
</script>
<template>
  <div class="fixed top-14 right-4 z-[150] bg-neutral-900 text-white border border-white/20 rounded p-2 text-xs max-w-[92vw]" @pointerdown.stop @wheel.stop>
    <button @click="open=!open">协作与并发调度 {{ open ? '收起' : '展开' }}</button>
    <span v-if="Object.keys(saveProblems).length" class="text-red-400 ml-2">有未保存修改</span>
    <div v-if="open" class="w-[480px] max-w-[85vw] max-h-[75vh] overflow-y-auto space-y-3 mt-2">
      <p>{{ syncMessage }}</p>
      <button class="bg-neutral-700 p-1 rounded" :disabled="busy" @click="backup">立即备份画布与素材</button>
      <p v-for="(error,id) in saveProblems" :key="id" class="text-red-300">{{ id }}：{{ error }}</p>
      <button v-if="selectedId" class="bg-neutral-700 p-1 rounded" @click="$emit('reload')">导出当前节点草稿并加载最新版本</button>
      <p>只调度勾选的工作流节点；依赖上游完成后执行。需要关键帧审核的视频暂停等待你在节点面板确认、执行。</p>
      <div v-for="server in servers" :key="server" class="flex gap-2 items-center">
        <span class="truncate flex-1">{{ server }}</span>
        <label>并发上限 <input type="number" min="1" max="8" :value="limits[server] ?? 2" class="w-12 bg-neutral-700" @change="setLimit(server,$event)" /></label>
      </div>
      <p class="text-amber-200">上限按同一服务地址全局共享；GPU 实际并行度由上游决定。建议先用 1–2。</p>
      <div class="max-h-48 overflow-y-auto border border-white/20 p-1">
        <label v-for="node in workflows" :key="node.id" class="block py-1">
          <input v-model="chosen" type="checkbox" :value="node.id" /> {{ content(node).displayName || content(node).name || node.id }}
        </label>
      </div>
      <button class="bg-cyan-900 p-2 rounded" :disabled="busy || !chosen.length" @click="start">执行所选 {{ chosen.length }} 个节点</button>
      <button class="bg-neutral-700 p-2 rounded ml-2" @click="stop">停止后续调度</button>
      <p role="status">{{ message }}</p>
      <div v-for="task in tasks.slice(-100)" :key="task.id" class="border-t border-white/10 py-1">
        {{ content(nodes.find(n=>n.id===task.node_id) || {content:'{}'} as Node).name || task.node_id }} — {{ labels[task.status] || task.status }}
        <p class="text-white/60">{{ task.error }}</p>
      </div>
    </div>
  </div>
</template>
