<script setup lang="ts">
import { onMounted, ref } from 'vue'
const props = defineProps<{canvasId?:string}>()
const emit = defineEmits<{(e:'scope',project:string,mode:string):void}>()
const project=ref('');const mode=ref('all');const message=ref('')
function apply(){emit('scope',project.value,mode.value)}
onMounted(async()=>{if(!props.canvasId)return;try{const r=await fetch(`/api/canvases/${props.canvasId}/project`);if(!r.ok)throw new Error('读取失败');project.value=(await r.json()).project;mode.value=project.value?'project':'all';apply()}catch{message.value='项目归属读取失败'}})
async function save(){
 if(!props.canvasId)return
 const r=await fetch(`/api/canvases/${props.canvasId}/project`,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({project:project.value.trim()})})
 message.value=r.ok?'画布项目已保存；新生成结果入库会自动归入此项目':'保存失败'
 if(r.ok){project.value=project.value.trim();apply()}
}
</script>
<template>
 <div class="px-6 py-2 text-xs text-white/70 space-y-2">
  <div class="flex gap-2">
   <input v-model="project" aria-label="当前画布项目" placeholder="画布项目，例如：打铁花" class="bg-neutral-800 p-1 flex-1" />
   <button @click="save">保存项目</button>
   <select v-model="mode" aria-label="资产范围" class="bg-neutral-800" @change="apply">
    <option value="project">当前项目＋公共</option><option value="public">公共资产</option><option value="all">全部资产</option>
   </select>
  </div>
  <p>{{message}}</p>
 </div>
</template>
