<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import type { Node } from '../../types'
import MentionDropdown from '../MentionDropdown.vue'
import { useNodeConfigs } from '../../composables/useNodeConfigs'
import { loadImagePresets } from '../../composables/useImagePresets'

const props = defineProps<{
  node: Node | null
  panelStyle: Record<string, string>
  nodeInputs?: { edgeId: string; sourceNodeId: string; sourceNodeType: string; sourceNodeName?: string; data: any }[]
}>()

const emit = defineEmits<{
  (e: 'save', content: string): void
  (e: 'remove-connected-edge', edgeId: string): void
}>()

const { modelInfoMap, getModels, getModelConfig } = useNodeConfigs()

const prompt = ref('')
const displayName = ref('')
const promptHtml = ref('')
const loading = ref(false)
const errorMessage = ref('')
const modalOpen = ref(false)
const images = ref<{ id: number; url: string; name?: string; mediaType?: 'image' | 'video' | 'audio' }[]>([])
const generatedImages = ref<{ id: number; url: string }[]>([])
const connectedImages = ref<Map<string, { id: string; url: string; name: string; displayName?: string; mediaType?: 'image' | 'video' | 'audio' }>>(new Map())
let imageCounter = 0
const previewImg = ref<{ id: number | string; url: string } | null>(null)

const selectedModel = ref('gpt-image-2')
const models = ref<string[]>(['gpt-image-2'])
const selectedRatio = ref('1:1')
const selectedResolution = ref('1K')
const selectedCount = ref(1)
const selectedPreset = ref('')

const defaultRatioOptions = ['自适应', '1:1', '4:3', '3:4', '3:2', '2:3', '16:9', '9:16', '5:4', '4:5', '21:9']
const ratioOptions = computed(() => {
  const model = selectedModel.value
  if (model === 'gpt-image-2' || model === 'gpt-image-2-pro') return ['1:1', '4:3', '3:2', '2:3', '16:9', '9:16']
  if (model === 'gpt-image-2-vip' || model.startsWith('doubao-seedream-') || model === 'grok-4-2-image') return ['1:1', '4:3', '3:4', '3:2', '2:3', '16:9', '9:16', '21:9']
  if (model.startsWith('firefly-gpt-image')) return ['自适应', '1:1', '16:9', '9:16', '21:9', '4:3', '3:2', '2:3', '5:4', '4:5', '3:4']
  if (model.startsWith('firefly-nano-banana2')) return ['1:1', '16:9', '9:16', '4:3', '3:4', '21:9', '3:2', '2:3', '5:4', '4:5', '8:1', '1:4', '1:8']
  if (model.startsWith('firefly-nano-banana')) return ['1:1', '16:9', '9:16', '4:3', '3:4', '21:9', '5:4', '4:5']
  return defaultRatioOptions
})
const resolutionOptions = computed(() => {
  if (selectedModel.value === 'gpt-image-2') return ['1K']
  if (selectedModel.value === 'gpt-image-2-pro') return ['2K', '4K']
  if (selectedModel.value.startsWith('doubao-seedream-') || selectedModel.value === 'grok-4-2-image') return ['2K']
  return ['1K', '2K', '4K']
})
const countOptions = computed(() => selectedModel.value.startsWith('firefly-') ? [1] : [1, 2, 4])

const presets = ref<{ id: string; name: string; prompt: string; category: string; scope: string }[]>([])
const presetCategories = ref<string[]>([])

const editableRef = ref<HTMLDivElement | null>(null)
const modalContentEditable = ref<HTMLDivElement | null>(null)
const showMention = ref(false)
const mentionFilter = ref('')
const mentionAnchor = ref<'inline' | 'modal'>('inline')
const mentionRange = ref<Range | null>(null)

interface DisplayImage {
  id: number | string
  url: string
  label: string
  displayLabel: string
  isRef: boolean
  mediaType?: 'image' | 'video' | 'audio'
}

const allDisplayImages = ref<DisplayImage[]>([])
let nodeInitialized = false

function deriveDisplayName(text: string): string {
  const firstLine = text.split(/\r?\n/).map(line => line.trim()).find(Boolean) || ''
  return firstLine.length > 32 ? `${firstLine.slice(0, 32)}…` : firstLine
}

function mediaLabel(mediaType: DisplayImage['mediaType'], index: number): string {
  const prefix = mediaType === 'audio' ? '音频' : mediaType === 'video' ? '视频' : '图片'
  return `${prefix}${index + 1}`
}

watch(ratioOptions, (options) => {
  if (!options.includes(selectedRatio.value)) selectedRatio.value = options[0]
})
watch(countOptions, (options) => {
  if (!options.includes(selectedCount.value)) selectedCount.value = options[0]
})
watch(resolutionOptions, (options) => {
  if (!options.includes(selectedResolution.value)) selectedResolution.value = options[0]
})

function rebuildDisplay() {
  const list: DisplayImage[] = []
  const counters: Record<string, number> = {}
  for (const [, img] of connectedImages.value) {
    const mediaType = img.mediaType || 'image'
    const index = counters[mediaType] || 0
    counters[mediaType] = index + 1
    list.push({ id: img.id, url: img.url, label: img.name, displayLabel: mediaLabel(mediaType, index), isRef: true, mediaType })
  }
  for (const img of images.value) {
    const label = img.name || '上传'
    const mediaType = img.mediaType || 'image'
    const index = counters[mediaType] || 0
    counters[mediaType] = index + 1
    list.push({ id: img.id, url: img.url, label, displayLabel: mediaLabel(mediaType, index), isRef: false, mediaType })
  }
  allDisplayImages.value = list
  nextTick(() => {
    refreshMentionThumbnails()
    convertTextMentions(editableRef.value)
    convertTextMentions(modalContentEditable.value)
  })
}

function createMentionChip(reference: DisplayImage): HTMLElement {
  const chip = document.createElement('span')
  chip.className = 'inline-flex items-center gap-0.5 align-middle'
  chip.contentEditable = 'false'
  const thumbnail = document.createElement('img')
  thumbnail.src = reference.url
  thumbnail.className = 'inline w-4 h-4 rounded object-cover'
  const label = document.createElement('span')
  label.className = 'text-blue-400'
  label.textContent = `@${reference.displayLabel}`
  chip.append(thumbnail, label)
  chip.setAttribute('data-mention-id', String(reference.id))
  chip.setAttribute('data-mention', reference.label)
  return chip
}

function convertTextMentions(editor: HTMLElement | null) {
  if (!editor || allDisplayImages.value.length === 0) return
  const references = new Map(allDisplayImages.value.map(reference => [reference.displayLabel, reference]))
  const walker = document.createTreeWalker(editor, NodeFilter.SHOW_TEXT)
  const textNodes: Text[] = []
  let current: globalThis.Node | null
  while ((current = walker.nextNode())) {
    if (!(current.parentElement?.closest('[data-mention]'))) textNodes.push(current as Text)
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
  if (editor === editableRef.value) {
    promptHtml.value = editor.innerHTML
    updatePromptPlainText()
  }
}

// Mention chips are only a visual representation. Their image URL must follow
// the current source node instead of retaining a stale URL saved in promptHtml.
function refreshMentionThumbnails() {
  const references = new Map(allDisplayImages.value.map(image => [String(image.id), image]))
  let updatedHtml = false
  for (const editor of [editableRef.value, modalContentEditable.value]) {
    if (!editor) continue
    let editorChanged = false
    for (const mention of editor.querySelectorAll<HTMLElement>('[data-mention-id]')) {
      const reference = references.get(mention.dataset.mentionId || '')
      const thumbnail = mention.querySelector<HTMLImageElement>('img')
      const label = mention.querySelector<HTMLElement>('.text-blue-400')
      if (!reference) continue
      if (thumbnail && thumbnail.getAttribute('src') !== reference.url) {
        thumbnail.src = reference.url
        editorChanged = true
      }
      if (label && label.textContent !== `@${reference.displayLabel}`) {
        label.textContent = `@${reference.displayLabel}`
        editorChanged = true
      }
    }
    if (editorChanged && editor === editableRef.value) {
      promptHtml.value = editor.innerHTML
      updatedHtml = true
    }
  }
  if (updatedHtml) updatePromptPlainText()
}

function updatePromptPlainText() {
  if (!promptHtml.value) return
  const container = document.createElement('div')
  container.innerHTML = promptHtml.value
  prompt.value = (container.textContent || '').trim()
}

watch(() => props.nodeInputs, (inputs) => {
  if (!inputs) return
  const currentEdgeIds = new Set<string>()
  for (const inp of inputs) {
    const sourceUrl = inp.data?.dataUrl || inp.data?.url
    if ((inp.sourceNodeType === 'asset' || inp.sourceNodeType === 'image' || inp.sourceNodeType === 'video') && sourceUrl) {
      currentEdgeIds.add(inp.edgeId)
      const name = typeof inp.data.name === 'string' && inp.data.name.trim()
        ? inp.data.name.trim()
        : `参考-${inp.sourceNodeId.slice(-6)}`
      const current = connectedImages.value.get(inp.edgeId)
      const displayName = inp.sourceNodeName || name
      const mediaType = inp.data?.mediaType || inferMediaType(sourceUrl)
      if (!current || current.id !== inp.sourceNodeId || current.url !== sourceUrl || current.name !== name || current.displayName !== displayName) {
        connectedImages.value.set(inp.edgeId, { id: inp.sourceNodeId, url: sourceUrl, name, displayName, mediaType })
      }
    }
  }
  for (const [edgeId] of connectedImages.value) {
    if (!currentEdgeIds.has(edgeId)) {
      connectedImages.value.delete(edgeId)
    }
  }
  rebuildDisplay()
}, { immediate: true, deep: true })

function inferMediaType(url: string): 'image' | 'video' | 'audio' {
  if (/\.(mp4|webm|mov|avi|mkv)(?:\?|$)/i.test(url)) return 'video'
  if (/\.(mp3|wav|ogg|flac|aac|m4a)(?:\?|$)/i.test(url)) return 'audio'
  return 'image'
}

function syncModels() {
  if (!props.node) return
  const typeModels = getModels(props.node.node_type)
  models.value = typeModels.length > 0 ? typeModels : ['gpt-image-2']
  if (!models.value.includes(selectedModel.value)) {
    selectedModel.value = models.value[0] || 'gpt-image-2'
  }
}

watch([() => props.node?.node_type, modelInfoMap], syncModels, { immediate: true })

watch(() => props.node, (n) => {
  if (n) {
    let shouldPersistDerivedName = false
    try {
      const data = n.content ? JSON.parse(n.content) : null
      if (data) {
        prompt.value = data.prompt || ''
        displayName.value = data.displayName || ''
        if (!displayName.value) {
          displayName.value = deriveDisplayName(prompt.value)
          shouldPersistDerivedName = Boolean(displayName.value)
        }
        promptHtml.value = data.promptHtml || ''
        if (typeof data.model === 'string' && data.model) selectedModel.value = data.model
        if (typeof data.aspect_ratio === 'string' && ratioOptions.value.includes(data.aspect_ratio)) selectedRatio.value = data.aspect_ratio
        if (typeof data.image_size === 'string' && resolutionOptions.value.includes(data.image_size)) selectedResolution.value = data.image_size
        if (typeof data.n === 'number' && countOptions.value.includes(data.n)) selectedCount.value = data.n
        images.value = data.images || []
        generatedImages.value = data.generated_images || []
        errorMessage.value = typeof data._error === 'string' ? data._error : ''
        if (!nodeInitialized && data._loading === true) {
          emit('save', buildContent({ images: images.value, generated_images: generatedImages.value, _loading: false, _error: '任务连接已中断，请重新生成' }))
        }
      } else {
        prompt.value = n.content || ''
        displayName.value = ''
        displayName.value = deriveDisplayName(prompt.value)
        promptHtml.value = ''
        images.value = []
        generatedImages.value = []
      }
    } catch {
      prompt.value = n.content || ''
      displayName.value = ''
      displayName.value = deriveDisplayName(prompt.value)
      promptHtml.value = ''
      images.value = []
      generatedImages.value = []
    }
    rebuildDisplay()
    nextTick(() => {
      if (editableRef.value) {
        if (promptHtml.value) {
          editableRef.value.innerHTML = promptHtml.value
          refreshMentionThumbnails()
        } else {
          editableRef.value.textContent = prompt.value
        }
        convertTextMentions(editableRef.value)
      }
      convertTextMentions(modalContentEditable.value)
    })
    loadPresets()
    nodeInitialized = true
    if (shouldPersistDerivedName) {
      const existing: any = {}
      try { Object.assign(existing, JSON.parse(n.content || '{}')) } catch {}
      emit('save', JSON.stringify({ ...existing, displayName: displayName.value }))
    }
  }
}, { immediate: true })

let promptSaveTimer: ReturnType<typeof setTimeout> | null = null
watch(prompt, () => {
  if (promptSaveTimer) clearTimeout(promptSaveTimer)
  promptSaveTimer = setTimeout(() => {
    const existing: any = {}
    try { if (props.node?.content) Object.assign(existing, JSON.parse(props.node.content)) } catch {}
    emit('save', buildContent({ images: existing.images || images.value, generated_images: existing.generated_images || generatedImages.value }))
  }, 800)
})

watch(displayName, () => {
  if (!props.node) return
  const existing: any = {}
  try { if (props.node.content) Object.assign(existing, JSON.parse(props.node.content)) } catch {}
  emit('save', buildContent({ images: existing.images || images.value, generated_images: existing.generated_images || generatedImages.value }))
})

async function loadPresets() {
  if (!props.node?.canvas_id) return
  try {
    const all = await loadImagePresets(props.node.canvas_id)
    presets.value = all
    presetCategories.value = [...new Set(all.map((p: any) => p.category))]
  } catch {}
}

function syncPrompt(anchor?: 'inline' | 'modal') {
  const el = anchor === 'modal' ? modalContentEditable.value : editableRef.value
  if (el) {
    const div = document.createElement('div')
    div.innerHTML = el.innerHTML
    prompt.value = (div.textContent || '').trim()
    promptHtml.value = el.innerHTML
  }
}

function buildContent(extras: Record<string, any> = {}) {
  return JSON.stringify({
    prompt: prompt.value,
    promptHtml: promptHtml.value,
    displayName: displayName.value.trim(),
    model: selectedModel.value,
    aspect_ratio: selectedRatio.value,
    image_size: selectedResolution.value,
    n: selectedCount.value,
    ...extras,
  })
}

function compressImage(dataUrl: string): Promise<string> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => {
      const max = 1024; let w = img.width, h = img.height
      if (w > h && w > max) { h = Math.round(h * max / w); w = max }
      else if (h > max) { w = Math.round(w * max / h); h = max }
      const c = document.createElement('canvas'); c.width = w; c.height = h
      c.getContext('2d')!.drawImage(img, 0, 0, w, h)
      resolve(c.toDataURL('image/jpeg', 0.85))
    }
    img.onerror = () => resolve(dataUrl)
    img.src = dataUrl
  })
}

function onAddImage() {
  const input = document.createElement('input')
  input.type = 'file'; input.accept = 'image/*'; input.multiple = true
  input.onchange = () => {
    if (!input.files) return
    for (const file of input.files) {
      const id = ++imageCounter
      const name = `图片${id}`
      const reader = new FileReader()
      reader.onload = () => {
        compressImage(reader.result as string).then(compressed => {
          images.value.push({ id, name, url: compressed })
          rebuildDisplay()
          emit('save', buildContent({ images: images.value, generated_images: generatedImages.value }))
        })
      }
      reader.readAsDataURL(file)
    }
  }
  input.click()
}

function applyPreset() {
  if (!selectedPreset.value) return
  const p = presets.value.find(pr => pr.id === selectedPreset.value)
  if (p) {
    prompt.value = p.prompt + ', ' + prompt.value
    if (editableRef.value) editableRef.value.textContent = prompt.value
    selectedPreset.value = ''
    syncPrompt()
  }
}

function onInput() {
  syncPrompt()
  convertTextMentions(editableRef.value)
  handleMention('inline')
}

function onModalInput() {
  syncPrompt('modal')
  convertTextMentions(modalContentEditable.value)
  handleMention('modal')
}

function handleMention(anchor: 'inline' | 'modal') {
  const sel = window.getSelection()
  if (!sel || !sel.rangeCount) { showMention.value = false; return }
  const range = sel.getRangeAt(0)
  const node = range.startContainer
  if (node.nodeType !== Node.TEXT_NODE) { showMention.value = false; return }
  const text = node.textContent || ''
  const cursorPos = range.startOffset
  const before = text.slice(0, cursorPos)
  const m = before.match(/@([^\s@]*)$/)
  if (m) {
    mentionFilter.value = m[1]
    mentionAnchor.value = anchor
    mentionRange.value = range.cloneRange()
    showMention.value = true
  } else {
    showMention.value = false
    mentionRange.value = null
  }
}

function insertMention(img: { id: any; name: string; displayName?: string; src: string; mediaType?: 'image' | 'video' | 'audio' }) {
  const el = mentionAnchor.value === 'modal' ? modalContentEditable.value : editableRef.value; if (!el) return
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
  if (!allDisplayImages.value.some(item => item.url === img.src)) {
    imageCounter++
    images.value.push({ id: imageCounter, name: img.name, url: img.src, mediaType: img.mediaType })
    rebuildDisplay()
  }
  const chip = document.createElement('span')
  chip.className = 'inline-flex items-center gap-0.5 align-middle'; chip.contentEditable = 'false'
  const thumbnail = document.createElement('img')
  thumbnail.src = img.src
  thumbnail.className = 'inline w-4 h-4 rounded object-cover'
  const label = document.createElement('span')
  label.className = 'text-blue-400'
  const display = allDisplayImages.value.find(item => String(item.id) === String(img.id) || item.url === img.src)
  label.textContent = `@${display?.displayLabel || mediaLabel(img.mediaType, allDisplayImages.value.filter(item => item.mediaType === (img.mediaType || 'image')).length)}`
  chip.append(thumbnail, label)
  chip.setAttribute('data-mention-id', String(img.id))
  chip.setAttribute('data-mention', img.name)
  const space = document.createTextNode(' ')
  range.insertNode(space); range.insertNode(chip)
  sel.removeAllRanges(); range.setStartAfter(space); range.collapse(true); sel.addRange(range)
  showMention.value = false
  mentionRange.value = null
  syncPrompt(mentionAnchor.value)
}

function promptForRequest(): string {
  const container = document.createElement('div')
  if (promptHtml.value) container.innerHTML = promptHtml.value
  else container.textContent = prompt.value
  const references = referencesForRequest()
  for (const mention of container.querySelectorAll<HTMLElement>('[data-mention]')) {
    const mentionId = mention.dataset.mentionId
    let index = mentionId ? references.findIndex(image => String(image.id) === mentionId) : -1
    if (index < 0) {
      const src = mention.querySelector('img')?.getAttribute('src')
      if (src) index = references.findIndex(image => image.url === src)
    }
    const replacement = index >= 0
      ? `[image ${index + 1}]（${references[index].label}）`
      : (mention.dataset.mention || mention.textContent || '参考图')
    mention.replaceWith(document.createTextNode(replacement))
  }
  const requestPrompt = (container.textContent || prompt.value).trim()
  if (references.length === 0) return requestPrompt
  return `${referenceInstructions(references)}\n\n${requestPrompt}`
}

function referenceRole(reference: DisplayImage): 'character' | 'scene' | 'other' {
  if (/人物|角色|肖像|character/i.test(reference.label)) return 'character'
  if (/场景|环境|空间|scene/i.test(reference.label)) return 'scene'
  return 'other'
}

// Character sheets go first because image-edit models treat the first input as
// the strongest visual anchor. The prompt numbering is derived from this same order.
function referencesForRequest(): DisplayImage[] {
  const priority = { character: 0, other: 1, scene: 2 }
  return allDisplayImages.value
    .map((reference, index) => ({ reference, index }))
    .sort((a, b) => priority[referenceRole(a.reference)] - priority[referenceRole(b.reference)] || a.index - b.index)
    .map(item => item.reference)
}

function referenceInstructions(references: DisplayImage[]): string {
  const lines = references.map((reference, index) => {
    const role = referenceRole(reference)
    if (role === 'character') {
      return `- image ${index + 1} 是人物身份硬参考（${reference.label}）：严格保持物种/人种、面部特征、发型、体型、服装与配色。`
    }
    if (role === 'scene') {
      return `- image ${index + 1} 是场景参考（${reference.label}）：用于环境、空间与光线，不得用它改变人物身份。`
    }
    return `- image ${index + 1} 是视觉参考（${reference.label}），按提示词中的用途使用。`
  })
  return [
    '【参考图强约束，优先级高于风格化发挥】',
    ...lines,
    '- 出现人物时必须复现人物身份参考；拟人动物必须保持原物种，禁止改画成真人。',
    '- 多镜头分镜中的同一人物必须在每格保持一致，不得重新设计角色。',
  ].join('\n')
}

async function generate() {
  if (!prompt.value || loading.value) return
  loading.value = true
  errorMessage.value = ''
  const modelConfig = getModelConfig(props.node?.node_type || 'image', selectedModel.value)
  try {
    const sizeBase = { '1K': 1024, '2K': 2048, '4K': 4096 }[selectedResolution.value] || 1024
    const ratioMap: Record<string, [number, number]> = { '1:1': [1,1], '4:3': [4,3], '3:4': [3,4], '3:2': [3,2], '2:3': [2,3], '16:9': [16,9], '9:16': [9,16], '5:4': [5,4], '4:5': [4,5], '21:9': [21,9] }
    const [rw, rh] = ratioMap[selectedRatio.value] || [1, 1]
    let w = Math.round(sizeBase * rw / rh)
    let h = sizeBase
    if (rw < rh) { w = sizeBase; h = Math.round(sizeBase * rh / rw) }
    // 确保最少像素（doubao 需要 ~3.7M）
    while (w * h < 3686400) { w = Math.round(w * 1.3); h = Math.round(h * 1.3) }
    const size = `${w}x${h}`
    const body: Record<string, any> = { model: selectedModel.value, prompt: promptForRequest(), n: selectedCount.value, size, aspect_ratio: selectedRatio.value, image_size: selectedResolution.value }
    if (modelConfig) {
      if (modelConfig.channel) body.channel = modelConfig.channel
      if (modelConfig.base_url) body.base_url = modelConfig.base_url
      if (modelConfig.api_key) body.api_key = modelConfig.api_key
      if (modelConfig.path) body.path = modelConfig.path
      if (modelConfig.protocol) body.protocol = modelConfig.protocol
    }
    // 传入连线参考图
    const refImages = referencesForRequest().filter(image => image.mediaType !== 'audio' && image.mediaType !== 'video').map(image => image.url)
    if (refImages.length > 16) throw new Error('参考图最多支持 16 张，请删除多余图片后重试')
    if (refImages.length > 0) {
      body.images = refImages
      // 兼容仍只接收单图字段的旧渠道。
      body.image = refImages[0]
      body.path = '/images/edits'
    }

    // 先清空旧图，保存到节点让画布显示空白/加载状态
    generatedImages.value = []
    emit('save', buildContent({ images: images.value, generated_images: [], _loading: true, _error: '' }))

    const res = await fetch('/api/images/generate', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }))
      throw new Error(typeof err.error === 'string' ? err.error : (err.error?.message || res.statusText))
    }
    const data = await res.json()
    if (data.data && Array.isArray(data.data)) {
      for (let i = 0; i < data.data.length; i++) {
        const item = data.data[i]
        const b64 = item.b64_json
        const url = item.url || (b64 ? (b64.startsWith('data:') ? b64 : `data:image/png;base64,${b64}`) : '')
        generatedImages.value.push({ id: Date.now() + i, url })
      }
    }
    emit('save', buildContent({ images: images.value, generated_images: generatedImages.value, _loading: false, _error: '' }))
  } catch (e: any) {
    errorMessage.value = e.message || '图片生成失败'
    emit('save', buildContent({ images: images.value, generated_images: generatedImages.value, _loading: false, _error: errorMessage.value }))
  } finally { loading.value = false }
}

function removeReference(img: { id: number | string; url: string }) {
  images.value = images.value.filter(image => image.id !== img.id)
  if (previewImg.value?.id === img.id) previewImg.value = null
  rebuildDisplay()
  emit('save', buildContent({ images: images.value, generated_images: generatedImages.value, _loading: false, _error: '' }))
}

function saveRecoveredImage(url: string) {
  const value = url.trim()
  if (!value || (!value.startsWith('http://') && !value.startsWith('https://') && !value.startsWith('data:image/'))) {
    errorMessage.value = '请输入有效的图片链接或选择图片文件'
    return
  }
  generatedImages.value = [{ id: Date.now(), url: value }]
  errorMessage.value = ''
  emit('save', buildContent({ images: images.value, generated_images: generatedImages.value, _loading: false, _error: '' }))
}

function recoverFromURL() {
  const url = window.prompt('粘贴中转站生成结果的图片 URL')
  if (url) saveRecoveredImage(url)
}

function recoverFromFile() {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*'
  input.onchange = () => {
    const file = input.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => saveRecoveredImage(String(reader.result || ''))
    reader.onerror = () => { errorMessage.value = '读取图片文件失败' }
    reader.readAsDataURL(file)
  }
  input.click()
}

watch(modalOpen, async (v) => {
  if (v) {
    await nextTick()
    if (modalContentEditable.value) {
      modalContentEditable.value.innerHTML = promptHtml.value || prompt.value
    }
  } else {
    if (editableRef.value && modalContentEditable.value) {
      editableRef.value.innerHTML = modalContentEditable.value.innerHTML
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

      <input v-model="displayName" type="text" placeholder="节点说明，如：高博人物形象" class="mb-2 w-full bg-transparent border-b border-white/10 px-1 py-1 text-xs text-white/70 outline-none placeholder:text-white/25 focus:border-white/30" />

      <div class="flex items-center gap-1.5 mb-2">
        <div class="shrink-0 w-10 h-10 rounded-lg border border-white/15 flex flex-col items-center justify-center text-white/40 gap-0.5">
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"><path d="M2 5V3a1 1 0 0 1 1-1h2M11 2h2a1 1 0 0 1 1 1v2M14 11v2a1 1 0 0 1-1 1h-2M5 14H3a1 1 0 0 1-1-1v-2"/></svg>
          <span class="text-[8px] leading-none">识图</span>
        </div>
        <div v-for="img in allDisplayImages" :key="img.id" class="relative shrink-0 w-10 h-10 rounded-lg overflow-hidden border border-white/15 cursor-pointer hover:border-white/50 transition-colors" @click.stop="previewImg = img">
          <span v-if="img.mediaType === 'audio'" class="w-full h-full flex items-center justify-center bg-cyan-500/15 text-cyan-300 text-lg">♫</span>
          <img v-else :src="img.url" class="w-full h-full object-cover" />
          <span class="absolute bottom-0 left-0 right-0 text-[7px] text-center bg-black/60 text-white/80 truncate px-0.5 leading-tight">{{ img.displayLabel }}</span>
          <button v-if="!img.isRef" type="button" class="absolute top-0 right-0 z-10 w-4 h-4 rounded-full bg-neutral-800 border border-white/20 text-white/60 hover:text-white flex items-center justify-center text-[10px] leading-none" title="删除参考图" @pointerdown.stop @click.stop="removeReference({ id: img.id, url: img.url })">×</button>
        </div>
        <button class="shrink-0 w-10 h-10 rounded-lg border border-white/15 flex flex-col items-center justify-center text-white/40 hover:text-white/60 hover:border-white/25 transition-colors gap-0.5" @click="onAddImage">
          <span class="text-sm leading-none">+</span>
          <span class="text-[8px] leading-none">参考图</span>
        </button>
        <button class="shrink-0 w-10 h-10 rounded-lg border border-white/15 flex flex-col items-center justify-center text-white/40 hover:text-white/60 hover:border-white/25 transition-colors gap-0.5" title="把已有图片直接放入当前节点" @click="recoverFromFile">
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 16l4-4 4 4 3-3 5 5"/><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/></svg>
          <span class="text-[8px] leading-none">结果图</span>
        </button>
        <button class="shrink-0 w-10 h-10 rounded-lg border border-white/15 flex flex-col items-center justify-center text-white/40 hover:text-white/60 hover:border-white/25 transition-colors gap-0.5" title="粘贴已有图片链接" @click="recoverFromURL">
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M10 13a5 5 0 0 0 7.1.1l2-2a5 5 0 0 0-7.1-7.1l-1.1 1.1"/><path d="M14 11a5 5 0 0 0-7.1-.1l-2 2A5 5 0 0 0 12 20l1.1-1.1"/></svg>
          <span class="text-[8px] leading-none">结果链接</span>
        </button>
      </div>

      <div v-if="showMention && mentionAnchor === 'inline'" class="relative">
        <MentionDropdown :connected-images="allDisplayImages.map(i => ({ id: i.id, name: i.label, displayName: i.displayLabel, dataUrl: i.url, mediaType: i.mediaType }))" :filter="mentionFilter" @insert="insertMention" />
      </div>

      <div v-if="errorMessage" class="mb-2 flex items-center gap-2 rounded-lg border border-red-400/30 bg-red-500/10 px-3 py-2 text-xs text-red-300">
        <span class="min-w-0 flex-1 break-words">{{ errorMessage }}</span>
        <button class="shrink-0 rounded border border-white/20 px-2 py-1 text-white/70 hover:bg-white/10" @click="recoverFromURL">粘贴链接</button>
        <button class="shrink-0 rounded border border-white/20 px-2 py-1 text-white/70 hover:bg-white/10" @click="recoverFromFile">导入图片</button>
      </div>

      <div ref="editableRef" contenteditable="true" class="prompt-editor w-full h-[200px] min-h-[200px] max-h-[200px] bg-transparent outline-0 text-white text-sm leading-5 px-1 overflow-y-auto whitespace-pre-wrap break-words empty:before:content-['输入图片提示词...'] empty:before:text-white/30" @input="onInput" />

      <div class="flex items-center justify-between gap-1 mt-3 pt-3 border-t border-white/10">
        <div class="flex items-center gap-0.5 min-w-0">
          <span class="text-[10px] text-white/30 ml-1">模型</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedModel" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-6 w-max min-w-[180px] shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="m in models" :key="m" :value="m" class="bg-neutral-900 text-white">{{ m }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <span class="text-[10px] text-white/30">比例</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedRatio" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-5 w-max min-w-[52px] shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="r in ratioOptions" :key="r" :value="r" class="bg-neutral-900 text-white">{{ r }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <span class="text-[10px] text-white/30">像素</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedResolution" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-5 w-max min-w-[44px] shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="r in resolutionOptions" :key="r" :value="r" class="bg-neutral-900 text-white">{{ r }}</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <span class="text-[10px] text-white/30">数量</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedCount" class="text-xs bg-transparent border-0 text-white/70 hover:text-white h-6 py-0 pl-0 pr-5 w-max min-w-[40px] shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option v-for="c in countOptions" :key="c" :value="c" class="bg-neutral-900 text-white">{{ c }}x</option>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <span class="text-[10px] text-white/30">预设</span>
          <div class="relative inline-flex items-center">
            <select v-model="selectedPreset" @change="applyPreset" class="text-xs bg-transparent border-0 text-white/50 hover:text-white h-6 py-0 pl-0 pr-6 w-max min-w-[56px] shrink-0 outline-none appearance-none cursor-pointer [color-scheme:dark]">
              <option value="" class="bg-neutral-900 text-white/40">预设</option>
              <optgroup v-for="cat in presetCategories" :key="cat" :label="cat">
                <option v-for="pr in presets.filter(p => p.category === cat)" :key="pr.id" :value="pr.id" class="bg-neutral-900 text-white">{{ pr.name }}</option>
              </optgroup>
            </select>
            <svg class="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-white/70" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
        </div>
        <button class="shrink-0 w-9 h-9 rounded-full bg-white flex items-center justify-center text-neutral-900 hover:bg-neutral-200 transition-colors disabled:opacity-50" :disabled="loading" @click="generate">
          <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>
          <span v-else class="loading loading-spinner loading-xs" />
        </button>
      </div>
    </div>
  </div>

  <Teleport to="body">
    <div v-if="modalOpen" class="fixed inset-0 z-[9998] flex items-center justify-center bg-black/70" @click.self="modalOpen = false">
      <div class="bg-neutral-900 border border-white/20 rounded-2xl w-[600px] max-h-[85vh] flex flex-col shadow-xl">
        <div class="flex items-center justify-between px-4 py-3 border-b border-white/10">
          <span class="text-sm font-medium text-white/70">图片生成</span>
          <button class="btn btn-xs btn-square bg-white/10 border border-white/30 text-white hover:bg-white/20" @click="modalOpen = false">
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
        <div class="flex-1 overflow-y-auto p-5 space-y-3">
          <div v-if="errorMessage" class="rounded-lg border border-red-400/30 bg-red-500/10 px-3 py-2 text-xs text-red-300 break-words">{{ errorMessage }}</div>
          <div class="relative">
            <div v-if="showMention && mentionAnchor === 'modal'" class="relative z-20">
              <MentionDropdown placement="inline" :connected-images="allDisplayImages.map(i => ({ id: i.id, name: i.label, displayName: i.displayLabel, dataUrl: i.url, mediaType: i.mediaType }))" :filter="mentionFilter" @insert="insertMention" />
            </div>
            <div ref="modalContentEditable" contenteditable="true" class="prompt-editor w-full h-[200px] min-h-[200px] max-h-[200px] bg-transparent outline-0 text-white text-sm leading-5 overflow-y-auto whitespace-pre-wrap break-words empty:before:content-['输入图片提示词...'] empty:before:text-white/30" @input="onModalInput" />
          </div>
          <div v-if="allDisplayImages.length > 0" class="flex flex-wrap gap-2">
            <div v-for="img in allDisplayImages" :key="img.id" class="relative w-16 h-16 rounded-lg overflow-hidden border border-white/15 cursor-pointer" @click="previewImg = img">
              <img :src="img.url" class="w-full h-full object-cover" />
              <span class="absolute bottom-0 left-0 right-0 text-[7px] text-center bg-black/60 text-white/80 truncate px-0.5">{{ img.displayLabel }}</span>
            </div>
          </div>
        </div>
        <div class="flex items-center justify-between gap-2 px-4 py-3 border-t border-white/10">
          <div class="flex items-center gap-1">
            <span class="text-[10px] text-white/30">模型</span>
            <select v-model="selectedModel" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/80 pl-1.5 pr-4 py-1.5"><option v-for="m in models" :key="m" :value="m" class="bg-neutral-800">{{ m }}</option></select>
            <span class="text-[10px] text-white/30">比例</span>
            <select v-model="selectedRatio" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/80 pl-1.5 pr-3 py-1.5"><option v-for="r in ratioOptions" :key="r" :value="r" class="bg-neutral-800">{{ r }}</option></select>
            <span class="text-[10px] text-white/30">像素</span>
            <select v-model="selectedResolution" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/80 pl-1.5 pr-3 py-1.5"><option v-for="r in resolutionOptions" :key="r" :value="r" class="bg-neutral-800">{{ r }}</option></select>
            <span class="text-[10px] text-white/30">数量</span>
            <select v-model="selectedCount" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/80 pl-1.5 pr-3 py-1.5"><option v-for="c in countOptions" :key="c" :value="c" class="bg-neutral-800">{{ c }}x</option></select>
            <span class="text-[10px] text-white/30">预设</span>
            <select v-model="selectedPreset" @change="applyPreset" class="appearance-none bg-white/5 border border-white/10 rounded text-xs text-white/50 pl-1.5 pr-4 py-1.5"><option value="" class="bg-neutral-800 text-white/40">预设</option><optgroup v-for="cat in presetCategories" :key="cat" :label="cat"><option v-for="pr in presets.filter(p => p.category === cat)" :key="pr.id" :value="pr.id" class="bg-neutral-800 text-white">{{ pr.name }}</option></optgroup></select>
          </div>
          <button class="shrink-0 w-9 h-9 rounded-full bg-white flex items-center justify-center text-neutral-900 hover:bg-neutral-200 disabled:opacity-50" :disabled="loading" @click="generate">
            <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>
            <span v-else class="loading loading-spinner loading-xs" />
          </button>
        </div>
      </div>
    </div>

    <div v-if="previewImg" class="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80" @click="previewImg = null">
      <img :src="previewImg.url" class="max-w-[90vw] max-h-[90vh] object-contain rounded-lg" @click.stop />
      <button class="absolute top-4 right-4 btn btn-sm btn-circle bg-white/10 border border-white/30 text-white hover:bg-white/20" @click="previewImg = null">
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
      </button>
    </div>
  </Teleport>
</template>
