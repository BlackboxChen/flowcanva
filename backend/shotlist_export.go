package main

import (
	"fmt"
	"strings"
)

// renderShotlistMarkdown 按 reference/DELIVERY_FORMAT.md 的 11 章结构渲染交付文件。
// P0 不使用故事板分支，因此跳过第 9 章。
func renderShotlistMarkdown(content *ShotlistContent) string {
	var sb strings.Builder

	title := strings.TrimSpace(content.Project.Title)
	if title == "" {
		title = "未命名项目"
	}
	platform := strings.TrimSpace(content.Project.TargetPlatform)
	if platform == "" {
		platform = "Seedance 2.5"
	}

	fmt.Fprintf(&sb, "# %s｜%s 分镜与视频提示词\n", title, platform)

	sb.WriteString("\n## 1. 项目与范围摘要\n\n")
	fmt.Fprintf(&sb, "- 目标平台：%s\n", platform)
	fmt.Fprintf(&sb, "- 目标时长：%d 秒\n", content.Project.TargetDurationS)
	fmt.Fprintf(&sb, "- 工作段数量：%d\n", len(content.Segments))
	fmt.Fprintf(&sb, "- 当前状态：%s\n", fallback(content.State, "未开始"))
	if content.Notes != "" {
		fmt.Fprintf(&sb, "- 备注：%s\n", content.Notes)
	}

	sb.WriteString("\n## 2. 剧本锁定清单与用户改动授权\n\n")
	renderScriptLists(&sb, content)

	sb.WriteString("\n## 3. 资产需求判断、用户选择与资产映射\n\n")
	renderAssets(&sb, content.Assets)

	sb.WriteString("\n## 4. 故事板方式\n\n")
	renderStoryboardMode(&sb, content.Segments)

	sb.WriteString("\n## 5. 用户目标时长、忠实估算与工作段方案\n\n")
	renderDuration(&sb, content)

	sb.WriteString("\n## 6. 全剧本 EV/DLG 覆盖表\n\n")
	renderCoverage(&sb, content)

	sb.WriteString("\n## 7. 空间调度\n\n")
	renderSpatial(&sb, content.Spatial)

	sb.WriteString("\n## 8. 各工作段文字分镜表\n\n")
	renderShotTables(&sb, content.Segments)

	sb.WriteString("\n## 10. Seedance 2.5 视频提示词\n\n")
	renderPrompts(&sb, content.Segments)

	sb.WriteString("\n## 11. 跨工作段连续性与生成风险\n\n")
	renderLedger(&sb, content)

	return sb.String()
}

func renderScriptLists(sb *strings.Builder, content *ShotlistContent) {
	fmt.Fprintf(sb, "- 剧本版本：%s\n", fallback(content.Script.Version, "v1"))
	if content.Script.SourceNodeID != "" {
		fmt.Fprintf(sb, "- 剧本来源节点：`%s`\n", content.Script.SourceNodeID)
	}
	sb.WriteString("\n### 事件清单（EV）\n\n")
	renderBulletList(sb, len(content.Script.EV), func(i int) (string, string) {
		return content.Script.EV[i].ID, content.Script.EV[i].Text
	})
	sb.WriteString("\n### 台词清单（DLG）\n\n")
	renderBulletList(sb, len(content.Script.DLG), func(i int) (string, string) {
		d := content.Script.DLG[i]
		return d.ID, fmt.Sprintf("%s：%s", d.Speaker, d.Text)
	})
	sb.WriteString("\n### 锁定事实（LOCK）\n\n")
	renderBulletList(sb, len(content.Script.LOCK), func(i int) (string, string) {
		return content.Script.LOCK[i].ID, content.Script.LOCK[i].Fact
	})
}

func renderAssets(sb *strings.Builder, assets []ShotlistAsset) {
	if len(assets) == 0 {
		sb.WriteString("用户选择不使用任何资产图。后续提示词完全省略素材句柄与占位。\n")
		return
	}
	sb.WriteString("| 句柄 | 节点 | 用途 | 参考 | 不参考 |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for _, a := range assets {
		fmt.Fprintf(sb, "| %s | `%s` | %s | %s | %s |\n",
			a.Handle, a.NodeID, fallback(a.Role, "-"), fallback(a.Use, "-"), fallback(a.NotUse, "-"))
	}
}

func renderStoryboardMode(sb *strings.Builder, segments []ShotlistSegment) {
	for _, seg := range segments {
		mode := "不使用故事板"
		if seg.Storyboard != nil && seg.Storyboard.Mode == "use" {
			mode = fmt.Sprintf("使用故事板（%d 格，审查：%s）", seg.Storyboard.N, fallback(seg.Storyboard.Review, "待审"))
		}
		fmt.Fprintf(sb, "- %s：%s\n", seg.ID, mode)
	}
}

func renderDuration(sb *strings.Builder, content *ShotlistContent) {
	d := content.Duration
	if d.TargetS == 0 && d.FaithfulEstimateS == 0 {
		sb.WriteString("尚未完成时长判断。\n")
		return
	}
	fmt.Fprintf(sb, "- 目标时长：%d 秒\n", d.TargetS)
	fmt.Fprintf(sb, "- 忠实估算：%d 秒\n", d.FaithfulEstimateS)
	fmt.Fprintf(sb, "- 结论：%s\n", fallback(d.Verdict, "-"))
	if d.Note != "" {
		fmt.Fprintf(sb, "- 说明：%s\n", d.Note)
	}
	sb.WriteString("\n| 工作段 | 标题 | 时长 | 拍摄形式 |\n|---|---|---|---|\n")
	for _, seg := range content.Segments {
		fmt.Fprintf(sb, "| %s | %s | %d 秒 | %s |\n",
			seg.ID, seg.Title, seg.DurationS, formLabel(seg.Form))
	}
}

func renderCoverage(sb *strings.Builder, content *ShotlistContent) {
	sb.WriteString("| 工作段 | 覆盖 EV | 覆盖 DLG |\n|---|---|---|\n")
	for _, seg := range content.Segments {
		fmt.Fprintf(sb, "| %s | %s | %s |\n",
			seg.ID, joinOrDash(seg.Coverage.EV), joinOrDash(seg.Coverage.DLG))
	}
}

func renderSpatial(sb *strings.Builder, s ShotlistSpatial) {
	if !s.Needed {
		fmt.Fprintf(sb, "本段无需俯视调度图。理由：%s\n", fallback(s.Reason, "未填写"))
		return
	}
	if s.Blocking != "" {
		fmt.Fprintf(sb, "**俯视调度**\n\n%s\n", s.Blocking)
	}
	if s.Axis != "" {
		fmt.Fprintf(sb, "\n**运动轴线与摄影机方位**\n\n%s\n", s.Axis)
	}
}

func renderShotTables(sb *strings.Builder, segments []ShotlistSegment) {
	for _, seg := range segments {
		fmt.Fprintf(sb, "### %s｜%s（%d 秒，%s）\n\n", seg.ID, seg.Title, seg.DurationS, formLabel(seg.Form))
		sb.WriteString("| 镜号 | 时间 | 画面动作概述 | 画面构图 | 机位 | 动作/表演 | 对白 | 声音 | 转场 |\n")
		sb.WriteString("|---|---|---|---|---|---|---|---|---|\n")
		for _, shot := range seg.Shots {
			fmt.Fprintf(sb, "| %d | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				shot.Idx, esc(shot.T), esc(shot.Action), esc(shot.Composition), esc(shot.Camera),
				esc(shot.Performance), esc(shot.Dialogue), esc(shot.Sound), esc(shot.Transition))
		}
		sb.WriteString("\n")
	}
}

func renderPrompts(sb *strings.Builder, segments []ShotlistSegment) {
	for _, seg := range segments {
		fmt.Fprintf(sb, "### %s｜%s（%d 秒，%s）\n\n", seg.ID, seg.Title, seg.DurationS, formLabel(seg.Form))
		if strings.TrimSpace(seg.Prompt) == "" {
			sb.WriteString("_该工作段尚未生成提示词。_\n\n")
			continue
		}
		sb.WriteString("```text\n")
		sb.WriteString(strings.TrimSpace(seg.Prompt))
		sb.WriteString("\n```\n\n")
	}
}

func renderLedger(sb *strings.Builder, content *ShotlistContent) {
	if len(content.Ledger) == 0 {
		sb.WriteString("无跨工作段连续性记录。\n")
		return
	}
	sb.WriteString("| 从 | 到 | 需继承 |\n|---|---|---|\n")
	for _, l := range content.Ledger {
		fmt.Fprintf(sb, "| %s | %s | %s |\n", l.From, l.To, joinOrDash(l.Inherit))
	}
}

// ─── 渲染辅助 ──────────────────────────────────────────────────────

func renderBulletList(sb *strings.Builder, n int, at func(int) (string, string)) {
	if n == 0 {
		sb.WriteString("_（空）_\n")
		return
	}
	for i := 0; i < n; i++ {
		id, text := at(i)
		fmt.Fprintf(sb, "- **%s** %s\n", id, text)
	}
}

func formLabel(form string) string {
	if form == "one_take" {
		return "单镜头一镜到底"
	}
	if form == "multi_shot" {
		return "多镜头剪辑"
	}
	return fallback(form, "-")
}

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}

func joinOrDash(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, "、")
}

// esc 转义 Markdown 表格里的竖线与换行，避免破坏表格结构。
func esc(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", "<br>")
	return strings.ReplaceAll(s, "\n", "<br>")
}
