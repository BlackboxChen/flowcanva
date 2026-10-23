// Finite sampling requires each visible segment to have 5 + 17*n frames.
export function normalizeH3Frames(graph: Record<string, any>) {
  const finite = Object.values(graph).filter(n => n.class_type === 'MiniMaxH3FiniteSegmentVideoSampler')
  let changed = false
  for (const sampler of finite) {
    const planner = graph[sampler.inputs?.finite_plan?.[0]]
    if (planner?.class_type !== 'MiniMaxH3TimelinePlanner') continue
    const timeline = JSON.parse(planner.inputs.timeline_data)
    const segments = timeline.segmentConfig?.segments
    if (!segments?.length) continue
    // Preserve deliberately overlapping plans; these require separate overlap rules.
    let previous = 0
    const lengths = segments.map((s: any) => {
      if (!Number.isInteger(s.startFrame) || !Number.isInteger(s.endFrame) || s.startFrame !== previous) throw new Error('H3 分段须为连续整数帧；重叠时间线请单独校验')
      previous = s.endFrame
      const length = s.endFrame - s.startFrame
      if (length < 5 || length > 3592) throw new Error('H3 分段帧数超出范围')
      return length
    })
    if (lengths.every((n: number) => (n - 5) % 17 === 0)) continue
    let cursor = 0
    segments.forEach((s: any, i: number) => {
      const length = 5 + 17 * Math.ceil((lengths[i] - 5) / 17)
      if (length > 3592) throw new Error('H3 补齐后分段超过上限')
      s.startFrame = cursor; cursor += length; s.endFrame = cursor
    })
    timeline.selection = { start: 0, duration: cursor / 24 }
    planner.inputs.generation_seconds = cursor / 24
    planner.inputs.timeline_data = JSON.stringify(timeline)
    changed = true
  }
  return changed
}
