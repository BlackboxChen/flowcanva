package main

import (
	"embed"
	"fmt"
	"path"
	"strings"
)

// 分镜技能知识库内嵌进二进制，发布版保持单 exe 零依赖。
//
//go:embed skills/shotlist
var shotlistSkillFS embed.FS

const shotlistSkillRoot = "skills/shotlist"

// SkillStep 描述分镜技能的一个执行步骤，与 SKILL.md 状态机中的一个状态对应。
type SkillStep struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Gate        string   `json:"gate"`      // 完成后进入的状态
	Kind        string   `json:"kind"`      // llm=需要模型参与，manual=纯人工确认
	Resources   []string `json:"resources"` // 本步注入的知识文件（相对技能根目录）
	Description string   `json:"description"`
}

// promptStepKey 是唯一允许加载动态模块的步骤。
// SKILL.md：只有第 6 步（编写视频提示词）允许读取并使用动态模块。
const promptStepKey = "prompts"

// dynamicOnlyResources 动态模块白名单，只能在 promptStepKey 步骤加载。
var dynamicOnlyResources = map[string]bool{
	"reference/MOTION_MODE_ROUTER.md":      true,
	"reference/DYNAMIC_MOTION_PHYSICS.md":  true,
	"reference/DYNAMIC_PLANNING.md":        true,
	"reference/DYNAMIC_PROMPT_CONTRACT.md": true,
	"reference/DYNAMIC_SPATIAL_LOCK.md":    true,
	"reference/DYNAMIC_DIAGNOSIS_QC.md":    true,
}

// shotlistSteps 严格按 SKILL.md 的状态机顺序排列。
// P0 只覆盖「不使用故事板」分支（5A），故事板分支（5B）留待 P1。
var shotlistSteps = []SkillStep{
	{
		Key: "lock_script", Label: "锁定剧本", Gate: "SCRIPT_LOCKED", Kind: "llm",
		Resources:   []string{"reference/SCRIPT_FIDELITY.md"},
		Description: "把剧本拆成 EV/DLG/LOCK 清单，台词逐字保留，不做任何改编",
	},
	{
		Key: "assets", Label: "资产映射", Gate: "ASSET_CONFIRMED", Kind: "llm",
		Resources:   []string{"reference/ASSET_CONFIRMATION.md"},
		Description: "为连入的素材写清「参考什么 / 不参考什么」边界",
	},
	{
		Key: "mode", Label: "故事板方式", Gate: "MODE_CONFIRMED", Kind: "manual",
		Resources:   nil,
		Description: "确认本段是否使用故事板。P0 仅支持「不使用故事板」",
	},
	{
		Key: "spatial", Label: "空间调度", Gate: "SPATIAL_CONFIRMED", Kind: "llm",
		Resources:   []string{"reference/SPATIAL_BLOCKING.md"},
		Description: "判断是否需要俯视调度图；不需要时给出明确理由",
	},
	{
		Key: "plan", Label: "工作段规划", Gate: "PLAN_CONFIRMED", Kind: "llm",
		Resources: []string{
			"reference/SEEDANCE_2_5_RULES.md",
			"reference/PLANNING_CAPACITY.md",
			"reference/DRAMA_SHOT_MOMENTUM.md",
		},
		Description: "拆分 SEG 工作段、比较目标时长与忠实估算、产出文字分镜表与 EV/DLG 覆盖表",
	},
	{
		Key: "prompts", Label: "视频提示词", Gate: "PROMPTS_VALIDATED", Kind: "llm",
		Resources: []string{
			"reference/PROMPT_PATTERNS.md",
			"reference/DRAMA_SHOT_MOMENTUM.md",
			"reference/MICRO_BEATS.md",
			"reference/CAMERA_EMOTION.md",
		},
		Description: "按 PROMPT_PATTERNS 结构逐段产出 Seedance 2.5 视频提示词",
	},
	{
		Key: "export", Label: "交付导出", Gate: "DELIVERED", Kind: "manual",
		Resources:   []string{"reference/DELIVERY_FORMAT.md"},
		Description: "按 DELIVERY_FORMAT 结构渲染完整 Markdown 交付文件",
	},
}

// findSkillStep 按 key 查找步骤定义。
func findSkillStep(key string) (SkillStep, bool) {
	for _, s := range shotlistSteps {
		if s.Key == key {
			return s, true
		}
	}
	return SkillStep{}, false
}

// loadSkillResource 读取一个知识文件。
// 拒绝路径穿越，并强制动态模块只能在提示词步骤加载。
func loadSkillResource(stepKey, rel string) (string, error) {
	rel = strings.TrimPrefix(strings.TrimSpace(rel), "/")
	if rel == "" || strings.Contains(rel, "..") {
		return "", fmt.Errorf("非法的技能资源路径: %q", rel)
	}
	if dynamicOnlyResources[rel] && stepKey != promptStepKey {
		return "", fmt.Errorf("动态模块 %s 只允许在「%s」步骤加载，当前步骤为 %s", rel, promptStepKey, stepKey)
	}
	data, err := shotlistSkillFS.ReadFile(path.Join(shotlistSkillRoot, rel))
	if err != nil {
		return "", fmt.Errorf("读取技能资源 %s 失败: %w", rel, err)
	}
	return string(data), nil
}

// loadSkillResources 按顺序拼接某一步骤需要的全部知识文件。
// 只注入当前步骤声明的资源，不做任何预加载。
func loadSkillResources(step SkillStep) (string, error) {
	var sb strings.Builder
	for _, rel := range step.Resources {
		content, err := loadSkillResource(step.Key, rel)
		if err != nil {
			return "", err
		}
		sb.WriteString("\n\n===== 知识文件：")
		sb.WriteString(rel)
		sb.WriteString(" =====\n")
		sb.WriteString(content)
	}
	return sb.String(), nil
}

// skillResourceManifest 返回知识库里实际存在的文件清单，供前端展示与排障。
func skillResourceManifest() ([]string, error) {
	var files []string
	err := walkSkillDir(shotlistSkillRoot, &files)
	return files, err
}

func walkSkillDir(dir string, out *[]string) error {
	entries, err := shotlistSkillFS.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		full := path.Join(dir, entry.Name())
		if entry.IsDir() {
			if err := walkSkillDir(full, out); err != nil {
				return err
			}
			continue
		}
		*out = append(*out, strings.TrimPrefix(full, shotlistSkillRoot+"/"))
	}
	return nil
}
