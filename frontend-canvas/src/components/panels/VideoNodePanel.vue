<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import type { Node } from '../../types'
import MentionDropdown from '../MentionDropdown.vue'
import { useNodeConfigs } from '../../composables/useNodeConfigs'

const props = defineProps<{
  node: Node | null
  panelStyle: Record<string, string>
  nodeInputs?: { edgeId: string; sourceNodeId: string; sourceNodeType: string; data: any }[]
}>()

const emit = defineEmits<{
  (e: 'save', content: string): void
  (e: 'remove-connected-edge', edgeId: string): void
}>()

const { modelInfoMap, getModels, getModelConfig } = useNodeConfigs()

function modelLabel(model: string) {
  const info = modelInfoMap.value.video?.find(item => item.name === model)
  const note = info?.parameters?.note
  return note ? `${model} (${note})` : model
}

const prompt = ref('')
const promptHtml = ref('')
const loading = ref(false)
const videos = ref<{ id: number; url: string }[]>([])
const selectedModel = ref('sd-2.0-15s-16x9-480p')
const models = ref<string[]>(['sd-2.0-15s-16x9-480p'])
const selectedRatio = ref('16:9')
const selectedResolution = ref('480p')
const selectedDuration = ref('15')
const selectedCount = ref(1)
const selectedMode = ref('文生视频')

const allModes = ['全能参考', '文生视频', '首帧', '首尾帧']
const modeRules: Record<string, (n: number) => boolean> = {
  '文生视频': (n) => n === 0,
  '首帧': (n) => n === 1,
  '首尾帧': (n) => n === 2,
  '全能参考': (n) => n >= 1,
}

function updateModeByImages() {
  const n = images.value.length
  if (!modeRules[selectedMode.value]?.(n)) {
    if (n === 0) selectedMode.value = '文生视频'
    else selectedMode.value = '全能参考'
  }
}

const isZhengSDModel = computed(() => /^(?:sd-2\.0-15s-(?:16x9|9x16)-480p|sd-2\.0-sec-480p)$/.test(selectedModel.value))
const ratioOptions = computed(() => isZhengSDModel.value ? ['16:9', '9:16'] : ['9:16', '16:9', '1:1'])
const resolutionOptions = computed(() => isZhengSDModel.value ? ['480p'] : ['480p', '720p', '1080p'])
const durationOptions = computed(() => {
  if (selectedModel.value === 'sd-2.0-sec-480p') return Array.from({ length: 12 }, (_, i) => String(i + 4))
  if (isZhengSDModel.value) return ['15']
  return Array.from({ length: 29 }, (_, i) => String(i + 2))
})
const countOptions = computed(() => isZhengSDModel.value ? [1] : [1, 2])

watch(countOptions, (options) => {
  if (!options.includes(selectedCount.value)) selectedCount.value = options[0]
})

interface UploadImage { id: number; name: string; dataUrl: string; mediaType: 'image' | 'video' | 'audio' }
const images = ref<UploadImage[]>([])
let imageCounter = 0
const connectedImages = ref<Map<string, UploadImage>>(new Map())

watch(() => images.value.length, () => {
  updateModeByImages()
})
const previewImg = ref<UploadImage | null>(null)
const modalOpen = ref(false)

function mediaLabel(mediaType: UploadImage['mediaType'], index: number): string {
  const prefix = mediaType === 'audio' ? '音频' : mediaType === 'video' ? '视频' : '图片'
  return `${prefix}${index + 1}`
}

function normalizeMediaNames() {
  const counters: Record<UploadImage['mediaType'], number> = { image: 0, audio: 0, video: 0 }
  for (const image of images.value) {
    image.name = mediaLabel(image.mediaType, counters[image.mediaType]++)
  }
}

function createMentionChip(reference: UploadImage): HTMLElement {
  const chip = document.createElement('span')
  chip.className = 'inline-flex items-center gap-0.5 align-middle'
  chip.contentEditable = 'false'
  chip.innerHTML = `<img src="${reference.dataUrl}" class="inline w-4 h-4 rounded object-cover" /><span class="text-blue-400">@${reference.name}</span>`
  chip.setAttribute('data-mention-id', String(reference.id))
  chip.setAttribute('data-mention', reference.name)
  return chip
}

// 删除或重排参考图后，按图片稳定 id / 缩略图地址重新绑定芯片，
// 这样图片3会跟随新编号变成图片2，被删除的图片芯片则直接移除。
function refreshMentionChips(anchor?: 'inline' | 'modal') {
  for (const editor of [editableRef.value, modalEditableRef.value]) {
    if (!editor) continue
    for (const mention of Array.from(editor.querySelectorAll<HTMLElement>('[data-mention]'))) {
      const mentionId = mention.dataset.mentionId
      const thumbnailUrl = mention.querySelector<HTMLImageElement>('img')?.getAttribute('src') || ''
      const reference = images.value.find(image => (
        (mentionId && String(image.id) === mentionId) ||
        (thumbnailUrl && image.dataUrl === thumbnailUrl)
      ))
      if (!reference) {
        mention.remove()
        continue
      }
      mention.dataset.mentionId = String(reference.id)
      mention.dataset.mention = reference.name
      const thumbnail = mention.querySelector<HTMLImageElement>('img')
      if (thumbnail && thumbnail.src !== reference.dataUrl) thumbnail.src = reference.dataUrl
      const label = mention.querySelector<HTMLElement>('.text-blue-400')
      if (label) label.textContent = `@${reference.name}`
    }
  }
  // When the modal is open it is the active source of truth. During initial
  // mount the inline editor can still be empty while promptHtml is being
  // restored, so never overwrite an existing prompt with that transient value.
  const sourceAnchor = anchor || (modalOpen.value ? 'modal' : 'inline')
  const source = sourceAnchor === 'modal' ? modalEditableRef.value : editableRef.value
  // An explicit edit (image removal from either editor) may legitimately leave
  // the editor empty. Without an explicit anchor, an empty editor is usually
  // just the pre-mount state and must not overwrite a saved plain-text prompt.
  if (source && (source.innerHTML || anchor)) syncPrompt(sourceAnchor)
}

function convertTextMentions(editor: HTMLElement | null) {
  if (!editor || images.value.length === 0) return
  const references = new Map(images.value.map(image => [image.name, image]))
  const walker = document.createTreeWalker(editor, NodeFilter.SHOW_TEXT)
  const textNodes: Text[] = []
  let current: globalThis.Node | null
  while ((current = walker.nextNode())) {
    if (!current.parentElement?.closest('[data-mention]')) textNodes.push(current as Text)
  }
  for (const textNode of textNodes) {
    const text = textNode.nodeValue || ''
    const pattern = /@(图片|音频|视频)\d+/g
    let match: RegExpExecArray | null
    let last = 0
    const fragment = document.createDocumentFragment()
    let changed = false
    while ((match = pattern.exec(text))) {
      const reference = references.get(match[0].slice(1))
      if (!reference) continue
      if (match.index > last) fragment.append(document.createTextNode(text.slice(last, match.index)))
      fragment.append(createMentionChip(reference))
      last = match.index + match[0].length
      changed = true
    }
    if (!changed) continue
    if (last < text.length) fragment.append(document.createTextNode(text.slice(last)))
    textNode.parentNode?.replaceChild(fragment, textNode)
  }
  if (editor === editableRef.value) syncPrompt()
}

const editableRef = ref<HTMLDivElement | null>(null)
const modalEditableRef = ref<HTMLDivElement | null>(null)
const showMention = ref(false)
const mentionFilter = ref('')
const mentionAnchor = ref<'inline' | 'modal'>('inline')
const mentionRange = ref<Range | null>(null)

function closeMentionOnEscape(event: KeyboardEvent) {
  if (event.key === 'Escape' && showMention.value) {
    event.preventDefault()
    event.stopPropagation()
    showMention.value = false
  }
}

onMounted(() => document.addEventListener('keydown', closeMentionOnEscape, true))
onUnmounted(() => document.removeEventListener('keydown', closeMentionOnEscape, true))

function syncPrompt(anchor?: 'inline' | 'modal') {
  const el = anchor === 'modal' ? modalEditableRef.value : editableRef.value
  if (el) {
    const div = document.createElement('div')
    div.innerHTML = el.innerHTML
    prompt.value = (div.textContent || '').trim()
    promptHtml.value = el.innerHTML
  }
}

function buildContent(extras: Record<string, any> = {}) {
  return JSON.stringify({ prompt: prompt.value, promptHtml: promptHtml.value, ...extras })
}

function compressImage(dataUrl: string): Promise<string> {
  return new Promise((resolve) => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onload = () => {
      try {
        // MegaBY requires every reference image dimension to be 300-6000px.
        // Keep the aspect ratio while avoiding the old 512px cap that made
        // wide or tall images fall below the 300px minimum on one side.
        const minSide = 300
        const maxSide = 2048
        const sourceMin = Math.min(img.width, img.height)
        const sourceMax = Math.max(img.width, img.height)
        const scale = Math.min(
          maxSide / sourceMax,
          sourceMin < minSide ? minSide / sourceMin : 1,
          6000 / sourceMax,
        )
        let w = Math.max(minSide, Math.round(img.width * scale))
        let h = Math.max(minSide, Math.round(img.height * scale))
        if (Math.max(w, h) > 6000) {
          const fit = 6000 / Math.max(w, h)
          w = Math.round(w * fit)
          h = Math.round(h * fit)
        }
        const c = document.createElement('canvas'); c.width = w; c.height = h
        c.getContext('2d')!.drawImage(img, 0, 0, w, h)
        resolve(c.toDataURL('image/jpeg', 0.7))
      } catch { resolve(dataUrl) }
    }
    img.onerror = () => resolve(dataUrl)
    img.src = dataUrl
  })
}

watch(() => props.nodeInputs, (inputs) => {
  if (!inputs) return
  const currentEdgeIds = new Set<string>()
  for (const inp of inputs) {
    const sourceUrl = inp.data?.dataUrl || inp.data?.url
    if ((inp.sourceNodeType === 'asset' || inp.sourceNodeType === 'image' || inp.sourceNodeType === 'video') && sourceUrl) {
      currentEdgeIds.add(inp.edgeId)
      if (!connectedImages.value.has(inp.edgeId)) {
        imageCounter++
        const localId = imageCounter
        const mediaType = inp.data?.mediaType || inferMediaType(sourceUrl)
        const add = (dataUrl: string) => {
          // Compression is asynchronous. The edge may have been removed while
          // it was in flight; do not resurrect that stale reference.
          const stillConnected = (props.nodeInputs || []).some(input => input.edgeId === inp.edgeId)
          if (!stillConnected) return
          const img = { id: localId, name: inp.data?.name || inp.data?.filename || `文件${localId}`, dataUrl, mediaType }
          connectedImages.value.set(inp.edgeId, img)
          images.value.push(img)
          images.value.sort((a, b) => a.id - b.id)
          normalizeMediaNames()
          nextTick(() => {
            refreshMentionChips()
            convertTextMentions(editableRef.value)
            convertTextMentions(modalEditableRef.value)
          })
        }
        if (mediaType === 'image') compressImage(sourceUrl).then(add)
        else add(sourceUrl)
      }
    }
  }
  for (const [edgeId, img] of connectedImages.value) {
    if (!currentEdgeIds.has(edgeId)) {
      connectedImages.value.delete(edgeId)
      images.value = images.value.filter(i => i.id !== img.id)
    }
  }
  updateModeByImages()
  nextTick(() => {
    refreshMentionChips()
    convertTextMentions(editableRef.value)
    convertTextMentions(modalEditableRef.value)
  })
}, { immediate: true, deep: true })

function inferMediaType(url: string): 'image' | 'video' | 'audio' {
  if (/\.(mp4|webm|mov|avi|mkv)(?:\?|$)/i.test(url)) return 'video'
  if (/\.(mp3|wav|ogg|flac|aac|m4a)(?:\?|$)/i.test(url)) return 'audio'
  return 'image'
}

function syncModels() {
  if (!props.node) return
  const typeModels = getModels(props.node.node_type)
  // 隐藏内部 Doubao 路由；MegaBY/ZhengAPI 模型保留各自前缀，避免同名冲突。
  const visibleModels = typeModels.filter(model => !model.startsWith('doubao-') && !model.includes('fast') && !model.endsWith('720p') && !model.endsWith('1080p'))
  models.value = visibleModels.length > 0 ? visibleModels : ['sd-2.0-15s-16x9-480p']
  if (!models.value.includes(selectedModel.value)) {
    selectedModel.value = models.value.find(model => isZhengAPIVideoModel(model)) || models.value[0]
  }
}

function isZhengAPIVideoModel(model: string) {
  return /^(?:sd-2\.0-15s-(?:16x9|9x16)-480p|sd-2\.0-sec-480p)$/.test(model)
}

watch([() => props.node?.node_type, modelInfoMap], syncModels, { immediate: true })

watch(selectedModel, (model) => {
  if (model === 'sd-2.0-sec-480p') {
    selectedRatio.value = '16:9'
    selectedResolution.value = '480p'
    selectedDuration.value = '15'
    return
  }
  const match = model.match(/^sd-2\.0-15s-(16x9|9x16)-480p$/)
  if (!match) return
  selectedRatio.value = match[1].replace('x', ':')
  selectedResolution.value = match[2]
  selectedDuration.value = '15'
}, { immediate: true })

watch([selectedRatio, selectedResolution], ([ratio, resolution]) => {
  if (!isZhengSDModel.value) return
  if (selectedModel.value === 'sd-2.0-sec-480p') return
  const family = 'sd-2.0'
  const model = `${family}-15s-${ratio.replace(':', 'x')}-${resolution}`
  if (models.value.includes(model)) selectedModel.value = model
})

watch(() => props.node, (n) => {
  if (n) {
    try {
      const data = n.content ? JSON.parse(n.content) : null
      if (data) { prompt.value = data.prompt || ''; promptHtml.value = data.promptHtml || ''; videos.value = data.videos || [] }
      else { prompt.value = n.content || ''; promptHtml.value = ''; videos.value = [] }
    } catch { prompt.value = n.content || ''; videos.value = [] }
    const connIds = new Set([...connectedImages.value.values()].map(i => i.id))
    // 保留未连线的手动上传图片，仅删除已断开的连线图片
    images.value = images.value.filter(i => {
      const isConnected = [...connectedImages.value.values()].some(ci => ci.id === i.id)
      return isConnected ? connIds.has(i.id) : true
    })
    normalizeMediaNames()
    updateModeByImages()
    nextTick(() => {
      if (editableRef.value) {
        if (promptHtml.value) {
          editableRef.value.innerHTML = promptHtml.value
          syncPrompt()
        } else {
          editableRef.value.textContent = prompt.value
        }
        convertTextMentions(editableRef.value)
      }
      convertTextMentions(modalEditableRef.value)
    })
  }
}, { immediate: true })

let promptSaveTimer: ReturnType<typeof setTimeout> | null = null
watch(prompt, () => {
  if (promptSaveTimer) clearTimeout(promptSaveTimer)
  promptSaveTimer = setTimeout(() => {
    const existing: any = {}
    try { if (props.node?.content) Object.assign(existing, JSON.parse(props.node.content)) } catch {}
    emit('save', buildContent({ videos: existing.videos || videos.value, _loading: existing._loading === true, _error: existing._error || '' }))
  }, 800)
})

function handleMention(anchor: 'inline' | 'modal') {
  const sel = window.getSelection()
  if (!sel || !sel.rangeCount) { showMention.value = false; return }
  const range = sel.getRangeAt(0); const node = range.startContainer
  if (node.nodeType !== Node.TEXT_NODE) { showMention.value = false; return }
  const text = node.textContent || ''; const cursorPos = range.startOffset
  const before = text.slice(0, cursorPos)
  const m = before.match(/@([^\s@]*)$/)
  if (m) {
    mentionFilter.value = m[1]
    mentionAnchor.value = anchor
    mentionRange.value = range.cloneRange()
    showMention.value = true
  }
  else { showMention.value = false }
}

function insertMention(img: { id: any; name: string; src: string }) {
  const el = mentionAnchor.value === 'modal' ? modalEditableRef.value : editableRef.value; if (!el) return
  const sel = window.getSelection(); if (!sel) return
  if (mentionRange.value) {
    sel.removeAllRanges()
    sel.addRange(mentionRange.value)
  }
  if (!sel.rangeCount) return
  const range = sel.getRangeAt(0); const node = range.startContainer
  if (node.nodeType !== Node.TEXT_NODE) return
  const text = node.textContent || ''; const pos = range.startOffset
  const m = text.slice(0, pos).match(/@([^\s@]*)$/); if (!m) return
  range.setStart(node, pos - m[0].length); range.setEnd(node, pos); range.deleteContents()
  const chip = document.createElement('span')
  chip.className = 'inline-flex items-center gap-0.5 align-middle'; chip.contentEditable = 'false'
  chip.innerHTML = `<img src="${img.src}" class="inline w-4 h-4 rounded object-cover" /><span class="text-blue-400">@${img.name}</span>`
  // Keep a stable reference to the underlying image. Display names are
  // renumbered after deletion, so they cannot be used to identify a chip.
  chip.setAttribute('data-mention-id', String(img.id))
  chip.setAttribute('data-mention', img.name)
  const space = document.createTextNode(' ')
  range.insertNode(space); range.insertNode(chip)
  sel.removeAllRanges(); range.setStartAfter(space); range.collapse(true); sel.addRange(range)
  showMention.value = false; mentionRange.value = null; syncPrompt()
}

function onAddMedia() {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*,video/*,audio/*'
  input.multiple = true
  input.onchange = () => {
    if (!input.files) return
    for (const file of input.files) {
      // Capture the id for this file before starting any asynchronous reads.
      // Using the mutable counter inside FileReader callbacks can give every
      // file in a multi-select the same id, making one delete remove them all.
      const localId = ++imageCounter
      const name = file.name || `文件${localId}`
      const reader = new FileReader()
      const isImage = file.type.startsWith('image/')
      const isVideo = file.type.startsWith('video/')
      const mediaType: 'image' | 'video' | 'audio' = isImage ? 'image' : isVideo ? 'video' : 'audio'
      if (isImage) {
        reader.onload = () => {
          compressImage(reader.result as string).then(compressed => {
            images.value.push({ id: localId, name, dataUrl: compressed, mediaType })
            images.value.sort((a, b) => a.id - b.id)
            normalizeMediaNames()
          })
        }
        reader.readAsDataURL(file)
      } else {
        reader.onload = () => {
          images.value.push({ id: localId, name, dataUrl: reader.result as string, mediaType })
          images.value.sort((a, b) => a.id - b.id)
          normalizeMediaNames()
        }
        reader.readAsDataURL(file)
      }
    }
  }
  input.click()
}

function removeImage(img: UploadImage, anchor: 'inline' | 'modal' = 'inline') {
  images.value = images.value.filter(i => i.id !== img.id)
  for (const [edgeId, connImg] of connectedImages.value) {
    if (connImg.id === img.id) { connectedImages.value.delete(edgeId); emit('remove-connected-edge', edgeId); break }
  }
  normalizeMediaNames()
  updateModeByImages()
  nextTick(() => {
    refreshMentionChips(anchor)
    convertTextMentions(editableRef.value)
    convertTextMentions(modalEditableRef.value)
    emit('save', buildContent({ videos: videos.value, _loading: false, _error: '' }))
  })
}

function onInput() {
  syncPrompt()
  convertTextMentions(editableRef.value)
  handleMention('inline')
}

function onModalInput() {
  syncPrompt('modal')
  convertTextMentions(modalEditableRef.value)
  handleMention('modal')
}

async function uploadToImageHost(dataUrl: string, forceHost = false): Promise<string> {
  const res = await fetch('/api/upload-image-host', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ data_url: dataUrl, force_host: forceHost }),
  })
  if (!res.ok) throw new Error('upload failed')
  const data = await res.json()
  const url = String(data.url || '')
  if (forceHost && !/^https?:\/\//i.test(url)) throw new Error('图床未返回公网直链')
  return url
}

async function ensurePublicMediaURL(url: string, forceHost = false): Promise<string> {
  if (!forceHost && /^https?:\/\//i.test(url)) return url
  // 画布内的历史素材通常保存为 /uploads/...，先读取后转成 data URL 上传图床。
  const response = await fetch(url)
  if (!response.ok) throw new Error(`无法读取本地参考素材（${response.status}）`)
  const blob = await response.blob()
  const dataURL = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error || new Error('读取参考素材失败'))
    reader.readAsDataURL(blob)
  })
  // Local/data URLs are never reachable by an upstream provider. They must be
  // published as an actual HTTP(S) URL regardless of the provider setting.
  return uploadToImageHost(dataURL, true)
}

async function generate() {
  if (!prompt.value || loading.value) return
  loading.value = true
  const modelConfig = getModelConfig(props.node?.node_type || 'video', selectedModel.value)
  try {
    const body: Record<string, any> = { model: selectedModel.value, prompt: prompt.value, seconds: selectedDuration.value, ratio: selectedRatio.value, resolution: selectedResolution.value, n: selectedCount.value, mode: selectedMode.value }
    if (modelConfig) {
      if (modelConfig.channel) body.channel = modelConfig.channel
      if (modelConfig.base_url) body.base_url = modelConfig.base_url
      if (modelConfig.api_key) body.api_key = modelConfig.api_key
      if (modelConfig.protocol) body.protocol = modelConfig.protocol
    }
    // 只用 images.value（已排序），过滤 image 类型用于上传图床，video/audio 直接传 URL
    const imgUrls = images.value.filter(i => i.mediaType === 'image').map(i => i.dataUrl).filter(Boolean)
    const vidRefs = images.value.filter(i => i.mediaType === 'video').map(i => i.dataUrl).filter(Boolean)
    const audRefs = images.value.filter(i => i.mediaType === 'audio').map(i => i.dataUrl).filter(Boolean)
    const protocol = String(modelConfig?.protocol || '').toLowerCase()
    const requiresPublicImageURLs = protocol === 'megaby_video'
      || protocol === 'geeknow_video'
      || protocol === 'zhengapi_video'
      || /^(?:megaby-|wan[-_]?3(?:\.0)?)/i.test(selectedModel.value)
    // MegaBY 等上游要求参考视频必须是公网可读的绝对 http/https URL；本地 /uploads 或 data: 地址先上传图床再提交。
    if (vidRefs.length > 0) {
      const publicVidRefs = await Promise.all(vidRefs.map(async url => {
        try { return await ensurePublicMediaURL(url, requiresPublicImageURLs) } catch (error: any) {
          if (requiresPublicImageURLs) throw new Error(`参考视频上传公网图床失败：${error?.message || '未知错误'}`)
          return url
        }
      }))
      body.video_urls = publicVidRefs
    }
    // MegaBY 需要能从公网读取音频；本地 /uploads 地址先上传图床再提交。
    if (audRefs.length > 0) {
      const publicAudRefs = await Promise.all(audRefs.map(async url => {
        try { return await ensurePublicMediaURL(url, requiresPublicImageURLs) } catch (error: any) {
          if (requiresPublicImageURLs) throw new Error(`参考音频上传公网图床失败：${error?.message || '未知错误'}`)
          return url
        }
      }))
      body.audio_urls = publicAudRefs
    }
    const uploadedUrls: string[] = []
    const forceImageHost = /wan[-_]?3(?:\.0)?/i.test(selectedModel.value)
    for (const url of imgUrls) {
      try {
        uploadedUrls.push(url.startsWith('data:')
          ? await uploadToImageHost(url, true)
          : await ensurePublicMediaURL(url, requiresPublicImageURLs || forceImageHost))
      } catch (error: any) {
        if (requiresPublicImageURLs || forceImageHost) {
          throw new Error(`参考图上传公网图床失败：${error?.message || '未知错误'}`)
        }
        uploadedUrls.push(url)
      }
    }
    if (selectedMode.value !== '文生视频' && uploadedUrls.length > 0) {
      if (selectedMode.value === '首帧') body.image_url = uploadedUrls[0]
      else if (selectedMode.value === '首尾帧') body.image_urls = uploadedUrls.slice(0, 2)
      else if (selectedMode.value === '全能参考') body.image_urls = uploadedUrls
    }

    emit('save', buildContent({ videos: videos.value, _loading: true, _error: '' }))

    const res = await fetch('/api/video/generate', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    if (!res.ok) { const err = await res.json().catch(() => ({ error: res.statusText })); throw new Error(typeof err.error === 'string' ? err.error : (err.error?.message || res.statusText)) }
    const data = await res.json()
    if (data.videos && Array.isArray(data.videos)) {
      videos.value = []
      for (let i = 0; i < data.videos.length; i++) videos.value.push({ id: Date.now() + i, url: data.videos[i] })
    }
    emit('save', buildContent({ videos: videos.value, _loading: false, _error: '' }))
  } catch (e: any) {
    emit('save', buildContent({ videos: videos.value, _loading: false, _error: e.message || '视频生成失败' }))
  } finally {
    loading.value = false
  }
}

watch(modalOpen, async (v) => {
  if (v) {
    await nextTick()
    if (modalEditableRef.value) {
      modalEditableRef.value.innerHTML = promptHtml.value || prompt.value
    }
  } else {
    if (editableRef.value && modalEditableRef.value) {
      editableRef.value.innerHTML = modalEditableRef.value.innerHTML
      syncPrompt()
    }
  }
})
</script>

<template>
  <div v-if="node" :style="panelStyle" class="pointer-events-auto" @pointerdown.stop>
    <div class="bg-neutral-900/95 backdrop-blur rounded-2xl border border-white/20 p-3 relative">
      <button class="btn btn-xs btn-square absolute top-2 right-2 bg-white/10 border border-white/30 text-white hover:bg-white/20 z-10" title="放大编辑" @click="modalOpen = true">
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="15 3 21 3 21 9"/><polyline points="9 21 3 21 3 15"/><line x1="21" y1="3" x2="14" y2="10"/><line x1="3" y1="21" x2="10" y2="14"/>
        </svg>
      </button>

      <!-- 模式 tabs -->
      <div class="flex gap-1 mb-2">
        <button v-for="m in allModes" :key="m"
          class="text-[10px] px-2 py-0.5 rounded transition-colors"
          :class="modeRules[m](images.length) ? (selectedMode === m ? 'bg-white/15 text-white' : 'text-white/40 hover:text-white/60') : 'text-white/15 cursor-not-allowed'"
          :disabled="!modeRules[m](images.length)"
          @click="selectedMode = m"
        >{{ m }}</button>
      </div>

      <div class="flex items-center gap-1.5 mb-2">
        <div class="shrink-0 w-10 h-10 rounded-lg border border-white/15 flex flex-col items-center justify-center text-white/40 gap-0.5">
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"><path d="M2 5V3a1 1 0 0 1 1-1h2M11 2h2a1 1 0 0 1 1 1v2M14 11v2a1 1 0 0 1-1 1h-2M5 14H3a1 1 0 0 1-1-1v-2"/></svg>
          <span class="text-[8px] leading-none">识图</span>
        </div>
        <div v-for="(img, i) in images" :key="img.id" class="relative shrink-0 w-10 h-10 rounded-lg overflow-hidden border border-white/15 cursor-pointer hover:border-white/50 transition-colors" @click.stop="previewImg = img">
          <img v-if="img.mediaType === 'image'" :src="img.dataUrl" class="w-full h-full object-cover" />
          <div v-else class="w-full h-full flex items-center justify-center text-white/20">
            <svg v-if="img.mediaType === 'video'" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/><path d="M19 10v2a7 7 0 0 1-14 0v-2"/><line x1="12" y1="19" x2="12" y2="23"/><line x1="8" y1="23" x2="16" y2="23"/></svg>
          </div>
          <span class="absolute bottom-0 left-0 right-0 text-[7px] text-center bg-black/60 text-white/80 truncate px-0.5 leading-tight">{{ img.name }}</span>
          <button class="absolute -top-1 -right-1 w-4 h-4 rounded-full bg-neutral-800 border border-white/20 text-white/60 hover:text-white flex items-center justify-center text-[10px] leading-none" @click.stop="removeImage(img, 'inline')">×</button>
        </div>
        <button class="shrink-0 w-10 h-10 rounded-lg border border-white/15 flex flex-col items-center justify-center text-white/40 hover:text-white/60 hover:border-white/25 transition-colors gap-0.5" @click="onAddMedia">
          <span class="text-sm leading-none">+</span>
          <span class="text-[8px] leading-none">添加</span>
        </button>
      </div>

      <div v-if="showMention && mentionAnchor === 'inline'" class="relative">
        <MentionDropdown :connected-images="images" :filter="mentionFilter" @insert="insertMention" />
      </div>

      <div ref="editableRef" contenteditable="true" class="prompt-editor w-full h-[200px] min-h-[200px] max-h-[200px] bg-transparent outline-0 text-white text-sm leading-5 px-1 overflow-y-auto whitespace-pre-wrap break-words empty:before:content-['输入视频提示词...'] empty:before:text-white/30" @input="onInput" />

      <div class="flex items-center justify-between gap-1 mt-3 pt-3 border-t border-white/10">
        <div class="flex items-center gap-0.5 min-w-0">
          <div class="relative inline-flex items-center">
            <select v-model="selectedModel" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-4 w-max min-w-0 shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="m in models" :key="m" :value="m" class="bg-neutral-900 text-white">{{ modelLabel(m) }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <div class="relative inline-flex items-center">
            <select v-model="selectedRatio" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-4 w-max min-w-0 shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="r in ratioOptions" :key="r" :value="r" class="bg-neutral-900 text-white">{{ r }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <div class="relative inline-flex items-center">
            <select v-model="selectedResolution" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-4 w-max min-w-0 shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="r in resolutionOptions" :key="r" :value="r" class="bg-neutral-900 text-white">{{ r }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <div class="relative inline-flex items-center">
            <select v-model="selectedDuration" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-4 w-max min-w-0 shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="d in durationOptions" :key="d" :value="d" class="bg-neutral-900 text-white">{{ d }}s</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <div class="relative inline-flex items-center">
            <select v-model="selectedCount" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-4 w-max min-w-0 shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="c in countOptions" :key="c" :value="c" class="bg-neutral-900 text-white">{{ c }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
        </div>
        <button class="shrink-0 w-9 h-9 rounded-full bg-white flex items-center justify-center text-neutral-900 hover:bg-neutral-200 disabled:opacity-50" :disabled="loading" @click="generate">
          <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>
          <span v-else class="loading loading-spinner loading-xs" />
        </button>
      </div>
    </div>
  </div>

  <Teleport to="body">
    <div v-if="previewImg" class="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80" @click="previewImg = null">
      <img :src="previewImg.dataUrl" class="max-w-[90vw] max-h-[90vh] object-contain rounded-lg" @click.stop />
      <button class="absolute top-4 right-4 btn btn-sm btn-circle bg-white/10 border border-white/30 text-white hover:bg-white/20" @click="previewImg = null">
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
      </button>
    </div>
  </Teleport>

  <Teleport to="body">
    <div v-if="modalOpen" class="fixed inset-0 z-[9998] flex items-center justify-center bg-black/70" @click.self="modalOpen = false">
      <div class="bg-neutral-900 border border-white/20 rounded-2xl w-[720px] max-h-[85vh] flex flex-col shadow-xl">
        <div class="flex items-center justify-between px-4 py-3 border-b border-white/10">
          <span class="text-sm font-medium text-white/70">视频生成</span>
          <button class="btn btn-xs btn-square bg-white/10 border border-white/30 text-white hover:bg-white/20" @click="modalOpen = false">
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
        <div class="flex-1 overflow-y-auto p-5 space-y-3">
          <div class="flex gap-1">
            <button v-for="m in allModes" :key="m"
              class="text-xs px-2 py-0.5 rounded transition-colors"
              :class="modeRules[m](images.length) ? (selectedMode === m ? 'bg-white/15 text-white' : 'text-white/40 hover:text-white/60') : 'text-white/15 cursor-not-allowed'"
              :disabled="!modeRules[m](images.length)"
              @click="selectedMode = m"
            >{{ m }}</button>
          </div>
          <div v-if="images.length > 0" class="flex flex-wrap gap-2">
            <div v-for="(img, i) in images" :key="img.id" class="relative w-16 h-16 rounded-lg overflow-hidden border border-white/15 cursor-pointer hover:border-white/50 transition-colors" @click.stop="previewImg = img">
              <img v-if="img.mediaType === 'image'" :src="img.dataUrl" class="w-full h-full object-cover" />
              <div v-else class="w-full h-full flex items-center justify-center text-white/20">
                <svg v-if="img.mediaType === 'video'" xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg>
                <svg v-else xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/><path d="M19 10v2a7 7 0 0 1-14 0v-2"/><line x1="12" y1="19" x2="12" y2="23"/><line x1="8" y1="23" x2="16" y2="23"/></svg>
              </div>
              <span class="absolute bottom-0 left-0 right-0 text-[7px] text-center bg-black/60 text-white/80 truncate px-0.5">{{ img.name }}</span>
              <button class="absolute -top-1 -right-1 w-5 h-5 rounded-full bg-neutral-800 border border-white/20 text-white/60 hover:text-white flex items-center justify-center text-xs leading-none" @click.stop="removeImage(img, 'modal')">×</button>
            </div>
          </div>
          <div class="relative">
            <div v-if="showMention && mentionAnchor === 'modal'" class="relative z-20">
              <MentionDropdown placement="inline" :connected-images="images" :filter="mentionFilter" @insert="insertMention" />
            </div>
            <div ref="modalEditableRef" contenteditable="true" class="prompt-editor w-full h-[200px] min-h-[200px] max-h-[200px] bg-transparent outline-0 text-white text-sm leading-5 overflow-y-auto whitespace-pre-wrap break-words empty:before:content-['输入视频提示词...'] empty:before:text-white/30" @input="onModalInput" />
          </div>
        </div>
        <div class="flex items-center justify-between gap-2 px-4 py-3 border-t border-white/10">
          <div class="flex items-center gap-1 flex-wrap">
            <div class="relative inline-flex items-center">
              <select v-model="selectedModel" class="text-xs bg-white/5 border border-white/10 rounded text-white/70 hover:text-white h-7 py-0 pl-1.5 pr-4 outline-none appearance-none cursor-pointer [color-scheme:dark]">
                <option v-for="m in models" :key="m" :value="m" class="bg-neutral-900 text-white">{{ modelLabel(m) }}</option>
              </select>
              <svg class="pointer-events-none absolute right-1 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
            <div class="relative inline-flex items-center">
              <select v-model="selectedRatio" class="text-xs bg-white/5 border border-white/10 rounded text-white/70 hover:text-white h-7 py-0 pl-1.5 pr-4 outline-none appearance-none cursor-pointer [color-scheme:dark]">
                <option v-for="r in ratioOptions" :key="r" :value="r" class="bg-neutral-900 text-white">{{ r }}</option>
              </select>
              <svg class="pointer-events-none absolute right-0.5 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
            <div class="relative inline-flex items-center">
              <select v-model="selectedResolution" class="text-xs bg-white/5 border border-white/10 rounded text-white/70 hover:text-white h-7 py-0 pl-1.5 pr-4 outline-none appearance-none cursor-pointer [color-scheme:dark]">
                <option v-for="r in resolutionOptions" :key="r" :value="r" class="bg-neutral-900 text-white">{{ r }}</option>
              </select>
              <svg class="pointer-events-none absolute right-0.5 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
            <div class="relative inline-flex items-center">
              <select v-model="selectedDuration" class="text-xs bg-white/5 border border-white/10 rounded text-white/70 hover:text-white h-7 py-0 pl-1.5 pr-4 outline-none appearance-none cursor-pointer [color-scheme:dark]">
                <option v-for="d in durationOptions" :key="d" :value="d" class="bg-neutral-900 text-white">{{ d }}s</option>
              </select>
              <svg class="pointer-events-none absolute right-0.5 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
            <div class="relative inline-flex items-center">
              <select v-model="selectedCount" class="text-xs bg-white/5 border border-white/10 rounded text-white/70 hover:text-white h-7 py-0 pl-1.5 pr-4 outline-none appearance-none cursor-pointer [color-scheme:dark]">
                <option v-for="c in countOptions" :key="c" :value="c" class="bg-neutral-900 text-white">{{ c }}</option>
              </select>
              <svg class="pointer-events-none absolute right-0.5 top-1/2 -translate-y-1/2 text-white/70" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
          </div>
          <button class="shrink-0 w-9 h-9 rounded-full bg-white flex items-center justify-center text-neutral-900 hover:bg-neutral-200 disabled:opacity-50" :disabled="loading" @click="generate">
            <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>
            <span v-else class="loading loading-spinner loading-xs" />
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
