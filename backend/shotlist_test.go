package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseTimeRange(t *testing.T) {
	cases := []struct {
		in         string
		start, end int
		ok         bool
	}{
		{"0—4秒", 0, 4, true},
		{"4—9秒", 4, 9, true},
		{"9—14秒", 9, 14, true},
		{"5-9秒｜镜头2", 5, 9, true},
		{"0~3秒", 0, 3, true},
		{" 12—18秒 ", 12, 18, true},
		{"全程单镜头", 0, 0, false},
		{"", 0, 0, false},
		{"abc—def秒", 0, 0, false},
	}
	for _, c := range cases {
		start, end, ok := parseTimeRange(c.in)
		if ok != c.ok || (ok && (start != c.start || end != c.end)) {
			t.Errorf("parseTimeRange(%q) = (%d, %d, %v)，期望 (%d, %d, %v)",
				c.in, start, end, ok, c.start, c.end, c.ok)
		}
	}
}

func TestExtractJSONObject(t *testing.T) {
	plain := `{"a":1}`
	if got, ok := extractJSONObject(plain); !ok || got != plain {
		t.Errorf("纯 JSON 解析失败: %q %v", got, ok)
	}

	fenced := "```json\n{\"a\":1}\n```"
	if got, ok := extractJSONObject(fenced); !ok || got != plain {
		t.Errorf("围栏 JSON 解析失败: %q %v", got, ok)
	}

	prose := "好的，结果如下：\n{\"a\":1}\n希望有帮助。"
	if got, ok := extractJSONObject(prose); !ok || got != plain {
		t.Errorf("夹带说明文字的 JSON 解析失败: %q %v", got, ok)
	}

	if _, ok := extractJSONObject("完全没有 JSON"); ok {
		t.Error("无 JSON 的内容不应解析成功")
	}
}

func TestValidateShotlistCatchesTimelineBreak(t *testing.T) {
	content := &ShotlistContent{
		Script: ShotlistScript{
			EV:  []ShotlistEV{{ID: "EV-01", Text: "推门进入"}},
			DLG: []ShotlistDLG{{ID: "DLG-01", Speaker: "Roko", Text: "你为什么一直瞒着我？"}},
		},
		Segments: []ShotlistSegment{{
			ID: "SEG-01", Title: "发现照片", DurationS: 15, Form: "multi_shot",
			Coverage: ShotlistCoverage{EV: []string{"EV-01"}, DLG: []string{"DLG-01"}},
			Shots: []ShotlistShot{
				{Idx: 1, T: "0—4秒", Camera: "门内南侧 35mm 广角低机位，轻微手持后移", Transition: "硬切"},
				// 故意留一个时间缺口：4—9 之后直接跳到 10—14
				{Idx: 2, T: "10—14秒", Camera: "85mm 近摄，先锁定照片再做一次受控焦点转移", Transition: "停"},
			},
			Prompt: "【全局声音与字幕约束】\n不要BGM、不要字幕，只保留音效。\n" +
				"【一句话概述】\n发现照片。\n【全局视觉设定】\n写实。\n" +
				"【详细情节】\n0—4秒 推门。\n【全局连续性与结尾约束】\n保持连续。",
		}},
	}

	result := validateShotlist(content)
	if len(result.Errors) == 0 {
		t.Fatal("时间轴有缺口，应当报错")
	}
	if !hasIssue(result.Errors, "TIMELINE_GAP") {
		t.Errorf("应当命中 TIMELINE_GAP，实际错误：%+v", result.Errors)
	}
	if !hasIssue(result.Errors, "TIMELINE_END") {
		t.Errorf("结束时间与时长不一致，应当命中 TIMELINE_END，实际错误：%+v", result.Errors)
	}
}

func TestValidateShotlistCatchesMissingDialogue(t *testing.T) {
	content := &ShotlistContent{
		Script: ShotlistScript{
			EV:  []ShotlistEV{{ID: "EV-01", Text: "质问"}},
			DLG: []ShotlistDLG{{ID: "DLG-01", Speaker: "女主", Text: "你为什么一直瞒着我？"}},
		},
		Segments: []ShotlistSegment{{
			ID: "SEG-01", Title: "质问", DurationS: 4, Form: "one_take",
			// coverage 故意不写 DLG
			Coverage: ShotlistCoverage{EV: []string{"EV-01"}},
			Shots: []ShotlistShot{
				{Idx: 1, T: "0—4秒", Camera: "35mm 中景跟随，沿人物右侧小弧线绕行到近景"},
			},
			Prompt: "【全局声音与字幕约束】\n不要BGM、不要字幕，只保留音效。\n" +
				"【一句话概述】\n质问。\n【全局视觉设定】\n写实。\n" +
				"【详细情节】\n0—4秒 她开口。\n【全局连续性与结尾约束】\n保持连续。",
		}},
	}

	result := validateShotlist(content)
	if !hasIssue(result.Errors, "DLG_UNCOVERED") {
		t.Errorf("台词未被覆盖，应当命中 DLG_UNCOVERED，实际错误：%+v", result.Errors)
	}
}

func TestValidateShotlistCatchesStoryboardLeak(t *testing.T) {
	content := &ShotlistContent{
		Script: ShotlistScript{EV: []ShotlistEV{{ID: "EV-01", Text: "事件"}}},
		Segments: []ShotlistSegment{{
			ID: "SEG-01", DurationS: 5, Form: "multi_shot",
			Coverage: ShotlistCoverage{EV: []string{"EV-01"}},
			Shots: []ShotlistShot{
				{Idx: 1, T: "0—5秒", Camera: "35mm 中景跟随，沿人物右侧小弧线绕行到近景"},
			},
			// 未标记使用故事板，提示词里却引用了 SB-01
			Prompt: "【全局声音与字幕约束】\n不要BGM。\n【一句话概述】\n事件。\n" +
				"【全局视觉设定】\n写实。\n【详细情节】\n故事板：参考SB-01的构图。\n" +
				"【全局连续性与结尾约束】\n连续。",
		}},
	}

	result := validateShotlist(content)
	if !hasIssue(result.Errors, "PROMPT_STORYBOARD_LEAK") {
		t.Errorf("不使用故事板却引用 SB-，应当命中 PROMPT_STORYBOARD_LEAK，实际错误：%+v", result.Errors)
	}
}

func TestValidateShotlistPassesOnCleanContent(t *testing.T) {
	content := &ShotlistContent{
		State:   "PROMPTS_VALIDATED",
		Script: ShotlistScript{
			Version: "v1",
			EV:      []ShotlistEV{{ID: "EV-01", Text: "推门进入旧公寓"}},
			DLG:     []ShotlistDLG{{ID: "DLG-01", Speaker: "Roko", Text: "你为什么一直瞒着我？"}},
		},
		Segments: []ShotlistSegment{{
			ID: "SEG-01", Title: "发现照片", DurationS: 9, Form: "multi_shot",
			Coverage: ShotlistCoverage{EV: []string{"EV-01"}, DLG: []string{"DLG-01"}},
			Shots: []ShotlistShot{
				{Idx: 1, T: "0—4秒", Camera: "门内南侧 35mm 广角低机位，轻微手持后移，保持入口在人物背后", Transition: "随视线硬切近景"},
				{Idx: 2, T: "4—9秒", Camera: "人物右前侧 50mm 中近景，极慢推进，不跳轴，结束停在手部", Transition: "手部动作匹配切道具特写"},
			},
			Prompt: "【全局声音与字幕约束】\n不要BGM、不要字幕，只保留音效。\n\n" +
				"【一句话概述】\nRoko 进入旧公寓并发现冰箱上的照片，情绪从警觉转为迟疑；采用 2 个镜头的连续剪辑，预计 9 秒。\n\n" +
				"【全局视觉设定】\n全局风格：低饱和写实电影质感，冷色窗光为主。\n关键材质与受光表现：湿风衣吸水变深。\n灯光基线：冷色阴天窗光从人物右后方进入。\n\n" +
				"【详细情节】\n0—4秒｜镜头1\n画面动作概述：Roko 推门进入。\n对白：无。\n\n" +
				"4—9秒｜镜头2\n画面动作概述：他停步发现照片。\n对白：「你为什么一直瞒着我？」\n\n" +
				"【全局连续性与结尾约束】\n严格保持人物脸、服装与视线轴一致。",
		}},
	}

	result := validateShotlist(content)
	if len(result.Errors) != 0 {
		t.Errorf("干净内容不应有 ERROR，实际：%+v", result.Errors)
	}
}

func TestRenderShotlistMarkdownHasAllSections(t *testing.T) {
	content := &ShotlistContent{
		Project:  ShotlistProject{Title: "旧公寓", TargetPlatform: "Seedance 2.5", TargetDurationS: 60},
		State:    "PROMPTS_VALIDATED",
		Script:   ShotlistScript{Version: "v1", EV: []ShotlistEV{{ID: "EV-01", Text: "推门"}}},
		Duration: ShotlistDuration{TargetS: 60, FaithfulEstimateS: 72, Verdict: "insufficient", Note: "差额 12 秒"},
		Spatial:  ShotlistSpatial{Needed: false, Reason: "单人戏，无轴线冲突"},
		Segments: []ShotlistSegment{{
			ID: "SEG-01", Title: "发现照片", DurationS: 9, Form: "multi_shot",
			Coverage: ShotlistCoverage{EV: []string{"EV-01"}},
			Shots:    []ShotlistShot{{Idx: 1, T: "0—4秒", Action: "推门", Camera: "低机位"}},
			Prompt:   "【全局声音与字幕约束】\n不要BGM。",
		}},
		Ledger: []ShotlistLedger{{From: "SEG-01", To: "SEG-02", Inherit: []string{"视线"}}},
	}

	md := renderShotlistMarkdown(content)
	for _, want := range []string{
		"# 旧公寓｜Seedance 2.5 分镜与视频提示词",
		"## 1. 项目与范围摘要",
		"## 2. 剧本锁定清单与用户改动授权",
		"## 3. 资产需求判断、用户选择与资产映射",
		"## 4. 故事板方式",
		"## 5. 用户目标时长、忠实估算与工作段方案",
		"## 6. 全剧本 EV/DLG 覆盖表",
		"## 7. 空间调度",
		"## 8. 各工作段文字分镜表",
		"## 10. Seedance 2.5 视频提示词",
		"## 11. 跨工作段连续性与生成风险",
		"SEG-01",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("导出的 Markdown 缺少 %q", want)
		}
	}
	if strings.Contains(md, "## 9.") {
		t.Error("不使用故事板时不应输出第 9 章")
	}
}

func TestLoadSkillResourceBlocksDynamicModuleOutsidePromptStep(t *testing.T) {
	if _, err := loadSkillResource("plan", "reference/MOTION_MODE_ROUTER.md"); err == nil {
		t.Error("plan 步骤不应能加载动态模块")
	}
	if _, err := loadSkillResource("prompts", "reference/MOTION_MODE_ROUTER.md"); err != nil {
		t.Errorf("prompts 步骤应当能加载动态模块，实际报错：%v", err)
	}
	if _, err := loadSkillResource("plan", "../../../etc/passwd"); err == nil {
		t.Error("路径穿越应当被拒绝")
	}
}

func TestEveryStepResourceExists(t *testing.T) {
	for _, step := range shotlistSteps {
		if _, err := loadSkillResources(step); err != nil {
			t.Errorf("步骤 %s 的资源加载失败：%v", step.Key, err)
		}
	}
}

func TestBuildShotlistMessagesInjectsOnlyStepResources(t *testing.T) {
	step, ok := findSkillStep("plan")
	if !ok {
		t.Fatal("找不到 plan 步骤")
	}
	messages, err := buildShotlistMessages(step, json.RawMessage(`{"script":{}}`))
	if err != nil {
		t.Fatalf("组装消息失败：%v", err)
	}
	if len(messages) != 2 || messages[0].Role != "system" || messages[1].Role != "user" {
		t.Fatalf("消息结构不正确：%+v", messages)
	}

	var system string
	if err := json.Unmarshal(messages[0].Content, &system); err != nil {
		t.Fatalf("system 消息不是字符串：%v", err)
	}
	if !strings.Contains(system, "SEEDANCE_2_5_RULES.md") {
		t.Error("plan 步骤应注入 SEEDANCE_2_5_RULES.md")
	}
	if !strings.Contains(system, "PLANNING_CAPACITY.md") {
		t.Error("plan 步骤应注入 PLANNING_CAPACITY.md")
	}
	// 知识文件正文里也会提到别的文件名，所以用注入标记判断，不能用裸文件名。
	if strings.Contains(system, "知识文件：reference/PROMPT_PATTERNS.md") {
		t.Error("plan 步骤不应注入 PROMPT_PATTERNS.md（那是提示词步骤的资源）")
	}
	if strings.Contains(system, "知识文件：reference/MOTION_MODE_ROUTER.md") {
		t.Error("plan 步骤不应注入动态模块")
	}
}

func hasIssue(issues []ShotlistIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
