export interface ImagePreset {
  id: string
  name: string
  prompt: string
  category: string
  scope: string
  preset_type?: string
}

const requests = new Map<string, Promise<ImagePreset[]>>()

// All image panels on one canvas share this request instead of loading the
// same global and canvas presets once per node.
export function loadImagePresets(canvasId: string): Promise<ImagePreset[]> {
  const cached = requests.get(canvasId)
  if (cached) return cached

  const request = Promise.all([
    fetch('/api/admin/presets?scope=global').then(response => response.json()),
    fetch(`/api/admin/presets?scope=canvas&canvas_id=${encodeURIComponent(canvasId)}`).then(response => response.json()),
  ]).then(([globalPresets, canvasPresets]) => {
    const all = [
      ...(Array.isArray(canvasPresets) ? canvasPresets : []),
      ...(Array.isArray(globalPresets) ? globalPresets : []),
    ]
    return all.filter((preset: ImagePreset) => (preset.preset_type || 'image') === 'image')
  }).catch(error => {
    requests.delete(canvasId)
    throw error
  })

  requests.set(canvasId, request)
  return request
}
