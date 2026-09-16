package main

// 分镜台节点 content 字段的 JSON 结构。
// 产物本体（故事板图 / 提示词文本）不存这里，只存节点 id 引用，避免数据双份。

type ShotlistContent struct {
	Version  string            `json:"version"`
	Project  ShotlistProject   `json:"project"`
	State    string            `json:"state"`
	Gates    map[string]any    `json:"gates,omitempty"`
	Script   ShotlistScript    `json:"script"`
	Assets   []ShotlistAsset   `json:"assets"`
	Duration ShotlistDuration  `json:"duration"`
	Spatial  ShotlistSpatial   `json:"spatial"`
	Segments []ShotlistSegment `json:"segments"`
	Ledger   []ShotlistLedger  `json:"ledger"`
	Notes    string            `json:"notes,omitempty"`
}

// ShotlistDuration 记录目标时长与忠实估算的对比结果。
type ShotlistDuration struct {
	TargetS           int    `json:"target_s"`
	FaithfulEstimateS int    `json:"faithful_estimate_s"`
	Verdict           string `json:"verdict"` // ok | insufficient
	Note              string `json:"note"`
}

// ShotlistSpatial 记录空间调度的判断结果；needed=false 时必须给出理由。
type ShotlistSpatial struct {
	Needed   bool   `json:"needed"`
	Reason   string `json:"reason"`
	Blocking string `json:"blocking"`
	Axis     string `json:"axis"`
}

type ShotlistProject struct {
	Title           string `json:"title"`
	TargetPlatform  string `json:"target_platform"`
	TargetDurationS int    `json:"target_duration_s"`
}

type ShotlistScript struct {
	SourceNodeID string         `json:"source_node_id,omitempty"`
	Version      string         `json:"version"`
	EV           []ShotlistEV   `json:"EV"`
	DLG          []ShotlistDLG  `json:"DLG"`
	LOCK         []ShotlistLOCK `json:"LOCK"`
}

type ShotlistEV struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type ShotlistDLG struct {
	ID      string `json:"id"`
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

type ShotlistLOCK struct {
	ID   string `json:"id"`
	Fact string `json:"fact"`
}

// ShotlistAsset 把画布上的一条连线翻译成一个素材句柄。
type ShotlistAsset struct {
	Handle string `json:"handle"` // @图片1
	NodeID string `json:"node_id"`
	Name   string `json:"name,omitempty"`
	Role   string `json:"role,omitempty"` // character | scene | prop | style | pose
	Use    string `json:"use"`
	NotUse string `json:"not_use"`
}

type ShotlistSegment struct {
	ID         string              `json:"id"`
	Title      string              `json:"title"`
	DurationS  int                 `json:"duration_s"`
	Form       string              `json:"form"` // multi_shot | one_take
	Coverage   ShotlistCoverage    `json:"coverage"`
	Shots      []ShotlistShot      `json:"shots"`
	Storyboard *ShotlistStoryboard `json:"storyboard,omitempty"`
	Prompt     string              `json:"prompt"`
	PromptNode string              `json:"prompt_node_id,omitempty"`
	VideoNode  string              `json:"video_node_id,omitempty"`
	Validation *ShotlistValidation `json:"validation,omitempty"`
}

type ShotlistCoverage struct {
	EV  []string `json:"EV"`
	DLG []string `json:"DLG"`
}

// ShotlistShot 的字段顺序即 PROMPT_PATTERNS.md 规定的固定顺序。
type ShotlistShot struct {
	Idx           int    `json:"idx"`
	T             string `json:"t"`
	Action        string `json:"action"`
	Composition   string `json:"composition"`
	Camera        string `json:"camera"`
	Performance   string `json:"performance"`
	Dialogue      string `json:"dialogue"`
	Sound         string `json:"sound"`
	Transition    string `json:"transition"`
	StoryboardRef string `json:"storyboard_ref,omitempty"`
}

type ShotlistStoryboard struct {
	Mode   string `json:"mode"` // use | skip
	Rows   int    `json:"rows"`
	Cols   int    `json:"cols"`
	N      int    `json:"n"`
	NodeID string `json:"node_id,omitempty"`
	Review string `json:"review,omitempty"` // pending | passed | rejected
}

type ShotlistLedger struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Inherit []string `json:"inherit"`
}

// ShotlistIssue 是一条校验结果。
type ShotlistIssue struct {
	Level   string `json:"level"` // ERROR | WARNING
	Code    string `json:"code"`
	Where   string `json:"where"` // 全局 或 SEG-01
	Message string `json:"message"`
}

type ShotlistValidation struct {
	Errors   []ShotlistIssue `json:"errors"`
	Warnings []ShotlistIssue `json:"warnings"`
}

// 分镜台状态机的全部状态，顺序即 SKILL.md 的推进顺序。
var shotlistStateOrder = []string{
	"SOURCE_READY",
	"SCRIPT_LOCKED",
	"ASSET_CONFIRMED",
	"MODE_CONFIRMED",
	"SPATIAL_CONFIRMED",
	"PLAN_CONFIRMED",
	"STORYBOARD_CONFIRMED",
	"PROMPTS_VALIDATED",
	"DELIVERED",
}

// stateIndex 返回状态在状态机中的序号，未知状态返回 -1。
func stateIndex(state string) int {
	for i, s := range shotlistStateOrder {
		if s == state {
			return i
		}
	}
	return -1
}
