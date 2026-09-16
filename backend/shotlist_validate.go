package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	shotlistMinSegmentSeconds = 4
	shotlistMaxSegmentSeconds = 30
)

// requiredPromptSections 是 PROMPT_PATTERNS.md 规定必须出现的段落标题。
var requiredPromptSections = []string{
	"【全局声音与字幕约束】",
	"【一句话概述】",
	"【全局视觉设定】",
	"【详细情节】",
	"【全局连续性与结尾约束】",
}

// cameraWordsInStyle 是【全局视觉设定】里不应出现的具体运镜词。
var cameraWordsInStyle = []string{"推镜", "拉镜", "摇镜", "移镜", "跟拍", "变焦", "硬切", "跳切"}

// validateShotlist 是 scripts/validate_delivery.py 的 Go 移植。
// 只做结构校验，不能替代返图审查和剧情语义判断。
func validateShotlist(content *ShotlistContent) ShotlistValidation {
	v := ShotlistValidation{Errors: []ShotlistIssue{}, Warnings: []ShotlistIssue{}}

	if len(content.Script.EV) == 0 && len(content.Script.DLG) == 0 {
		addIssue(&v, "ERROR", "SCRIPT_EMPTY", "全局", "剧本清单为空，请先完成「锁定剧本」")
	}
	if len(content.Segments) == 0 {
		addIssue(&v, "ERROR", "NO_SEGMENT", "全局", "没有任何工作段，请先完成「工作段规划」")
		return v
	}

	evIDs := map[string]bool{}
	for _, ev := range content.Script.EV {
		evIDs[ev.ID] = true
	}
	dlgByID := map[string]ShotlistDLG{}
	for _, dlg := range content.Script.DLG {
		dlgByID[dlg.ID] = dlg
	}

	coveredEV := map[string]bool{}
	coveredDLG := map[string]bool{}

	for i := range content.Segments {
		seg := &content.Segments[i]
		where := seg.ID
		if where == "" {
			where = fmt.Sprintf("第 %d 段", i+1)
		}
		validateSegment(&v, seg, where, evIDs, dlgByID, coveredEV, coveredDLG)
	}

	for _, ev := range content.Script.EV {
		if !coveredEV[ev.ID] {
			addIssue(&v, "ERROR", "EV_UNCOVERED", "全局",
				fmt.Sprintf("%s（%s）没有被任何工作段的 coverage 覆盖", ev.ID, truncate(ev.Text, 24)))
		}
	}
	for _, dlg := range content.Script.DLG {
		if !coveredDLG[dlg.ID] {
			addIssue(&v, "ERROR", "DLG_UNCOVERED", "全局",
				fmt.Sprintf("%s（%s）没有被任何工作段的 coverage 覆盖", dlg.ID, truncate(dlg.Text, 24)))
		}
	}

	validateAssets(&v, content.Assets)
	return v
}

func validateSegment(v *ShotlistValidation, seg *ShotlistSegment, where string,
	evIDs map[string]bool, dlgByID map[string]ShotlistDLG,
	coveredEV, coveredDLG map[string]bool) {

	if seg.DurationS < shotlistMinSegmentSeconds || seg.DurationS > shotlistMaxSegmentSeconds {
		addIssue(v, "ERROR", "DURATION_RANGE", where,
			fmt.Sprintf("单元时长 %d 秒，必须落在 %d—%d 秒之间",
				seg.DurationS, shotlistMinSegmentSeconds, shotlistMaxSegmentSeconds))
	}

	if seg.Form != "multi_shot" && seg.Form != "one_take" {
		addIssue(v, "ERROR", "FORM_INVALID", where,
			fmt.Sprintf("拍摄形式 %q 非法，只能是 multi_shot 或 one_take", seg.Form))
	}

	for _, id := range seg.Coverage.EV {
		if !evIDs[id] {
			addIssue(v, "ERROR", "COVERAGE_EV_UNKNOWN", where, fmt.Sprintf("coverage 引用了不存在的 %s", id))
			continue
		}
		coveredEV[id] = true
	}
	for _, id := range seg.Coverage.DLG {
		if _, ok := dlgByID[id]; !ok {
			addIssue(v, "ERROR", "COVERAGE_DLG_UNKNOWN", where, fmt.Sprintf("coverage 引用了不存在的 %s", id))
			continue
		}
		coveredDLG[id] = true
	}

	if len(seg.Shots) == 0 {
		addIssue(v, "ERROR", "NO_SHOT", where, "该工作段没有任何镜头")
	} else {
		validateTimeline(v, seg, where)
	}

	if strings.TrimSpace(seg.Prompt) == "" {
		addIssue(v, "ERROR", "NO_PROMPT", where, "该工作段还没有视频提示词")
	} else {
		validatePrompt(v, seg, where)
	}
}

// validateTimeline 检查镜头时间戳是否整数秒、连续、完整覆盖本段时长。
func validateTimeline(v *ShotlistValidation, seg *ShotlistSegment, where string) {
	type span struct{ start, end int }
	spans := make([]span, 0, len(seg.Shots))

	for _, shot := range seg.Shots {
		start, end, ok := parseTimeRange(shot.T)
		if !ok {
			addIssue(v, "ERROR", "TIMESTAMP_FORMAT", where,
				fmt.Sprintf("镜头 %d 的时间戳 %q 无法解析，应形如「0—4秒」", shot.Idx, shot.T))
			continue
		}
		if end <= start {
			addIssue(v, "ERROR", "TIMESTAMP_ORDER", where,
				fmt.Sprintf("镜头 %d 的结束时间不大于开始时间：%s", shot.Idx, shot.T))
			continue
		}
		spans = append(spans, span{start, end})

		if strings.TrimSpace(shot.Camera) == "" {
			addIssue(v, "ERROR", "SHOT_CAMERA_EMPTY", where, fmt.Sprintf("镜头 %d 缺少机位描述", shot.Idx))
		}
		if strings.TrimSpace(shot.Transition) == "" && seg.Form == "multi_shot" {
			addIssue(v, "WARNING", "SHOT_TRANSITION_EMPTY", where,
				fmt.Sprintf("镜头 %d 没有写转场，跨镜动势无法继承", shot.Idx))
		}
		if len([]rune(shot.Camera)) < 12 {
			addIssue(v, "WARNING", "SHOT_CAMERA_THIN", where,
				fmt.Sprintf("镜头 %d 的机位过于简略，应写清起始状态、跟随对象、触发变化和结束动势", shot.Idx))
		}
	}

	if len(spans) == 0 {
		return
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	if spans[0].start != 0 {
		addIssue(v, "ERROR", "TIMELINE_START", where,
			fmt.Sprintf("时间轴从 %d 秒开始，必须从 0 秒开始", spans[0].start))
	}
	for i := 1; i < len(spans); i++ {
		if spans[i].start != spans[i-1].end {
			addIssue(v, "ERROR", "TIMELINE_GAP", where,
				fmt.Sprintf("时间轴在 %d 秒处不连续（上一段结束于 %d 秒）", spans[i].start, spans[i-1].end))
		}
	}
	if last := spans[len(spans)-1].end; last != seg.DurationS {
		addIssue(v, "ERROR", "TIMELINE_END", where,
			fmt.Sprintf("时间轴结束于 %d 秒，与本段时长 %d 秒不一致", last, seg.DurationS))
	}
}

// validatePrompt 检查提示词结构与剧本保真。
func validatePrompt(v *ShotlistValidation, seg *ShotlistSegment, where string) {
	prompt := seg.Prompt

	if !strings.HasPrefix(strings.TrimSpace(prompt), requiredPromptSections[0]) {
		addIssue(v, "ERROR", "PROMPT_HEADER", where,
			"提示词必须以【全局声音与字幕约束】开头")
	}
	for _, section := range requiredPromptSections {
		if !strings.Contains(prompt, section) {
			addIssue(v, "ERROR", "PROMPT_SECTION_MISSING", where, "提示词缺少段落 "+section)
		}
	}

	usesStoryboard := seg.Storyboard != nil && seg.Storyboard.Mode == "use"
	if !usesStoryboard && (strings.Contains(prompt, "SB-") || strings.Contains(prompt, "故事板")) {
		addIssue(v, "ERROR", "PROMPT_STORYBOARD_LEAK", where,
			"该段标记为不使用故事板，提示词中不得出现故事板字段或 SB- 引用")
	}
	if usesStoryboard && (seg.Storyboard.NodeID == "" || seg.Storyboard.Review != "passed") {
		addIssue(v, "ERROR", "STORYBOARD_NOT_CONFIRMED", where,
			"该段使用故事板，但返图尚未落地或未通过审查")
	}

	if style := extractSection(prompt, "【全局视觉设定】"); style != "" {
		for _, word := range cameraWordsInStyle {
			if strings.Contains(style, word) {
				addIssue(v, "WARNING", "STYLE_HAS_CAMERA_MOVE", where,
					fmt.Sprintf("【全局视觉设定】里出现了运镜词「%s」，该段不应混入具体镜头动作", word))
			}
		}
	}
}

func validateAssets(v *ShotlistValidation, assets []ShotlistAsset) {
	for i, a := range assets {
		handle := a.Handle
		if handle == "" {
			handle = fmt.Sprintf("第 %d 项素材", i+1)
		}
		if strings.TrimSpace(a.Use) == "" {
			addIssue(v, "WARNING", "ASSET_USE_EMPTY", "全局", handle+" 没有写「参考什么」")
		}
		if strings.TrimSpace(a.NotUse) == "" {
			addIssue(v, "WARNING", "ASSET_NOTUSE_EMPTY", "全局", handle+" 没有写「不参考什么」")
		}
	}
}

// parseTimeRange 解析「0—4秒」「5-9秒｜镜头2」这类时间戳，返回起止整数秒。
func parseTimeRange(raw string) (int, int, bool) {
	s := strings.TrimSpace(raw)
	if idx := strings.IndexAny(s, "｜|"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "秒"))

	separator := ""
	for _, cand := range []string{"—", "–", "~", "-"} {
		if strings.Contains(s, cand) {
			separator = cand
			break
		}
	}
	if separator == "" {
		return 0, 0, false
	}

	parts := strings.SplitN(s, separator, 2)
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return start, end, true
}

// extractSection 取出以 title 开头、到下一个【段落】为止的正文。
func extractSection(text, title string) string {
	idx := strings.Index(text, title)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(title):]
	if next := strings.Index(rest, "【"); next >= 0 {
		return rest[:next]
	}
	return rest
}

func addIssue(v *ShotlistValidation, level, code, where, message string) {
	issue := ShotlistIssue{Level: level, Code: code, Where: where, Message: message}
	if level == "ERROR" {
		v.Errors = append(v.Errors, issue)
		return
	}
	v.Warnings = append(v.Warnings, issue)
}

func truncate(s string, limit int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
