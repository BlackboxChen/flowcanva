export interface WorkflowOutput {
  name: string
  value: string
  type: 'image' | 'video' | 'audio' | 'text'
  sourceURL?: string
}

const pending = new Map<string, Promise<string>>()
const cacheKey = 'flowcanva:workflow-local-media:v1'

function readCache(): Record<string, string> {
  try { return JSON.parse(localStorage.getItem(cacheKey) || '{}') } catch { return {} }
}

async function persistRemoteMedia(remote: string, kind: string): Promise<string> {
  const cached = readCache()[remote]
  if (cached?.startsWith('/uploads/')) {
    try { if ((await fetch(cached, { method: 'HEAD' })).ok) return cached } catch { /* download again */ }
  }
  // Use the existing backend proxy so saving works without ComfyUI browser CORS.
  const response = await fetch(`/api/comfyui/proxy-view?url=${encodeURIComponent(remote)}`)
  if (!response.ok) throw new Error(`下载生成素材失败：HTTP ${response.status}`)
  const blob = await response.blob()
  if (!blob.size || blob.size > 1024 ** 3 || !/^(image|video|audio)\//i.test(blob.type)) {
    throw new Error('生成素材为空、超过1GB或响应不是媒体文件，未保存远程引用')
  }
  const parsed = new URL(remote)
  const filename = (parsed.searchParams.get('filename') || parsed.pathname.split('/').pop() || '').split(/[\\/]/).pop()
  const fallback = kind === 'video' ? 'output.mp4' : kind === 'audio' ? 'output.wav' : 'output.png'
  const body = new FormData()
  body.append('file', blob, filename?.includes('.') ? filename : fallback)
  const uploaded = await fetch('/api/admin/assets/upload', { method: 'POST', body })
  if (!uploaded.ok) throw new Error(`素材已生成，但保存到本地失败：HTTP ${uploaded.status}`)
  const asset = await uploaded.json()
  if (!asset.url?.startsWith('/uploads/')) throw new Error('本地素材接口未返回有效文件地址')
  try { localStorage.setItem(cacheKey, JSON.stringify({ ...readCache(), [remote]: asset.url })) } catch { /* cache is optional; file is already persisted */ }
  return asset.url
}

/** Publish media only after a durable local copy exists. Keep its origin for tracing. */
export async function persistWorkflowMedia(outputs: WorkflowOutput[]): Promise<WorkflowOutput[]> {
  const saved: WorkflowOutput[] = []
  for (const output of outputs) {
    if (output.type === 'text' || !/^https?:\/\//i.test(output.value)) { saved.push(output); continue }
    let task = pending.get(output.value)
    if (!task) {
      task = persistRemoteMedia(output.value, output.type)
      pending.set(output.value, task)
    }
    try { saved.push({ ...output, value: await task, sourceURL: output.sourceURL || output.value }) }
    finally { pending.delete(output.value) }
  }
  return saved
}
