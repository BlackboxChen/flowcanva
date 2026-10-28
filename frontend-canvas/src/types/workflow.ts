export interface WorkflowOutput {
  name: string
  value: string
  type: 'image' | 'video' | 'audio' | 'text'
  sourceURL?: string
}
