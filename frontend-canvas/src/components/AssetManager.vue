<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { useAssets, ASSET_CATEGORIES } from '../composables/useAssets'
import type { Asset } from '../types'
import AssetTags from './AssetTags.vue'
import AssetScope from './AssetScope.vue'
import { fetchAssets } from '../api'
const props = defineProps<{canvasId?:string}>()
const scopeProject = ref('')
const scopeMode = ref('all')
const trash = ref(false)
const trashedAssets = ref<Asset[]>([])
const libraryMessage = ref('')
async function toggleTrash() {
  try { trash.value=!trash.value; if(trash.value) trashedAssets.value=await fetchAssets(true); else await loadAssets(true) } catch { libraryMessage.value='读取回收站失败' }
}
async function restoreAsset(id:string) {
  try { const r=await fetch(`/api/admin/assets/${id}/restore`,{method:'POST'});if(!r.ok)throw new Error();trashedAssets.value=trashedAssets.value.filter(a=>a.id!==id);await loadAssets(true) } catch {libraryMessage.value='恢复失败'}
}
async function trashAsset(id:string) {
  try { await removeAsset(id); libraryMessage.value='已移入回收站，可恢复；画布中已引用的文件保留。' }
  catch { libraryMessage.value='删除失败，请重试' }
}
async function showReferences(id:string) {
  try { const r=await fetch(`/api/admin/assets/${id}/references`);if(!r.ok)throw new Error();const refs=await r.json();libraryMessage.value=refs.length ? refs.map((r:any)=>`${r.canvas_name} / ${r.node_id}`).join('；') : '暂无画布引用' } catch {libraryMessage.value='引用检查失败'}
}

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { assets, addAsset, setCategory, removeAsset, loadAssets } = useAssets()
const activeCategory = ref<string>('全部')
const tagQuery = ref('')
const isDragOver = ref(false)
const uploading = ref(false)
const PAGE_SIZE = 24
const page = ref(1)
const previewAsset = ref<Asset | null>(null)

loadAssets(true)

watch([activeCategory, tagQuery], () => { page.value = 1 })

const filtered = computed(() => {
  let list = trash.value ? trashedAssets.value : assets.value
  if(scopeMode.value==='project' && scopeProject.value) list=list.filter(a=>!a.project || a.project===scopeProject.value)
  if(scopeMode.value==='public') list=list.filter(a=>!a.project)
  if (activeCategory.value !== '全部') {
    list = list.filter(a => a.category === activeCategory.value)
  }
  const q = tagQuery.value.trim().toLowerCase()
  if (q) list = list.filter(a => (a.tags + ' ' + a.filename).toLowerCase().includes(q))
  return list
})

const categoryCounts = computed(() => {
  const map: Record<string, number> = {}
  for (const a of assets.value) {
    map[a.category] = (map[a.category] || 0) + 1
  }
  return map
})

const totalPages = computed(() => Math.max(1, Math.ceil(filtered.value.length / PAGE_SIZE)))
const pagedAssets = computed(() => {
  const start = (page.value - 1) * PAGE_SIZE
  return filtered.value.slice(start, start + PAGE_SIZE)
})

function triggerUpload() {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*,video/*,audio/*'
  input.multiple = true
  input.onchange = async () => {
    const files = input.files
    if (!files) return
    uploading.value = true
    for (let i = 0; i < files.length; i++) {
      await addAsset(files[i])
    }
    uploading.value = false
  }
  input.click()
}

async function onDrop(e: DragEvent) {
  isDragOver.value = false
  const files = e.dataTransfer?.files
  if (!files) return
  uploading.value = true
  for (let i = 0; i < files.length; i++) {
    await addAsset(files[i])
  }
  uploading.value = false
}

function onDragOver(e: DragEvent) {
  e.preventDefault()
  isDragOver.value = true
}

function onDragLeave() {
  isDragOver.value = false
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + 'B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + 'KB'
  return (bytes / (1024 * 1024)).toFixed(1) + 'MB'
}

const previewIndex = computed(() => {
  if (!previewAsset.value) return -1
  return filtered.value.findIndex(a => a.id === previewAsset.value!.id)
})

function openPreview(asset: Asset) {
  previewAsset.value = asset
}

function closePreview() {
  previewAsset.value = null
}

function nextPreview() {
  const idx = previewIndex.value
  if (idx < filtered.value.length - 1) {
    previewAsset.value = filtered.value[idx + 1]
  }
}

function prevPreview() {
  const idx = previewIndex.value
  if (idx > 0) {
    previewAsset.value = filtered.value[idx - 1]
  }
}

function onPreviewKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') closePreview()
  if (e.key === 'ArrowRight') nextPreview()
  if (e.key === 'ArrowLeft') prevPreview()
}

watch(previewAsset, (val) => {
  if (val) {
    document.addEventListener('keydown', onPreviewKeydown)
  } else {
    document.removeEventListener('keydown', onPreviewKeydown)
  }
})

onUnmounted(() => {
  document.removeEventListener('keydown', onPreviewKeydown)
})
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-[9990] flex items-center justify-center" @click.self="emit('close')">
      <div class="absolute inset-0 bg-black/70 backdrop-blur-sm" />
      <div class="relative w-[960px] max-w-[95vw] max-h-[90vh] bg-neutral-900/95 backdrop-blur rounded-2xl border border-white/20 shadow-sm flex flex-col">
        <!-- 头部 -->
        <div class="flex items-center justify-between px-6 py-4 border-b border-white/10 shrink-0">
          <h2 class="text-white text-lg font-semibold">资产管理</h2>
          <button class="btn btn-sm btn-square bg-white/10 border border-white/30 text-white hover:bg-white/20" @click="emit('close')">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>

        <AssetScope :canvas-id="props.canvasId" @scope="(project,mode)=>{scopeProject=project;scopeMode=mode;page=1}" />
        <div class="px-6 text-xs text-white/70"><button @click="toggleTrash">{{trash?'返回资产库':'打开回收站'}}</button><p>{{libraryMessage}}</p></div>
        <!-- 上传区 -->
        <div
          class="mx-6 mt-4 border-2 border-dashed rounded-xl p-6 text-center transition-colors cursor-pointer shrink-0"
          :class="isDragOver ? 'border-blue-400 bg-blue-400/10' : 'border-white/10 hover:border-white/25'"
          @click="triggerUpload"
          @drop.prevent="onDrop"
          @dragover.prevent="onDragOver"
          @dragleave="onDragLeave"
        >
          <template v-if="uploading">
            <div class="flex items-center justify-center gap-2 text-white/50">
              <span class="loading loading-spinner loading-sm" />
              <span class="text-sm">上传中...</span>
            </div>
          </template>
          <template v-else>
            <svg xmlns="http://www.w3.org/2000/svg" width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="mx-auto mb-2 text-white/25">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
              <polyline points="17 8 12 3 7 8"/>
              <line x1="12" y1="3" x2="12" y2="15"/>
            </svg>
            <p class="text-white/40 text-sm">拖放图片/视频/音频到此处上传，或点击选择文件</p>
            <p class="text-white/20 text-xs mt-1">支持 PNG/JPG/WebP/GIF/MP4/MP3/WAV 等格式</p>
          </template>
        </div>

        <!-- 分类 tab -->
        <div class="flex flex-wrap gap-1 px-6 py-3 shrink-0">
          <button
            v-for="cat in ASSET_CATEGORIES" :key="cat"
            class="px-2.5 py-1 text-xs rounded-md border transition-colors"
            :class="activeCategory === cat
              ? 'bg-white/20 border-white/40 text-white'
              : 'bg-transparent border-white/10 text-white/50 hover:text-white/80'"
            @click="activeCategory = cat"
          >{{ cat }} {{ cat !== '全部' ? `(${categoryCounts[cat] || 0})` : '' }}</button>
          <span v-if="activeCategory === '全部'" class="ml-auto text-xs text-white/30 self-center">{{ filtered.length }} 个资产</span>
        </div>

        <input v-model="tagQuery" aria-label="按标签或名称筛选资产" placeholder="筛选标签或名称，例如 项目:打铁花 / 章节:第二章" class="mx-6 mb-3 p-2 bg-neutral-800 text-white text-sm rounded" />

        <!-- 资产网格 -->
        <div class="flex-1 overflow-y-auto px-6 pb-4 min-h-0">
          <div v-if="pagedAssets.length === 0" class="flex flex-col items-center justify-center h-full text-white/20 py-16">
            <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round" stroke-linejoin="round" class="mb-3">
              <rect x="3" y="3" width="18" height="18" rx="2" ry="2"/>
              <circle cx="8.5" cy="8.5" r="1.5"/>
              <polyline points="21 15 16 10 5 21"/>
            </svg>
            <p class="text-sm">暂无资产，拖放图片上传</p>
          </div>
          <div v-else class="grid grid-cols-4 gap-3">
            <div
              v-for="a in pagedAssets" :key="a.id"
              class="group bg-neutral-800/60 rounded-xl border border-white/10 overflow-hidden hover:border-white/25 transition-all"
            >
              <!-- 缩略图 -->
              <div class="aspect-square bg-neutral-900 flex items-center justify-center overflow-hidden cursor-pointer" @click="openPreview(a)">
                <img :src="a.url" class="w-full h-full object-contain" :alt="a.filename" loading="lazy" />
              </div>
              <!-- 信息栏 -->
              <div class="p-2 space-y-1">
                <p class="text-xs text-white/80 truncate" :title="a.filename">{{ a.filename }}</p>
                <AssetTags :asset="a" />
                <button class="text-xs text-white/60" @click="showReferences(a.id)">查看引用</button>
                <button v-if="trash" class="text-xs text-cyan-300 ml-2" @click="restoreAsset(a.id)">恢复</button>
                <p class="text-[10px] text-white/30">{{ formatSize(a.size) }}</p>
                <div class="flex items-center gap-1.5">
                  <select
                    class="flex-1 bg-neutral-700 border border-white/10 rounded text-[10px] text-white/70 px-1.5 py-0.5 outline-none [color-scheme:dark]"
                    :value="a.category"
                    @click.stop
                    @change="setCategory(a.id, ($event.target as HTMLSelectElement).value)"
                  >
                    <option v-for="cat in ASSET_CATEGORIES.filter(c => c !== '全部')" :key="cat" :value="cat">{{ cat }}</option>
                  </select>
                  <button
                    class="shrink-0 rounded bg-neutral-700 border border-white/10 text-white/70 hover:text-red-400 hover:border-red-500/30 px-2 py-1 text-xs"
                    v-if="!trash" title="删除资产并移入回收站，已引用的文件保留"
                    @click="trashAsset(a.id)"
                  >删除</button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 分页 -->
        <div v-if="totalPages > 1" class="flex items-center justify-center gap-2 py-3 border-t border-white/10 shrink-0">
          <button
            class="btn btn-xs bg-white/10 border border-white/20 text-white/60 hover:text-white hover:bg-white/20"
            :disabled="page <= 1"
            @click="page = Math.max(1, page - 1)"
          >上一页</button>
          <span class="text-xs text-white/40">{{ page }} / {{ totalPages }}</span>
          <button
            class="btn btn-xs bg-white/10 border border-white/20 text-white/60 hover:text-white hover:bg-white/20"
            :disabled="page >= totalPages"
            @click="page = Math.min(totalPages, page + 1)"
          >下一页</button>
        </div>
      </div>
    </div>

    <!-- 图片预览灯箱 -->
    <div
      v-if="previewAsset"
      class="fixed inset-0 z-[9999] flex items-center justify-center bg-black/90 backdrop-blur-sm"
      @click.self="closePreview"
    >
      <!-- 关闭 -->
      <button class="absolute top-4 right-4 btn btn-sm btn-square bg-white/10 border border-white/30 text-white hover:bg-white/20 z-10" @click="closePreview">
        <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
      </button>

      <!-- 上一张 -->
      <button
        v-if="previewIndex > 0"
        class="absolute left-4 top-1/2 -translate-y-1/2 btn btn-sm btn-square bg-white/10 border border-white/30 text-white hover:bg-white/20 z-10"
        @click.stop="prevPreview"
      >
        <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="15 18 9 12 15 6"/></svg>
      </button>

      <!-- 下一张 -->
      <button
        v-if="previewIndex < filtered.length - 1"
        class="absolute right-4 top-1/2 -translate-y-1/2 btn btn-sm btn-square bg-white/10 border border-white/30 text-white hover:bg-white/20 z-10"
        @click.stop="nextPreview"
      >
        <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="9 18 15 12 9 6"/></svg>
      </button>

      <img :src="previewAsset.url" class="max-w-[90vw] max-h-[90vh] object-contain rounded-lg" :alt="previewAsset.filename" />

      <!-- 底部信息 -->
      <div class="absolute bottom-4 left-1/2 -translate-x-1/2 text-white/60 text-xs">
        {{ previewAsset.filename }} &middot; {{ formatSize(previewAsset.size) }} &middot; {{ previewIndex + 1 }} / {{ filtered.length }}
      </div>
    </div>
  </Teleport>
</template>
