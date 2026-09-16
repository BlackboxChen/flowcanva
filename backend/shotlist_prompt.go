package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// shotlistPersona 是所有步骤共享的角色与铁律，来源为 SKILL.md 的「不可违反的约束」。
const shotlistPersona = `你是 FlowCanva 画布里的导演分镜助手，执行 Deepwhite 导演拍摄技能 v3.7。

不可违反的约束：
- 把用户提供的剧本作为唯一剧情源。逐字保留台词的说话人、文字、顺序和含义。
- 只补充摄影、构图、调度、表演微节拍、环境活动、声音和生成控制；
  不得改变剧情事实、人物动机、关系、因果、胜负、伤势或结局。
- 遇到时长、容量或生成稳定性冲突时，如实报告差额并请求用户授权，不得自行删改剧本。
- 只完成当前步骤要求的产出，不要提前生成后续步骤的内容。
- 目标生成模型为 Seedance 2.5。

输出格式要求（必须严格遵守）：
- 只输出一个 JSON 对象。不要输出 Markdown 代码围栏，不要输出任何解释性文字或前后缀。
- JSON 必须能被标准解析器直接解析：双引号、无尾随逗号、无注释、无省略号。
- 所有字符串里的换行写成 \n 转义，不要输出真实换行符。`

// shotlistPromptSpec 描述单个步骤的任务说明与期望的 JSON 结构。
type shotlistPromptSpec struct {
	Task   string
	Schema string
}

var shotlistPromptSpecs = map[string]shotlistPromptSpec{
	"lock_script": {
		Task: `本步任务：锁定剧本。
完整读取用户提供的剧本正文，区分「剧本事实」与「导演可设计层」。
建立 EV（事件）、DLG（台词）、LOCK（不可改动的事实）三份清单。
每句台词原文必须逐字写入 DLG，不得润色、缩短、同义替换、合并或补写。
剧本缺少必要内容时，在 notes 里说明缺什么，不要自行编造。`,
		Schema: `{
  "script": {
    "version": "v1",
    "EV":   [{"id": "EV-01",  "text": "事件描述"}],
    "DLG":  [{"id": "DLG-01", "speaker": "说话人", "text": "台词原文"}],
    "LOCK": [{"id": "LOCK-01", "fact": "不可改动的剧本事实"}]
  },
  "notes": "剧本范围、缺失项与改动授权说明"
}`,
	},

	"assets": {
		Task: `本步任务：资产映射。
用户已经通过画布连线选定了要使用的素材，每项素材带有 node_id 与显示名。
为每一项素材写清「参考什么」与「不参考什么」，两者都必须写。
- 精确参考图已清楚呈现外观时，不重复堆砌细节。
- 姿势参考只控制动作，不控制身份、服装和场景。
- 构图参考只控制画面关系，不自动控制画风。
handle 按输入顺序从 @图片1 开始编号，node_id 必须原样返回，不要改写。
用户选择不使用任何资产时，返回空数组并在 notes 说明一致性风险。`,
		Schema: `{
  "assets": [
    {
      "handle": "@图片1",
      "node_id": "原样返回输入的 node_id",
      "role": "character | scene | prop | style | pose",
      "use": "参考……",
      "not_use": "不参考……"
    }
  ],
  "notes": "不使用资产时的一致性风险提示，或空字符串"
}`,
	},

	"spatial": {
		Task: `本步任务：判断是否需要俯视调度图。
依次判断当前工作段的多人关系、关键道具落点、遮挡、视线、运动轴线、摄影机方位是否会影响生成结果。
- 任一项成立：needed=true，并在 blocking 写出俯视调度，在 axis 写出运动轴线与摄影机方位。
- 全部不成立：needed=false，在 reason 写明跳过理由，blocking 与 axis 留空字符串。
不要为了凑内容而强行输出调度图。`,
		Schema: `{
  "spatial": {
    "needed": true,
    "reason": "需要或不需要的判断理由",
    "blocking": "俯视调度文字描述；needed=false 时为空字符串",
    "axis": "运动轴线与摄影机方位；needed=false 时为空字符串"
  }
}`,
	},

	"plan": {
		Task: `本步任务：规划时长、工作段与文字分镜。
1. 比较用户目标时长与忠实呈现估算。目标不足时在 duration.note 报告差额，verdict 写 insufficient，绝不删减 EV/DLG。
2. 按剧情目标、动作结果、情绪转折、模型容量拆分 SEG 工作段。单段时长必须在 4—30 秒之间。
3. 每段选择拍摄形式：form 为 multi_shot（多镜头剪辑）或 one_take（单镜头一镜到底）。
4. 为每个镜头完成导演判断并写入 shots，字段固定为
   action / composition / camera / performance / dialogue / sound / transition，顺序不变。
   - action 是画面动作概述；camera 要写清摄影机起始状态、跟随对象、触发变化和结束动势。
   - dialogue 逐字复制对应 DLG 的台词；无对白写「无」。
   - transition 按需继承人物位置、速度、方向、重心、视线、道具手位、摄影机侧位、环境运动和声音桥。
   - 没有信息焦点、表演节拍、空间关系或主任务变化时，不只为换景别切镜。
5. 每段必须给出 coverage，列出本段覆盖的 EV 与 DLG 编号。全剧本的 EV/DLG 必须被完整覆盖。
6. ledger 记录相邻工作段之间需要继承的连续性要素。`,
		Schema: `{
  "duration": {
    "target_s": 60,
    "faithful_estimate_s": 72,
    "verdict": "ok | insufficient",
    "note": "差额与处理建议"
  },
  "segments": [
    {
      "id": "SEG-01",
      "title": "工作段标题",
      "duration_s": 14,
      "form": "multi_shot",
      "coverage": {"EV": ["EV-01"], "DLG": ["DLG-01"]},
      "shots": [
        {
          "idx": 1,
          "t": "0—4秒",
          "action": "画面动作概述",
          "composition": "画面构图",
          "camera": "机位：起始状态、跟随对象、触发变化、结束动势",
          "performance": "动作/表演",
          "dialogue": "逐字台词；无对白写「无」",
          "sound": "声音",
          "transition": "转场"
        }
      ]
    }
  ],
  "ledger": [
    {"from": "SEG-01", "to": "SEG-02", "inherit": ["人物位置", "视线", "摄影机侧位"]}
  ]
}`,
	},

	"prompts": {
		Task: `本步任务：编写 Seedance 2.5 视频提示词。
对指定的每个工作段各写一条完整提示词，严格遵循 PROMPT_PATTERNS.md 的结构与字段顺序：
【全局声音与字幕约束】→【素材说明】→【一句话概述】→【全局视觉设定】→【详细情节】→【全局连续性与结尾约束】。
- 第一行必须是「不要BGM、不要字幕，只保留音效。」
- 【素材说明】按输入 assets 的顺序声明，每项同时写参考与不参考内容；无资产时整段省略，不写占位。
- 【一句话概述】概括本段并写明拍摄形式与实际时长。
- 【全局视觉设定】必须写全「全局风格」「关键材质与受光表现」「灯光基线」，且不得混入具体运镜。
- 【详细情节】用连续整数秒时间戳覆盖完整时长；多镜头用「0—4秒｜镜头1」格式，一镜到底用「路径节点」格式。
- 多镜头的每镜头字段固定顺序：画面动作概述 → 画面构图 → 机位 → 动作/表演 → 对白 → 声音 → 转场。
- 对白必须逐字复制对应 DLG，保持说话人、字词与顺序；不得改成旁白或字幕。
- 本段不使用故事板，因此完全省略故事板字段、故事板占位与 SB- 引用，不写相关禁令。
- 时间戳必须位于正文，使用整数秒，连续覆盖完整时长，不得重叠。`,
		Schema: `{
  "segments": [
    {
      "id": "SEG-01",
      "prompt": "【全局声音与字幕约束】\\n不要BGM、不要字幕，只保留音效。\\n\\n【一句话概述】\\n……\\n\\n【全局视觉设定】\\n……\\n\\n【详细情节】\\n……\\n\\n【全局连续性与结尾约束】\\n……"
    }
  ]
}`,
	},
}

// buildShotlistMessages 组装一次分镜步骤调用的完整 messages。
// 只注入当前步骤声明的知识文件，不做任何预加载。
func buildShotlistMessages(step SkillStep, input json.RawMessage) ([]ChatMessage, error) {
	spec, ok := shotlistPromptSpecs[step.Key]
	if !ok {
		return nil, fmt.Errorf("步骤 %s 不需要模型参与", step.Key)
	}

	knowledge, err := loadSkillResources(step)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString(shotlistPersona)
	if knowledge != "" {
		sb.WriteString("\n\n以下是本步骤必须遵守的知识文件原文：")
		sb.WriteString(knowledge)
	}
	sb.WriteString("\n\n")
	sb.WriteString(spec.Task)
	sb.WriteString("\n\n请严格按以下 JSON 结构输出：\n")
	sb.WriteString(spec.Schema)

	systemMsg, err := newTextMessage("system", sb.String())
	if err != nil {
		return nil, err
	}

	userText := "以下是本步骤的输入数据（JSON）：\n" + string(input)
	userMsg, err := newTextMessage("user", userText)
	if err != nil {
		return nil, err
	}

	return []ChatMessage{systemMsg, userMsg}, nil
}

// newTextMessage 构造一条纯文本消息。
func newTextMessage(role, text string) (ChatMessage, error) {
	encoded, err := json.Marshal(text)
	if err != nil {
		return ChatMessage{}, err
	}
	return ChatMessage{Role: role, Content: encoded}, nil
}
