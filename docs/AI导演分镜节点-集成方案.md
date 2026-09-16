# AI 导演分镜节点（Shotlist）集成方案

> 目标：把 `deepwhite-director-shooting-v3-7` 技能的多阶段工作流搬进 FlowCanva 画布，
> 让"锁定剧本 → 资产映射 → 空间调度 → 工作段规划 → 故事板 → Seedance 2.5 视频提示词 → 出片"
> 变成画布上一条可视化、可断点续跑、产物可复用的流水线。
>
> 版本：v1 草案 ｜ 面向 FlowCanva（Go + Vue3 + Three.js）

---

## 0. 一句话结论

**新增一个节点类型 `shotlist`（显示名「分镜台」），它既是流程主控（承载 v3.7 的 9 状态状态机与 Gate 协议），
又是产物分发器——每完成一个阶段，就把产物落地成画布上的真实节点（table / image / text / video）并用连线串起来。**

不复刻技能的文件输出形态，而是把技能拆成三块分别安置：

| 技能组成 | 在画布里的归宿 |
|---|---|
| 流程骨架（9 状态 + Gate + 返工路由） | `shotlist` 节点的状态机 + 面板上的 Gate 卡片 |
| 知识库（`reference/*.md` 19 个文件 / 148K） | 后端 `go:embed` 嵌入，按步骤**按需注入** system prompt |
| 产出物（Shotlist.md 11 章 + N 宫格故事板 + 视频提示词） | 落地为 `table` / `image` / `text` / `video` 节点，导出时才拼成 Markdown |

**不要用 `director` 作为类型名**——该类型已被现有的 Three.js 3D 导演台占用（`database.go:113`、`types/index.ts:14`）。

---

## 1. 为什么是"主控节点 + 产物落地"而不是别的

三个候选方案对比：

| | 方案 A：单节点向导面板 | **方案 B：主控节点 + 产物落地（推荐）** | 方案 C：后端任务引擎 |
|---|---|---|---|
| 做法 | 9 个状态塞进一个面板的多步向导，产物全存 `content` | 主控节点跑流程，产物落地成真实节点并连线 | 新建 task 表 + worker + SSE，后端全自动跑 |
| 复用画布能力 | 几乎没有 | **复用图片生成、视频生成、素材连线、`@图片N` 芯片** | 复用生成端点，但绕开了画布 |
| 产物可编辑/可复用 | 差，困在 JSON 里 | **好，能被下游节点直接消费** | 中 |
| 工作量 | 小 | 中 | 大 |
| 主要问题 | 看不到画布的可视化优势 | 需要新增"面板创建节点"能力 | 无任务表/无 SSE，几乎从零建 |

**选 B 的决定性理由**：技能里反复出现的 `@图片1：参考女主的脸部身份……不参考原图姿势`，
在画布里天然就是**「上游连过来的图片节点」**。技能要求"资产映射必须确认"，
画布里就是"勾选哪几条连线参与参考"。这不是勉强类比，而是同一个语义。
把这个映射关系用起来，技能的 Gate 就变成了画布上看得见、点得到的东西。

---

## 2. 整体架构

```
                    画布（InfiniteCanvas）
┌────────────────────────────────────────────────────────────────┐
│                                                                │
│  [剧本 table]──┐                                               │
│                ├──▶ ┌──────────────────┐                       │
│  [角色 image]──┤    │  分镜台 shotlist  │                       │
│  [场景 image]──┤    │  ──────────────  │                       │
│  [道具 image]──┘    │  状态机 / Gate    │                       │
│                     └────────┬─────────┘                       │
│                              │ 产物落地 + 自动连线               │
│         ┌────────────────────┼────────────────────┐            │
│         ▼                    ▼                    ▼            │
│   [文字分镜 table]    [故事板 image]      [提示词 text]          │
│                              │                    │            │
│                              └────────┬───────────┘            │
│                                       ▼                        │
│                                 [视频 video]  ← Seedance 2.5   │
└────────────────────────────────────────────────────────────────┘
                               │
                               ▼
             后端 /api/skills/shotlist/*（新增）
             ├── skill_pack.go      知识库嵌入 + 步骤→资源映射
             ├── shotlist_prompt.go 各步骤 prompt 组装
             ├── shotlist_validate.go  Python 校验脚本的 Go 移植
             └── shotlist_handlers.go 路由
                               │
                               ▼
             复用既有端点：/api/llm/chat、/api/images/generate、/api/video/generate
```

**职责边界**

- 前端面板：状态机推进、Gate 交互、步骤执行编排、产物落地
- 后端：知识库供给、prompt 组装、结构校验、无状态代理
- 复用：所有生成能力（LLM / 图片 / 视频）一律走既有端点，不新开生成通道

---

## 3. 数据模型

### 3.1 节点类型

```
node_type = 'shotlist'     显示名「分镜台」
默认尺寸 640 × 420（useNodes.ts:96-106 的 defaults 表）
```

### 3.2 `nodes.content` JSON Schema

`content` 是唯一数据载体（沿用现有约定）。**产物本体不存这里，只存 node_id 引用**，避免数据双份。

```jsonc
{
  "version": "3.7",
  "project": {
    "title": "旧公寓",
    "target_platform": "Seedance 2.5",
    "target_duration_s": 60
  },

  "state": "PLAN_CONFIRMED",          // 9 状态之一，断点续跑的唯一依据

  "gates": {                          // Gate 确认记录，满足后不再重复询问
    "SOURCE_READY":      { "at": "2026-09-15T15:20:00Z" },
    "SCRIPT_LOCKED":     { "at": "...", "version": "v1" },
    "ASSET_CONFIRMED":   { "decision": "use", "at": "..." },
    "MODE_CONFIRMED":    { "SEG-01": "use", "SEG-02": "skip" },
    "SPATIAL_CONFIRMED": { "decision": "skip", "reason": "单人戏，无轴线冲突" },
    "PLAN_CONFIRMED":    { "at": "..." }
  },

  "script": {                         // SCRIPT_LOCKED 产物
    "source_node_id": "nd_ab12cd34",
    "version": "v1",
    "EV":   [{ "id": "EV-01",  "text": "Roko 推门进入旧公寓" }],
    "DLG":  [{ "id": "DLG-01", "speaker": "Roko", "text": "你为什么一直瞒着我？" }],
    "LOCK": [{ "id": "LOCK-01", "fact": "照片在冰箱上，右手取" }]
  },

  "assets": [                         // 资产映射：把连线翻译成 @图片N 句柄
    {
      "handle": "@图片1",
      "node_id": "nd_role01",
      "role": "character",
      "use": "参考女主的脸部身份、发型和黑色风衣",
      "not_use": "不参考原图姿势、背景、光线和文字"
    }
  ],

  "segments": [
    {
      "id": "SEG-01",
      "title": "旧公寓发现照片",
      "duration_s": 14,
      "form": "multi_shot",           // multi_shot | one_take
      "storyboard": {
        "mode": "use",                 // use | skip（按 SEG 独立记录，不继承）
        "grid": { "rows": 3, "cols": 3, "n": 9 },
        "node_id": "nd_sb01",          // 落地后的 image 节点
        "review": "passed",            // pending | passed | rejected
        "review_notes": ""
      },
      "shots": [
        {
          "idx": 1, "t": "0—4秒", "storyboard_ref": "SB-01",
          "action": "Roko 推门进入，湿靴跨过门槛",
          "composition": "人物位于左三分之一，门框形成前景遮挡",
          "camera": "门内南侧 35mm 广角低机位，轻微手持后移",
          "performance": "肩膀略抬，呼吸短促",
          "dialogue": "无",
          "sound": "门轴轻响、湿靴落地声",
          "transition": "4 秒处随他的视线硬切近景"
        }
      ],
      "coverage": { "EV": ["EV-01"], "DLG": [] },   // EV/DLG 覆盖表
      "prompt_node_id": "nd_p001",    // 落地后的 text 节点
      "prompt": "【全局声音与字幕约束】\n不要BGM、不要字幕，只保留音效。\n...",
      "video_node_id": "nd_v001",     // 可选：生成视频后的 video 节点
      "validation": { "errors": [], "warnings": [], "checked_at": "..." }
    }
  ],

  "ledger": [                         // 跨工作段连续性账本
    { "from": "SEG-01", "to": "SEG-02", "inherit": ["人物位置", "速度", "视线", "摄影机侧位"] }
  ],

  "_step": "plan",                    // 当前正在执行的步骤，用于进度显示
  "_loading": false,
  "_loading_started_at": 0,
  "_error": ""
}
```

**为什么用 JSON 而不是 Markdown 做中间表示**：
Markdown 只是最终渲染形态。校验（逐字比对 DLG、时间戳连续性）、落地节点、导出三件事都要读同一份数据，
用 JSON 做单一事实源，Markdown 由导出器生成。

---

## 4. 状态机 ↔ 画布 Gate 映射

技能的状态机与 Gate 协议**完整保留**，只是每个 Gate 在画布上换了个交互形态：

| 技能状态 | 画布上的交互 | 完成条件 | 谁触发 |
|---|---|---|---|
| `SOURCE_READY` | 检测到连入的 `table`/`text` 节点有内容 | 剧本正文 + 制作范围 + 目标平台齐备 | 自动 |
| `SCRIPT_LOCKED` | 面板显示 EV/DLG/LOCK 三张清单，可逐条编辑 | 用户点「锁定剧本」 | 人工一键 |
| `ASSET_CONFIRMED` | 列出所有连入的 `asset`/`image` 节点，勾选 + 填「参考/不参考」 | 全部勾选项填完边界 | 人工勾选 |
| `MODE_CONFIRMED` | 每个 SEG 一个「用/不用故事板」开关 | 所有 SEG 都有取值 | 人工 |
| `SPATIAL_CONFIRMED` | 生成俯视调度 SVG 内联预览 | 确认或填跳过理由 | 人工 |
| `PLAN_CONFIRMED` | 工作段表 + 文字分镜表（表格形式） | 确认 | 人工 |
| `STORYBOARD_CONFIRMED` | 每段宫格图落地成 `image` 节点 → 人工审图 → 通过/打回 | 该段返图审查通过 | 人工 |
| `PROMPTS_VALIDATED` | 自动跑 Go 版校验，错误/警告内联展示 | 无 ERROR | 自动 |
| `DELIVERED` | 「导出 Shotlist.md」按钮 | 文件产出 | 一键 |

**必须守住的几条（技能的不可违反约束）**：

1. 未满足当前分支硬性 Gate 时**不生成**故事板、正式提示词或最终交付——面板的按钮按状态禁用。
2. `MODE_CONFIRMED` **按 SEG 记录，不继承**。第一段选了故事板，第二段仍要单独问。
3. Gate 一旦确认就写进 `content.gates`，**不再重复询问**（对应技能"把用户此前消息中的明确选择视为已通过对应 Gate"）。
4. 返工路由按技能原表实现——只重做受影响范围，不回退整个项目。例如改提示词措辞只回到 `PROMPTS_VALIDATED`。

---

## 5. 知识库注入方案（关键工程点）

技能的 19 个 `reference/*.md` 共 148K，**绝不能全量塞进 context**。
技能自己就写了「保持引用只从本文件直接进入一层资源；不要为了'可能有用'预加载所有参考文件」——照搬这条原则。

### 5.1 落位

```
backend/skills/shotlist/
├── SKILL.md                    # 技能主文件（流程骨架）
├── reference/                  # 19 个 md，原样拷入
├── scripts/validate_delivery.py  # 保留作参考，实际逻辑移植到 Go
└── templates/HTML_TEMPLATE.md
```

用 `go:embed` 嵌入（与 `embed.go:14` 嵌入 `seed.json` 同机制），用户端零依赖。

### 5.2 步骤 → 资源映射表

后端维护一张硬编码映射，**每次 LLM 调用只注入当前步骤需要的 1~3 个文件**：

```go
// backend/skill_pack.go
var stepResources = map[string][]string{
    "lock_script":  {"reference/SCRIPT_FIDELITY.md"},
    "assets":       {"reference/ASSET_CONFIRMATION.md"},
    "spatial":      {"reference/SPATIAL_BLOCKING.md"},
    "plan":         {"reference/SEEDANCE_2_5_RULES.md", "reference/PLANNING_CAPACITY.md"},
    "plan_drama":   {"reference/DRAMA_SHOT_MOMENTUM.md"},
    "storyboard":   {"reference/STORYBOARD_ASSETS.md"},
    "prompt_drama": {"reference/PROMPT_PATTERNS.md", "reference/DRAMA_SHOT_MOMENTUM.md",
                     "reference/MICRO_BEATS.md", "reference/CAMERA_EMOTION.md"},
    "prompt_action":{"reference/PROMPT_PATTERNS.md", "reference/MOTION_MODE_ROUTER.md",
                     "reference/DYNAMIC_MOTION_PHYSICS.md", "reference/DYNAMIC_PLANNING.md",
                     "reference/DYNAMIC_PROMPT_CONTRACT.md"},
    "deliver":      {"reference/DELIVERY_FORMAT.md"},
}
```

**动态模块只在第 6 步（编写提示词）才允许加载**——这是技能里写死的硬约束，
后端要在路由层拦住：`plan` 步骤请求 `MOTION_MODE_ROUTER.md` 一律拒绝。

### 5.3 新增端点

```
GET  /api/skills                      列出可用技能
GET  /api/skills/shotlist/steps       返回步骤清单、Gate 定义、每步资源
POST /api/skills/shotlist/prompt      组装某一步的 system+user messages 并转发 LLM
POST /api/skills/shotlist/validate    跑结构校验，返回 {errors, warnings}
POST /api/skills/shotlist/export      由 JSON 渲染 Shotlist.md
```

`/prompt` 是核心：前端只说"我要执行 plan 步骤，输入是这份 script JSON"，
后端负责挑资源、拼 prompt、调上游、返回。**前端永远不接触 reference 内容**，知识库演进不影响前端。

---

## 6. 校验脚本移植

技能的 `scripts/validate_delivery.py` 必须移植成 Go——后端没有任何 Python/shell 执行能力
（`main.go` 唯一的 `os/exec` 是开浏览器，参数全硬编码）。

新建 `backend/shotlist_validate.go`，输入是 §3.2 的 JSON 结构，输出 `{errors, warnings}`。

必须覆盖的检查项（对照 `PROMPT_PATTERNS.md` 最终自检）：

| 级别 | 检查项 |
|---|---|
| ERROR | `DLG` 中的台词是否逐字出现在对应提示词的「对白」字段（说话人 + 文字 + 顺序） |
| ERROR | 本段 `EV`/`DLG` 覆盖是否完整（`coverage` 与 `segments[].shots` 实际引用比对） |
| ERROR | 时间戳是否整数秒、连续、无重叠、覆盖完整时长 |
| ERROR | 单元时长是否落在 4—30 秒 |
| ERROR | 提示词是否以`【全局声音与字幕约束】`开头，且含`【一句话概述】`/`【全局视觉设定】`/`【详细情节】`/`【全局连续性与结尾约束】` |
| ERROR | 使用故事板时，`storyboard.node_id` 必须存在且 `review == "passed"` |
| ERROR | 不使用故事板时，提示词里不得出现 `@图片N…故事板` 或 `SB-` 引用 |
| WARN | `【全局视觉设定】` 里混入了具体运镜词 |
| WARN | 素材只写了「参考」没写「不参考」（或反之） |
| WARN | 文戏镜头缺少「第一帧/结束动势」描述 |
| WARN | 多镜头切点无必要性说明，或 `transition` 未继承任何连续性要素 |

**校验不能替代返图审查和剧情语义判断**——这条要写进面板的提示文案。

---

## 7. 流程执行方式

### 7.1 第一期：前端编排（推荐）

面板内维护 step runner，每一步 = 一次 `/api/skills/shotlist/prompt` 调用：

```
用户点「生成文字分镜」
  → 面板 emit('save', {..., _loading: true, _step: 'plan'})     // 先落库，画布显示"生成中"
  → POST /api/skills/shotlist/prompt  { step: 'plan', script, assets, target_duration }
  → 流式返回（复用 chatWithLLMStream 的 SSE 解析）
  → 解析 → 写入 segments[] → emit('save', {..., _loading: false, state: 'PLAN_CONFIRMED'})
  → 产物落地为 table 节点并连线
```

要点：
- **每步完成立即落库**，刷新页面从 `content.state` 恢复，不丢进度
- 复用 `VideoNodePanel` 的 `_loading` + `_loading_started_at` + `AbortController` 模式
- 复用 `App.vue:367` 的 `expireStaleVideoTasks` 思路，加一个 `expireStaleShotlistTasks`
- 长任务（故事板图片）本身就是既有 `POST /api/images/generate`，无需改造

### 7.2 第二期（可选）：后端任务引擎

若需要"关掉页面也不中断"，再加：
- 新表 `skill_runs`（id / node_id / state / steps JSON / created_at）与 `run_steps`
- `POST /api/skills/shotlist/run` + `GET /api/skills/shotlist/run/:id/events`（SSE，照抄 `llm_handler.go:121-146` 的写法）

**第一期不做**，因为后端目前没有任务表、没有 worker、所有生成都是请求内同步阻塞（最长 30 分钟占连接），
引入任务引擎是独立的一块工程，不应和本功能耦合上线。

---

## 8. 产物落地策略

这是本方案的核心动作。每个阶段的产物落地成真实节点：

| 阶段 | 落地节点类型 | 复用的现有能力 |
|---|---|---|
| 剧本锁定 | `table` | `ScriptEditorModal.vue`（15 列默认表头、TSV 粘贴、列宽拖拽） |
| 空间调度 | 内联 SVG（不落地）或 `image` | 直接渲染俯视图 |
| 文字分镜 | `table` | 同上，列头用镜号/时长/画面描述/机位/动作/对白/声音/转场 |
| 故事板宫格 | `image` | `POST /api/images/generate`，参考图 = 已确认资产（最多 16 张） |
| 视频提示词 | `text` | 直接可复制 |
| 生成视频 | `video` | `POST /api/video/generate`（默认模型已是 `doubao-seedance-2-0-260128`） |

### 8.1 需要新增的能力：面板创建节点

现有代码里已有先例——`App.vue:541-572` 在 image/video 返回多个结果时会自动克隆节点并复制连线。
把它抽象成通用能力：

```ts
// App.vue 新增
async function spawnNode(opts: {
  type: string
  content: string
  nearNodeId: string        // 以哪个节点为锚点定位
  offset?: { dx: number; dy: number }
  connectFrom?: string[]    // 需要连线的源节点
  connectTo?: string[]
}): Promise<Node>
```

`shotlist` 面板通过新事件 `@spawn-node` 调用它。**这是本方案唯一需要动到 App.vue 核心逻辑的地方**，要谨慎。

### 8.2 反向数据流：产物节点回写给分镜台

用户在落地后的 `table`/`text` 节点上手动改了内容，分镜台要知道。
两个选项：

- **A（推荐，第一期）**：不反向同步。分镜台只在「重新生成」时读回上游节点最新内容。简单、无环。
- **B（第二期）**：给 `nodeInputs` 加分支，让分镜台监听产物节点的变更。会引入双向依赖，慎重。

---

## 9. 改动清单

### 9.1 前端画布端（`frontend-canvas/`）

**必改 8 处**（新增节点类型的标准动作）：

| # | 文件 | 位置 | 改什么 |
|---|---|---|---|
| 1 | `src/types/index.ts` | `:14` | 联合类型加 `\| 'shotlist'` |
| 2 | `src/App.vue` | `:21-31` `NODE_LABELS` | 加 `shotlist: '分镜台'`；`:139` 视需要加入 `alwaysOn` |
| 3 | `src/components/LeftToolbar.vue` | `:26-88` | 加一段 `<svg v-else-if="nt.type === 'shotlist'">` |
| 4 | `src/components/CanvasNode.vue` | `:402-439` `typeMeta` | 加名称 + 16×16 图标 path |
| 5 | `src/components/CanvasNode.vue` | `:731-889` | 加内容渲染块：**状态徽章 + 当前步骤 + 进度** |
| 6 | `src/components/InfiniteCanvas.vue` | `:16-25` `panelMap` | 加 `shotlist: ShotlistNodePanel` |
| 7 | `src/components/NodeLayerPanel.vue` | `:14-24` | `NODE_LABELS` + `NODE_ICONS` 各加一项 |
| 8 | `src/composables/useNodes.ts` | `:96-106` `defaults` | 加 `shotlist: { w: 640, h: 420, color: '#...' }` |

**选改 3 处**：

| # | 文件 | 说明 |
|---|---|---|
| 9 | `InfiniteCanvas.vue:76-83` `connectMenuTypes` | 让"拉线到空白处"也能建分镜台 |
| 10 | `InfiniteCanvas.vue:597-609` | 通用面板的排除名单 `text/asset/table/image/video`——`shotlist` 不在其中，走通用路径即可，**无需常驻**（流程节点单实例，不必像 image/video 那样 v-for 常驻） |
| 11 | `InfiniteCanvas.vue:173-208` `nodeInputs` | **必须加分支**，否则下游拿到的是原始 content 字符串。分镜台作为上游时输出 `{state, segments:[{id,title,prompt}], script}` |

**新建文件**：

```
src/components/panels/ShotlistNodePanel.vue      主面板（Gate 驱动的向导，建议拆子组件）
src/components/shotlist/GateCard.vue             Gate 确认卡片
src/components/shotlist/ScriptLockList.vue       EV/DLG/LOCK 清单
src/components/shotlist/AssetMappingList.vue     资产勾选 + 参考/不参考边界
src/components/shotlist/SegmentPlanTable.vue     工作段表 + 文字分镜表
src/components/shotlist/StoryboardReview.vue     宫格图审查
src/components/shotlist/PromptViewer.vue         提示词查看 + 校验结果
src/components/shotlist/SpatialPreview.vue       俯视调度 SVG
src/composables/useShotlistRunner.ts             步骤执行器（状态机推进）
src/composables/useSkillPack.ts                  技能资源元数据获取
```

> 遵循 AGENTS.md：单文件 ≤ 300 行、一个文件一个职责。`ShotlistNodePanel.vue` 必然超，所以拆子组件。

### 9.2 后端（`backend/`）

| 文件 | 动作 |
|---|---|
| `database.go:113` | `types` 数组加 `"shotlist"`（否则管理台看不到、前端 `enabled` 过滤会挡掉） |
| `seed.json` | 加 `node_configs.shotlist`：`extra_config.models[]` 给可选模型，`extra_config.roles[]` 可承载"风格档"（如「写实电影」「动画」） |
| `skill_pack.go` | **新建**：`go:embed skills/shotlist/**` + 步骤→资源映射 + 按需取用 |
| `shotlist_prompt.go` | **新建**：各步骤 system/user prompt 组装 |
| `shotlist_validate.go` | **新建**：Python 校验脚本的 Go 移植 |
| `shotlist_handlers.go` | **新建**：`/api/skills/shotlist/*` 四个端点 |
| `router.go` | 注册 `/api/skills` 路由组 |
| `skills/shotlist/` | **新建目录**：从 `~/.workbuddy-ai/skills/deepwhite-director-shooting-v3-7/` 拷贝 `SKILL.md` + `reference/` + `templates/` |

### 9.3 管理台（`frontend-admin/`）

| 文件 | 动作 |
|---|---|
| `src/App.vue:60,67` | 侧栏类型数组 + 中文名映射加 `shotlist` |
| `src/pages/Dashboard.vue:17` | `ALL_NODES` 加一项 |
| `src/main.ts:26-69` | 加路由 |
| `src/pages/ShotlistNodeConfig.vue` | **新建**，可复用 `SimpleNodeConfig.vue`（参考 `DirectorNodeConfig.vue`） |

---

## 10. 分期实施

### P0 — 打通主链路（MVP，不含故事板）

- [ ] 后端：`skill_pack.go` 嵌入知识库 + `/api/skills/shotlist/prompt`
- [ ] 后端：`database.go` + `seed.json` 注册类型
- [ ] 前端：8 处必改 + `ShotlistNodePanel.vue` 骨架
- [ ] 流程：`SOURCE_READY → SCRIPT_LOCKED → ASSET_CONFIRMED → MODE_CONFIRMED → PLAN_CONFIRMED → PROMPTS_VALIDATED`
- [ ] 分支：只做 `5A 不使用故事板`（技能明确说这条路径"直接进入视频提示词"）
- [ ] 产出：每段提示词落地为 `text` 节点 + 连线

**验收**：拿一段 30 秒中文剧本，能从连线剧本节点走到复制出符合 `PROMPT_PATTERNS.md` 结构的 Seedance 2.5 提示词。

### P1 — 故事板分支 + 校验 + 导出

- [ ] `STORYBOARD_CONFIRMED` 全流程：宫格 prompt → `/api/images/generate` → 落地 `image` 节点 → 审查 UI
- [ ] `shotlist_validate.go` 全部 ERROR 级检查项
- [ ] `/api/skills/shotlist/export` 渲染 `Shotlist_<范围>_Seedance2_5_ZH.md`
- [ ] 空间调度（`SPATIAL_BLOCKING.md`）分支
- [ ] `MODE_CONFIRMED` 按 SEG 独立记录（不继承）

### P2 — 产物闭环 + 视频生成

- [ ] `App.vue` 的 `spawnNode` 通用能力
- [ ] 文字分镜落地为 `table` 节点（复用 ScriptEditorModal）
- [ ] 提示词 → `video` 节点 → 调 `/api/video/generate` 出片
- [ ] 跨段连续性账本 `ledger` 的展示与校验
- [ ] 返工路由：8 条变更 → 状态回退映射

### P3 — 任务引擎（可选）

- [ ] `skill_runs` / `run_steps` 表
- [ ] 后端 run + SSE 事件流
- [ ] 关页面不中断、多端一致

---

## 11. 风险与对策

| 风险 | 说明 | 对策 |
|---|---|---|
| **剧本保真度漂移** | 技能的第一原则是"不擅自改编剧情"，LLM 自由生成极易漂 | 每次 LLM 调用都把 `EV/DLG/LOCK` 清单作为硬约束注入；Go 校验做 DLG 逐字比对，不一致直接 ERROR |
| **知识库 token 成本** | 19 个文件全量注入会爆 context 且稀释注意力 | 严格按 §5.2 映射表按需注入，且**在路由层禁止**非第 6 步加载动态模块 |
| **长流程断点** | 前端编排在关页面时中断 | 每步完成立即落库；从 `content.state` 恢复；`_step` 显示进度 |
| **无 JSON mode** | `llm_handler.go` 不透传 `response_format`，结构化输出不可靠 | ① prompt 里给严格 JSON 模板 + few-shot；② 前端写容错解析（去 markdown 围栏、补全截断）；③ 可选：改造 `normalChat`/`streamChat` 支持 `response_format`（需上游支持） |
| **`parameters` 字段位置非标准** | `llm_handler.go:60` 把 `parameters` 放在 body 顶层，上游大概率忽略 temperature | 第一期不依赖 temperature；若需要，顺手修成平铺 |
| **产物落地动到 App.vue 核心** | `spawnNode` 要改主流程 | 抽象成独立函数，不改现有 image/video 分支；先写单测/手工回归 |
| **命名冲突** | `director` 已被 3D 导演台占用 | 用 `shotlist`，显示名「分镜台」 |
| **校验脚本无法直接跑** | 后端无 Python/shell | 移植成 Go，`scripts/*.py` 只作规则参考保留在技能目录 |
| **技能版本漂移** | 技能升级（v3.7 → v3.8）后后端嵌入的知识库会过期 | `SKILL.md` frontmatter 加版本号，后端启动时打日志提示；提供 `scripts/sync_skill.sh` 一键同步 |

---

## 12. 待你拍板的决策点

1. **流程执行放前端还是后端？** 建议第一期前端（快、可复用现有 SSE），第二期再考虑后端任务引擎。
2. **产物是否落地为真实节点？** 建议是——这是选方案 B 的意义所在。若你只想要"一个能出提示词的面板"，方案 A 更省事。
3. **故事板宫格图用哪个模型？** 画布现有 GeekNow 系列（`gpt-image-2` 系、`doubao-seedream` 系）能否满足"火柴人简笔画多宫格"的稳定性？需要实测。
4. **是否需要"一画布一项目"隔离？** `canvases.project_type` 已预留 `'workflow'` 值但前端完全未使用；如果分镜项目要独立管理，可以启用它。
5. **是否接受"技能知识库内嵌进后端"？** 这会让技能与产品耦合。替代方案是后端只存一份"资源清单"，运行时从磁盘读——但发布版是单 exe 零依赖，`go:embed` 是唯一稳妥解。

---

## 附录 A：技能结构速查

```
~/.workbuddy-ai/skills/deepwhite-director-shooting-v3-7/
├── SKILL.md                          流程骨架（9 状态 + Gate + 返工路由 + 资源路由表）
├── reference/                        19 个知识文件，148K
│   ├── SCRIPT_FIDELITY.md            剧本保真（所有任务必读）
│   ├── ASSET_CONFIRMATION.md         资产策略
│   ├── SPATIAL_BLOCKING.md           俯视调度
│   ├── SEEDANCE_2_5_RULES.md         模型限制
│   ├── PLANNING_CAPACITY.md          容量规划
│   ├── STORYBOARD_ASSETS.md          故事板
│   ├── PROMPT_PATTERNS.md            提示词模板（核心）
│   ├── DRAMA_SHOT_MOMENTUM.md        文戏镜头动势
│   ├── MOTION_MODE_ROUTER.md         ┐
│   ├── DYNAMIC_MOTION_PHYSICS.md     │ 动态模块
│   ├── DYNAMIC_PLANNING.md           │ 仅第 6 步可加载
│   ├── DYNAMIC_PROMPT_CONTRACT.md    │
│   ├── DYNAMIC_SPATIAL_LOCK.md       │
│   ├── DYNAMIC_DIAGNOSIS_QC.md       ┘
│   ├── STYLE_BLOCK.md / MICRO_BEATS.md / CAMERA_EMOTION.md
│   ├── DELIVERY_FORMAT.md / HTML_QA.md / PLAN_TYPES.md
├── scripts/
│   ├── validate_delivery.py          交付校验（需移植为 Go）
│   ├── validate_dynamic_prompt.py    动态提示词校验
│   └── test_validate_delivery.py
├── templates/HTML_TEMPLATE.md
└── agents/openai.yaml
```

## 附录 B：画布现有节点类型（避免命名冲突）

`text` 文本 / `image` 图片 / `video` 视频 / `table` 剧本 / `full_image` 全图 /
`agent` 智能体 / `workflow` 工作流(ComfyUI) / `asset` 资产 / `director` 3D 导演台
