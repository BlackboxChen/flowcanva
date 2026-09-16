<script setup lang="ts">
import { ref, reactive } from 'vue'
import type { NodeRole } from '../types'

const props = defineProps<{ modelValue: NodeRole[] }>()
const emit = defineEmits<{
  (e: 'update:modelValue', value: NodeRole[]): void
  /** 用户确实改动了角色列表时触发，供父组件决定何时落库。 */
  (e: 'change'): void
}>()

const showAddRole = ref(false)
const editingIndex = ref<number | null>(null)
const newRole = reactive<NodeRole>({ key: '', label: '', prompt: '' })
const editRole = reactive<NodeRole>({ key: '', label: '', prompt: '' })

function commit(next: NodeRole[]) {
  emit('update:modelValue', next)
  emit('change')
}

function addRole() {
  if (!newRole.key.trim() || !newRole.label.trim()) return
  commit([...props.modelValue, { ...newRole }])
  newRole.key = ''
  newRole.label = ''
  newRole.prompt = ''
  showAddRole.value = false
}

function startEditRole(i: number) {
  editingIndex.value = i
  Object.assign(editRole, props.modelValue[i])
}

function saveEditRole() {
  if (editingIndex.value === null) return
  const index = editingIndex.value
  editingIndex.value = null
  commit(props.modelValue.map((r, i) => (i === index ? { ...editRole } : r)))
}

function cancelEditRole() {
  editingIndex.value = null
}

function removeRole(i: number) {
  if (editingIndex.value === i) editingIndex.value = null
  commit(props.modelValue.filter((_, idx) => idx !== i))
}
</script>

<template>
  <div class="card bg-base-100 border border-base-200/80 shadow-sm rounded-2xl overflow-hidden">
    <div class="border-b border-base-200/80 bg-base-100 px-6 py-4 flex items-center justify-between">
      <div class="space-y-0.5">
        <h3 class="text-sm font-bold text-base-content/80">预设角色模板库</h3>
        <p class="text-[10px] text-base-content/40 font-medium">定义专属角色（Persona），节点使用时可在画布面板中切选</p>
      </div>
      <button v-if="!showAddRole" class="btn btn-ghost hover:bg-primary/10 text-primary btn-xs font-bold gap-1 rounded-lg" @click="showAddRole = true">
        <svg xmlns="http://www.w3.org/2000/svg" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4"/></svg>
        添加角色
      </button>
    </div>

    <div class="p-6">
      <div v-if="modelValue.length > 0" class="flex flex-col gap-4">
        <div v-for="(r, i) in modelValue" :key="i" class="relative group">
          <!-- 编辑中 -->
          <div v-if="editingIndex === i" class="bg-base-200/10 rounded-2xl p-5 border border-base-200/80 space-y-4 shadow-inner">
            <div class="flex items-center justify-between border-b border-base-200/60 pb-2 mb-2">
              <h4 class="text-xs font-extrabold text-base-content/80">编辑角色配置</h4>
              <button class="text-base-content/40 hover:text-base-content text-[11px] font-bold" @click="cancelEditRole">✕</button>
            </div>
            <div class="grid grid-cols-2 gap-3">
              <label class="form-control w-full">
                <div class="label py-0.5"><span class="label-text text-[10px] font-bold text-base-content/50">角色标识</span></div>
                <input v-model="editRole.key" type="text" class="input input-bordered input-sm w-full focus:ring-1 focus:ring-primary/10 text-xs font-mono" />
              </label>
              <label class="form-control w-full">
                <div class="label py-0.5"><span class="label-text text-[10px] font-bold text-base-content/50">角色展示名称</span></div>
                <input v-model="editRole.label" type="text" class="input input-bordered input-sm w-full focus:ring-1 focus:ring-primary/10 text-xs" />
              </label>
            </div>
            <label class="form-control w-full">
              <div class="label py-0.5"><span class="label-text text-[10px] font-bold text-base-content/50">系统 System Prompt</span></div>
              <textarea v-model="editRole.prompt" class="textarea textarea-bordered w-full h-24 focus:ring-1 focus:ring-primary/10 text-xs leading-relaxed" />
            </label>
            <div class="flex gap-2 justify-end">
              <button class="btn btn-primary btn-xs px-3 rounded-lg font-bold" @click="saveEditRole">确认修改</button>
              <button class="btn btn-ghost btn-xs px-3 rounded-lg font-bold" @click="cancelEditRole">取消</button>
            </div>
          </div>

          <!-- 展示卡片 -->
          <div v-else class="bg-base-100 rounded-xl p-4 border border-base-200/85 hover:border-base-300 transition-colors shadow-sm">
            <div class="flex items-center justify-between mb-2">
              <div class="flex items-center gap-2 min-w-0">
                <span class="text-xs font-extrabold text-base-content/85">{{ r.label }}</span>
                <code class="text-[9px] bg-base-200 text-base-content/50 font-bold font-mono px-2 py-0.5 rounded-md">{{ r.key }}</code>
              </div>
              <div class="flex gap-1 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
                <button class="btn btn-ghost hover:bg-primary/10 text-primary btn-xs rounded-lg px-2 text-[10px] font-bold" @click="startEditRole(i)">编辑</button>
                <button class="btn btn-ghost hover:bg-error/10 hover:text-error btn-xs rounded-lg px-2 text-[10px] font-bold" @click="removeRole(i)">删除</button>
              </div>
            </div>
            <p class="text-xs text-base-content/40 leading-relaxed font-medium line-clamp-3 whitespace-pre-wrap">{{ r.prompt || '（未配置 System Prompt）' }}</p>
          </div>
        </div>
      </div>
      <div v-else-if="!showAddRole" class="text-center py-6">
        <p class="text-xs text-base-content/30 font-medium">暂无任何预设角色配置模板</p>
      </div>

      <!-- 新建角色 -->
      <div v-if="showAddRole" class="bg-base-200/10 rounded-2xl p-5 border border-base-200/80 space-y-4 shadow-inner">
        <div class="flex items-center justify-between border-b border-base-200/60 pb-2 mb-2">
          <h4 class="text-xs font-extrabold text-base-content/80">新增系统角色预设</h4>
          <button class="text-base-content/40 hover:text-base-content text-[11px] font-bold" @click="showAddRole = false">✕</button>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <label class="form-control w-full">
            <div class="label py-0.5"><span class="label-text text-[10px] font-bold text-base-content/50">唯一英文标识 (key)</span></div>
            <input v-model="newRole.key" type="text" placeholder="realistic" class="input input-bordered input-sm w-full focus:ring-1 focus:ring-primary/10 text-xs font-mono" />
          </label>
          <label class="form-control w-full">
            <div class="label py-0.5"><span class="label-text text-[10px] font-bold text-base-content/50">角色展示名称</span></div>
            <input v-model="newRole.label" type="text" placeholder="写实电影" class="input input-bordered input-sm w-full focus:ring-1 focus:ring-primary/10 text-xs" />
          </label>
        </div>
        <label class="form-control w-full">
          <div class="label py-0.5"><span class="label-text text-[10px] font-bold text-base-content/50">角色提示词 (System Prompt)</span></div>
          <textarea v-model="newRole.prompt" class="textarea textarea-bordered w-full h-24 focus:ring-1 focus:ring-primary/10 text-xs leading-relaxed" placeholder="整体风格走低饱和写实电影质感，表演克制，摄影运动贴近人物心理。" />
        </label>
        <div class="flex gap-2 pt-2 justify-end">
          <button class="btn btn-primary btn-xs px-3 rounded-lg font-bold" @click="addRole">同步并添加</button>
          <button class="btn btn-ghost btn-xs px-3 rounded-lg font-bold" @click="showAddRole = false">取消</button>
        </div>
      </div>
    </div>
  </div>
</template>
