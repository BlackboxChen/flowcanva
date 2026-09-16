# 画布 Agent 对话（Canvas Copilot）集成方案

> 目标：在 FlowCanva 画布里加一个**对话式 Agent**，用户用自然语言指挥它操作画布——
> 建节点、连线、写提示词、跑图片/视频生成、整理布局、定位视图。
>
> 版本：v1 草案 ｜ 面向 FlowCanva（Go + Vue3 + Three.js）

---

## 0. 一句话结论

**Agent 循环跑在前端，后端只做「LLM 转发 + 生成能力代理」。**

理由只有一个但足够硬：**画布状态的唯一真相源在前端**。
`useNodes` / `useEdges` 的 ref 持有 nodes/edges，`viewport` 是前端 reactive；
后端只有一份滞后的 DB 快照。所有写操作必须走前端 composable 才能同时更新本地状态与数据库
（`addNode` 既 push 到 `nodes.value` 又调 `createNode`）。撤销栈 `useHistory` 也在前端。

所以架构是：**画布是编排层，后端是能力层**——和分镜台方案同一套哲学。

```
用户输入
   ↓
[前端] useCanvasAgent —— Agent 循环
   ├─ 组装画布上下文（分层摘要）
   ├─ 调 /api/llm/chat（带 tools）
   ├─ 解析 tool_calls
   ├─ 批量校验 → 单次 pushHistory → 顺序执行
   └─ 工具结果回填 → 再调 LLM（最多 N 轮）
        ↓
[前端] useCanvasTools —— 工具执行器
   ├─ 写操作 → useNodes / useEdges / useCanvas（已有 API）
   └─ 生成类 → POST /api/images/generate、/api/video/generate（已有端点）
```

---

## 1. 三个必须先定的判断

### 1.1 执行器放前端，不放后端

| 维度 | 前端执行 | 后端执行 |
|---|---|---|
| 状态一致性 | ✅ 本地 ref + DB 同步一步到位 | ❌ 后端写库后前端不知道，除非重拉全量 |
| 撤销栈 | ✅ 直接 `pushHistory()` | ❌ 撤销栈在前端，后端够不着 |
| 生成任务编排 | ✅ 复用面板已有的 `_loading` / 轮询 / AbortController | ❌ 割裂成两套 |
| 上下文准确性 | ✅ 读的是实时内存状态 | ⚠️ 读的是滞后 DB 快照 |
| 关页面续跑 | ❌ 中断 | ✅ 可续跑 |

最后一行是后端唯一的优势，但**代价太大**：要新建 task 表 + worker + SSE，且后端目前所有生成都是请求内同步阻塞（最长 30 分钟占连接）。
**第一期前端，第二期再评估是否需要后端化。**

### 1.2 工具调用协议：原生优先，文本协议兜底

后端 `llm_handler.go` 目前**不支持 tools**——`ChatRequest` 只有 model/messages/stream/channel/base_url/api_key/parameters，
`normalChat` 与 `streamChat` 构造 bodyMap 时也没有 `tools` / `tool_choice`。

两条路：

**A（首选）后端透传原生 function calling**
`ChatRequest` 加两个字段，bodyMap 透传即可，改动不到 10 行：

```go
type ChatRequest struct {
    // ...现有字段
    Tools      any `json:"tools,omitempty"`
    ToolChoice any `json:"tool_choice,omitempty"`
}
```

风险：**依赖上游网关支持**。项目默认走 `ZHENGAPI_*` 中转，中转站不一定透传 `tools`。
上线前必须实测：发一个带 tools 的请求，看返回里有没有 `tool_calls`。

**B（兜底）结构化文本协议**
不依赖上游，要求模型输出：

````
```action
{"tool": "node.create", "args": {"type": "image", "x": 100, "y": 200}}
```
````

前端解析代码块。兼容性 100%，但模型可能写坏 JSON、可能夹带解释文字。

**建议：A 为主，启动时探测一次，失败则自动降级到 B。** 把降级开关做成管理台配置项。

### 1.3 流式与工具调用的冲突

`streamChat` 只转发 `data:` 行，而工具调用的增量是 `delta.tool_calls[]` 分片
（需要按 `index` 累积拼接 `arguments` 字符串）。
前端 `chatWithLLMStream` 只取 `delta.content`（`api/index.ts:162`）——**tool_calls 会被直接丢掉**。

**取舍：Agent 的"决策轮"用非流式，"表达轮"用流式。**

- 决策轮：调 `chatWithLLM`（非流式），一次拿到完整 `tool_calls`，好解析
- 表达轮：工具执行完后，再调一次流式，让用户看到 Agent"说话"

非流式对决策轮完全够用——反正用户真正想看的是"Agent 在画布上做了什么"，不是它思考的 token。

---

## 2. 工具集设计（画布动作面）

工具全部映射到**已存在的函数**，不新造轮子：

### 2.1 只读工具

| 工具 | 底层 | 说明 |
|---|---|---|
| `canvas.get_state` | 读 `nodes` / `edges` / `viewport` | 返回精简摘要；可传 `node_id` 拿单个节点完整 content |
| `canvas.find_nodes` | 过滤 `nodes.value` | 按类型/关键词/位置范围查找，返回 id 列表 |

### 2.2 写工具

| 工具 | 底层调用 | 位置 |
|---|---|---|
| `node.create` | `addNode(type, wx, wy)` | `useNodes.ts:90` |
| `node.update_content` | `updateNodeContent(id, content)` | `useNodes.ts:145` |
| `node.update_config` | `updateNodeConfig(id, config)` | `useNodes.ts:154` |
| `node.move` | `moveNode(id, x, y)` | `useNodes.ts:123` |
| `node.resize` | `resizeNode(id, w, h, x, y)` | `useNodes.ts:133` |
| `node.delete` | `removeNode(id)` + `removeNodeEdges(id)` | `useNodes.ts:161` / `useEdges.ts:55` |
| `edge.create` | `addEdge(src, tgt)` | `useEdges.ts:14`（已含自连/重复/环路校验） |
| `edge.delete` | `removeEdge(edgeId)` | `useEdges.ts:49` |
| `asset.create` | `addNode('asset')` + 写 content | 参考 `App.vue:399` 的截图建资产 |

### 2.3 视图工具（让用户"看见" Agent 在干活）

| 工具 | 底层 | 说明 |
|---|---|---|
| `view.focus_node` | `focusOn(x, y, w, h)` | `useCanvas.ts:157`，自动算合适缩放 |
| `view.navigate` | `navigateTo(wx, wy)` | `useCanvas.ts:151` |
| `view.reset` | `resetView()` | |
| `view.select` | `selectNode(id)` | |
| `canvas.auto_arrange` | `autoArrange()` | `App.vue:245`，已有 Sugiyama 分层布局 |

**视图工具不是装饰**。Agent 改了画布却不动视图，用户会一脸茫然。建议规则：
**Agent 每完成一批写操作，自动 `focus_node` 到最相关的那个节点。**

### 2.4 生成类工具（⚠️ 涉及真实扣费）

| 工具 | 底层 | 说明 |
|---|---|---|
| `node.generate_image` | `POST /api/images/generate` | 参考 `ImageNodePanel.generate()` |
| `node.generate_video` | `POST /api/video/generate` | 参考 `VideoNodePanel.generate()` |

**这两类必须走确认流程。** 项目 `AGENTS.md` 明确写了：

> API 接口操作：非用户明确要求的扣款行为（如大量 API 调用、购买资源等）一律禁止

所以设计上：**生成类工具在 UI 上渲染成带「执行 / 取消」按钮的确认卡片，Agent 无权自动执行**。
可选给用户一个「允许自动生成」开关，但默认关闭，且单次会话有次数上限。

---

## 3. 核心新增抽象：ActionBatch（批量操作 + 单次撤销）

这是本方案**最重要的一个工程点**，不解决会出大问题。

### 3.1 问题

现有代码里**每个 handler 各自 `pushHistory()`**：

```ts
function handleMoveNode(id, x, y) { pushHistory(); moveNode(id, x, y) }
function handleCreateEdge(a, b)    { pushHistory(); addEdge(a, b) }
function handleDeleteNode(id)      { pushHistory(); removeNode(id); removeNodeEdges(id) }
```

用户手动操作时，一次点击 = 一次 push，撤销栈是干净的。

但 Agent 一次回复可能产生 **5~10 个工具调用**。如果逐个 push：
- 用户按一次 Ctrl+Z 只撤销了 1/10 个操作，要按 10 次
- `MAX_HISTORY_ENTRIES = 30`（`useHistory.ts:9`）会被瞬间灌满，**把用户之前的手动操作历史挤出去**

### 3.2 设计

```ts
// composables/useActionBatch.ts
export function useActionBatch(pushHistory: () => void) {
  let depth = 0

  /** 批处理期间抑制自动 pushHistory，由批次统一提交一次 */
  async function run<T>(actions: (() => Promise<any>)[], opts?: { silent?: boolean }): Promise<T[]> {
    if (depth > 0) return Promise.all(actions.map(a => a()))   // 嵌套批次直接执行
    depth++
    if (!opts?.silent) pushHistory()
    try {
      const results = []
      for (const a of actions) results.push(await a())          // 顺序执行，避免并发写库竞态
      return results
    } finally {
      depth--
    }
  }

  return { run, isBatching: () => depth > 0 }
}
```

配套改动：把 `App.vue` 里所有 handler 的 `pushHistory()` 换成 `batch.run()` 包裹，
并在 `useNodes` / `useEdges` 的写函数里加"批处理中不 push"的判断。

**更彻底的做法（第二期）**：把 undo/redo 从"快照替换"改成"逆操作执行"。
因为现在 `useHistory.undo()` 有个**已有缺陷**（见 §7.1），Agent 场景会放大它。

---

## 4. 画布上下文注入（防止上下文爆炸）

画布可能有几百个节点。全量塞进 prompt 会爆 token 且稀释注意力。**分层摘要**：

### 4.1 默认注入（L1 概览）

紧凑的一行一节点格式，不是 JSON（省 token）：

```
画布：旧公寓分镜 ｜ 节点 24 个，连线 19 条 ｜ 视口 (0,0) zoom 1.0 ｜ 当前选中：nd_a1b2
nd_a1b2|image|(120,340,400x300)|"赛博朋克女孩，霓虹灯…"|已生成1张
nd_c3d4|text |(560,340,480x270)|"第一幕：雨夜…"|空
nd_e5f6|table|(120,700,960x540)|剧本 8行×15列
nd_g7h8|asset|(1080,340,320x300)|role_01.png
...
```

摘要生成规则：

| 类型 | 摘要内容 |
|---|---|
| `text` | content 前 100 字 |
| `image` | `prompt` 前 80 字 + 已生成张数 |
| `video` | `prompt` 前 80 字 + 视频数 + 是否 loading |
| `table` | 行列数 |
| `asset` | 文件名 |
| `agent` | `---` 分隔符前的提示词部分 |
| `workflow` | 工作流名 + 输出数 |
| `director` | 场景名 + 角色数 |
| `shotlist` | 当前状态（若已实现分镜台方案） |

### 4.2 裁剪策略

- 节点数 ≤ 60：全量给 L1
- 节点数 > 60：只给**视口内 + 选中节点相邻（上下游 1 跳）**的节点，其余给一句"另有 N 个节点在视口外，可用 canvas.get_state 查询"
- 选中节点：给**完整 content**（截断到 4000 字符）
- 边：只给 `source→target` 的 id 对，不带其他信息

### 4.3 主动查询

给 Agent `canvas.get_state` / `canvas.find_nodes` 两个工具，让它按需拉细节。
比一次性塞满上下文更省也更准。

---

## 5. Agent 循环

```
1. 用户输入 "把这张图做成视频，再连一个提示词节点"
2. 组装 messages：
   [system: 角色设定 + 工具说明 + 操作规范]
   [system: 画布当前状态摘要]
   [user: 用户输入]
   [历史消息...]
3. POST /api/llm/chat（非流式，带 tools）
4. 若返回 tool_calls：
   a. 校验每个调用（节点存在？类型合法？会不会成环？参数类型对？）
   b. 校验失败 → 不执行，把错误作为 tool result 回填，让模型自己纠正
   c. 需要确认的（生成类/批量删除）→ 挂起，等用户点确认
   d. batch.run(actions) —— 单次 pushHistory + 顺序执行
   e. 每个工具的返回值转成 role:"tool" 消息回填
   f. 回到 3，最多 N 轮（建议 8）
5. 若返回纯文本：
   → 流式重新生成一遍"给用户看的话"（或者直接用这次的非流式结果）
6. 收尾：自动 focus 到最相关节点
```

### 5.1 System Prompt 要点

```
你是 FlowCanva 画布的编排助手。你能通过工具读写画布。

铁律：
1. 不确定节点 id 时，先调 canvas.get_state 或 canvas.find_nodes，不要猜。
2. 生成图片/视频会产生真实费用，工具会要求用户确认，你不要反复重试。
3. 一次回复尽量少而准地调用工具；不要为了"看起来做了很多"而建无用节点。
4. 连线前确认两个节点都存在且方向正确（数据从上游流向下游）。
5. 操作完用一句话告诉用户你改了什么，不要罗列工具名。
6. 你不能修改用户已有的节点内容，除非用户明确要求。
```

### 5.2 防死循环

- 轮次上限 8
- 连续 2 轮工具全部失败 → 停止，把错误告诉用户
- 同一个工具 + 同样参数重复调用 → 拦截

---

## 6. UI 形态

### 6.1 入口

- **顶部栏**加一个「AI 助手」按钮（`App.vue:646-654` 那一排）
- 快捷键 `Ctrl/Cmd + K`
- 首次进入画布时给一次气泡提示

### 6.2 面板

**右侧抽屉式**（不是模态框），理由：
- 不遮挡画布——Agent 操作时用户要能看见画布变化
- 能常驻显示工具调用过程
- 宽度 380px，可折叠成一条窄边

```
┌──────────────────────────────┐
│  AI 助手              ─  ×  │
├──────────────────────────────┤
│  👤 把这张图做成视频          │
│                              │
│  🤖 好的，我先建一个视频节点   │
│     ┌──────────────────────┐ │
│     │ ⚙ node.create        │ │
│     │   视频节点 @ (1080,340)│ │
│     │   ✓ 已创建 nd_x9y8    │ │
│     └──────────────────────┘ │
│     ┌──────────────────────┐ │
│     │ ⚙ edge.create        │ │
│     │   nd_a1b2 → nd_x9y8   │ │
│     │   ✓ 已连线            │ │
│     └──────────────────────┘ │
│     ┌──────────────────────┐ │
│     │ ⚠ node.generate_video │ │
│     │   预计消耗 1 次额度    │ │
│     │   [ 执行 ]  [ 取消 ]  │ │
│     └──────────────────────┘ │
├──────────────────────────────┤
│  [ 输入框…              ] ↑  │
└──────────────────────────────┘
```

### 6.3 工具调用卡片（关键交互）

每张卡片显示：工具图标 + 人类可读的动作描述 + 状态（待执行/执行中/成功/失败/待确认）。

**点击卡片 → `focus_node` 定位到相关节点。** 这是让用户建立信任的关键——
Agent 说"我建了个节点"，用户点一下就能看到它在哪。

失败卡片显示具体原因（"目标节点不存在"、"该连线会形成环路"），并提供「重试」按钮。

---

## 7. 顺带发现的现有缺陷（建议一并修）

### 7.1 `useHistory` 的 undo/redo 不回写数据库 ⚠️

`useHistory.ts:40-42` 与 `:50-52`：

```ts
nodes.value = snapshot.nodes
edges.value = snapshot.edges
```

**只替换了前端数组，没有调用任何 API。** 后果：撤销后刷新页面，被撤销的改动会"复活"。
Agent 批量操作会大幅放大这个问题（一次撤销涉及 10 个节点）。

**两种修法**：
- **轻**：undo/redo 后调一个 `POST /api/canvases/:cid/sync` 全量覆盖端点（后端事务内 delete + insert）
- **重**：改成逆操作模型，每批操作记录 inverse ops，undo 时执行逆操作并走正常 API

建议第一期用"轻"方案，稳定后再考虑重构。

### 7.2 `llm_handler.go` 的 `parameters` 位置非标准

`llm_handler.go:60-62` 把 `parameters` 放在 body 顶层：

```go
bodyMap["parameters"] = req.Parameters
```

OpenAI 标准是 `temperature` / `max_tokens` 平铺。上游大概率忽略这个字段。
Agent 场景需要控制 `temperature`（决策轮要低），顺手修成平铺更好。

### 7.3 `removeNodeEdges` 只改本地

`useEdges.ts:55-59` 只 filter 本地数组。好在 `edges` 表对 `nodes` 有 `ON DELETE CASCADE`
（`database.go:100-101`），删节点时后端会级联删边，所以本地 filter 是对的。
**但要注意**：Agent 若只调 `removeNodeEdges` 而不删节点，边不会真的消失。工具实现要成对调用。

---

## 8. 改动清单

### 8.1 前端新建

```
src/components/ChatDrawer.vue             对话抽屉（入口 + 布局）
src/components/chat/MessageBubble.vue     消息气泡（Markdown 渲染，复用 marked）
src/components/chat/ToolCallCard.vue      工具调用卡片（状态 + 确认按钮 + 定位）
src/components/chat/ContextChip.vue       上下文提示（"已读取 24 个节点"）
src/composables/useCanvasAgent.ts         Agent 循环（决策轮/执行轮/确认挂起）
src/composables/useCanvasTools.ts         工具定义 + 执行器（映射到已有函数）
src/composables/useCanvasContext.ts       画布状态摘要生成 + 裁剪
src/composables/useActionBatch.ts         批量操作 + 单次 pushHistory
src/composables/useAgentSession.ts        会话状态（第一期内存，第二期落库）
```

> 遵循 `AGENTS.md`：单文件 ≤ 300 行、一个文件一个职责。`useCanvasTools.ts` 按只读/写/视图/生成拆成 4 个文件。

### 8.2 前端修改

| 文件 | 改动 |
|---|---|
| `App.vue` | 顶部栏加 AI 助手按钮；挂载 `ChatDrawer`；所有 handler 的 `pushHistory()` 换成 `batch.run()`；`Ctrl+K` 快捷键 |
| `api/index.ts` | 加 `chatWithLLMWithTools(model, messages, tools, modelConfig)`（非流式） |
| `types/index.ts` | 加 `ToolCall` / `ToolResult` / `AgentMessage` 类型 |

### 8.3 后端修改

| 文件 | 改动 |
|---|---|
| `llm_handler.go` | `ChatRequest` 加 `Tools` / `ToolChoice` 字段；`normalChat` 与 `streamChat` 的 bodyMap 透传；顺手修 `parameters` 平铺 |
| `router.go` | 若加会话持久化：注册 `/api/agent/*` 路由 |

### 8.4 后端新建（第二期）

```
backend/agent_handlers.go    会话 CRUD
backend/agent_session.go     agent_sessions / agent_messages 表模型
```

新表：

```sql
CREATE TABLE IF NOT EXISTS agent_sessions (
  id         TEXT PRIMARY KEY,
  canvas_id  TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
  title      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);

CREATE TABLE IF NOT EXISTS agent_messages (
  id         TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
  role       TEXT NOT NULL,          -- user | assistant | tool
  content    TEXT NOT NULL DEFAULT '',
  tool_calls TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
```

`database.go` 的 `migrate()` 里追加即可（走已有的 `CREATE TABLE IF NOT EXISTS` 切片）。

### 8.5 管理台

| 文件 | 改动 |
|---|---|
| `frontend-admin/src/pages/AgentNodeConfig.vue` | 新建或复用 `SimpleNodeConfig`，配置 Agent 用的模型/渠道/是否启用工具调用降级 |

> 注意：Agent 不是画布节点，但可以复用 `node_configs` 表存配置——
> 加一条 `node_type = 'canvas_agent'` 的记录，不进 `database.go:113` 的节点类型数组即可（避免出现在左侧工具栏）。

---

## 9. 分期实施

### P0 — 只读 + 基础写（打通闭环）

- [ ] `llm_handler.go` 透传 `tools`，**实测上游是否支持**（这是 Go/No-Go 点）
- [ ] `useCanvasContext.ts` 分层摘要
- [ ] `useCanvasTools.ts`：`canvas.get_state` / `node.create` / `node.move` / `node.update_content` / `edge.create` / `view.focus_node`
- [ ] `useActionBatch.ts` + `App.vue` handler 改造
- [ ] `ChatDrawer.vue` + `ToolCallCard.vue`
- [ ] `useCanvasAgent.ts` 主循环（不含确认流程）

**验收**：说"在画布上建一个文本节点，写一段雨夜场景的描述，再建个图片节点连起来"，能正确完成且一次 Ctrl+Z 全部撤销。

### P1 — 生成类 + 确认机制

- [ ] `node.generate_image` / `node.generate_video` 工具
- [ ] 确认卡片 UI + 挂起/恢复机制
- [ ] 「允许自动生成」开关 + 单次会话次数上限
- [ ] 失败重试、错误回填给模型自我纠正

### P2 — 会话持久化 + 体验打磨

- [ ] `agent_sessions` / `agent_messages` 表 + `/api/agent/*` 端点
- [ ] 多会话、历史回溯
- [ ] 修 §7.1 的 undo 不回写 DB 缺陷
- [ ] 上下文裁剪策略调优（大画布实测）

### P3 — 与分镜台联动

- [ ] 给 `shotlist` 节点暴露可编程接口（`shotlist.start` / `shotlist.confirm_gate` / `shotlist.export`）
- [ ] Agent 能说"帮我建个分镜台，把剧本连进去，跑规划阶段"

---

## 10. 风险与对策

| 风险 | 说明 | 对策 |
|---|---|---|
| **上游不支持 tool calling** | 默认走 ZHENGAPI 中转，不一定透传 `tools` | P0 第一件事就是实测；准备文本协议降级方案（§1.2 B） |
| **流式丢 tool_calls** | `chatWithLLMStream` 只取 `delta.content`（`api/index.ts:162`） | 决策轮用非流式；不要试图在流式里拼 tool_calls |
| **撤销栈被灌爆** | `MAX_HISTORY_ENTRIES=30`，Agent 一次 10 个操作就挤掉用户历史 | `ActionBatch` 单次 push（§3） |
| **真实扣费** | 生成类工具会产生费用 | 强制人工确认，默认关闭自动生成（`AGENTS.md` 硬性要求） |
| **上下文爆炸** | 大画布几百节点 | 分层摘要 + 视口裁剪 + `get_state` 按需拉取（§4） |
| **undo 不回写 DB** | 现有缺陷，Agent 场景放大 | §7.1，第一期用全量 sync 端点兜底 |
| **Agent 乱建节点** | 模型为了"表现"堆无用节点 | system prompt 约束 + UI 上卡片可逐条取消 + 一键撤销整批 |
| **环路连线** | `addEdge` 已有 BFS 校验会静默 return | 工具包装层要捕获"没返回 edge 对象"= 失败，把原因回填给模型 |
| **并发写库竞态** | 10 个 `createNode` 并发 | `batch.run` 内**顺序执行**，不 `Promise.all` |
| **误删** | Agent 批量删节点 | 删除类操作一律走确认卡片；单批删除数 > 3 时额外警告 |

---

## 11. 待你拍板的决策点

1. **是否接受"Agent 循环在前端"？** 代价是关页面中断，收益是状态一致 + 撤销可用。若必须关页面续跑，得先建后端任务引擎（成本高）。
2. **上游到底支不支持 tools？** 这是 Go/No-Go。建议我先发一个探测请求实测，再决定是走原生还是文本协议。
3. **生成类工具要不要"允许自动生成"开关？** 我倾向默认关闭——`AGENTS.md` 明确禁止非用户要求的扣款行为。
4. **Agent 能不能改用户已有节点的内容？** 我倾向默认不能（只能新建和自己刚建的），需要用户显式授权。
5. **要不要顺手修 `useHistory` 的 undo 不回写 DB？** 这是既有 bug，但独立于本功能，可以单开一轮。

---

## 附录 A：画布可编程动作面速查（来自现有代码）

```
useNodes()    addNode / moveNode / resizeNode / updateNodeContent / updateNodeConfig
              removeNode / selectNode / expireStaleVideoTasks
useEdges()    addEdge（含自连/重复/环路 BFS 校验）/ removeEdge / removeNodeEdges
useCanvas()   screenToWorld / worldToScreen / zoomAt / resetView / zoomIn / zoomOut
              navigateTo / focusOn / restoreViewport
useHistory()  push / undo / redo / canUndo / canRedo
App.vue       autoArrange（Sugiyama 分层）/ handleGridSplit / handleSavePanelForNode
              handleCreateAssetFromScreenshot / pasteCopiedNode
API           /api/llm/chat  /api/images/generate  /api/video/generate
              /api/canvases/:cid/nodes  /api/canvases/:cid/edges
              /api/admin/assets/upload  /api/admin/presets
```

## 附录 B：与分镜台方案的关系

| | 分镜台（shotlist 节点） | 画布 Agent（本方案） |
|---|---|---|
| 形态 | **固定工作流**，9 状态 + 人工 Gate | **自由编排**，自然语言驱动 |
| 适合 | 有既定方法论的生产流程 | 零散、探索性的画布操作 |
| 状态存哪 | `nodes.content`（可断点续跑） | 前端内存（第一期） |
| 确定性 | 高，步骤固定 | 低，模型自由发挥 |
| 联动 | Agent 可作为分镜台的"操作手"（P3） | — |

两者互补，不冲突。**共用的基础设施**：`useActionBatch`、画布状态摘要、`llm_handler` 的 tools 透传。
建议先做 Agent 的 P0（基础设施），分镜台的产物落地也能直接受益。
