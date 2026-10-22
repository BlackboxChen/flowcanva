# FlowCanva — 无限画布 AI 创作平台

基于 **无限画布 + 可视化节点 + AI 管线编排** 的全栈创作工具。像搭积木一样串联 AI 能力，从创意到产出，一条流程走完。

支持 LLM 对话、AI 图片生成、AI 视频生成、ComfyUI 无缝集成、智能体编排、工作流自动化等多种节点类型，**一个平台打通所有 AI 生成式能力**。

## 📸 界面预览

| 画布 | 后台仪表盘 |
|------|------------|
| ![画布](png/画布.png) | ![仪表盘](png/仪表盘.png) |

| 节点配置 | 素材资产 |
|----------|----------|
| ![节点](png/节点.png) | ![素材](png/素材资产.png) |

| 画板项目管理 | 导演台 |
|--------------|--------|
| ![画板](png/画板.png) | ![导演台](png/导演台.png) |

> 画布实际使用效果

![画布效果](png/画布2.png)

---

## ✨ 核心亮点

### 🎨 无限画布，自由拖拽
- 基于 **Three.js 3D 渲染引擎**，丝滑缩放平移
- 节点自由拖拽、连线编排，所见即所得
- 支持小地图导航、撤销/重做、历史记录
- 暗色主题，极简交互，沉浸式创作体验

### 🤖 ComfyUI 无缝集成
- **一键对接本地/远程 ComfyUI 服务**，后端自动代理 prompt 提交
- 实时轮询获取 ComfyUI 执行结果（图片 / GIF / 文本）
- 图片代理查看，无需暴露 ComfyUI 端口
- 支持自定义 workflow JSON，兼容所有 ComfyUI 工作流

### 🧠 多模型 LLM 管线
- 支持 **OpenAI / DeepSeek / Claude / 自定义渠道** 等多模型切换
- 内置 6 种文本角色（通用/歌词/翻译/总结/代码/写作），开箱即用
- 每个节点可独立配置模型、API Key、base_url、temperature 等参数
- 节点间串联：上一个 LLM 输出 → 下一个节点输入，构建 AI 流水线

### 🖼️ 多模态生成
- **图片生成**：支持 OpenAI 兼容通道 + ComfyUI 后端 + 本地模型
- **视频生成**：对接 ComfyUI 视频工作流
- 生成结果自动存入画布节点，支持预览、下载、分享
- 内置 **SM.MS 图床**（免费无需注册，上传后得到公网 URL）一键托管上传

### 📦 多节点生态

| 节点类型 | 能力 |
|---------|------|
| 📝 文本节点 | Markdown 渲染 + 编辑 + LLM 生成 |
| 🖼️ 图片节点 | AI 生成 / 本地上传 / SM.MS 图床托管 / URL 链接 / 预览 / 下载 / 宫格裁切 |
| 🎬 视频节点 | 生成 / 播放 / 下载 |
| 📋 剧本节点 | 分镜剧本表格编辑,支持行列插入与表格粘贴 |
| 🎬 导演台 | 3D 场景编排,角色/道具布局 (Three.js) |
| 🖼️ 全屏图片 | 高清大图沉浸预览 |
| 🌐 360° 全景 | 基于 Pannellum 的实景漫游 |
| 🤖 智能体节点 | 多轮对话 + 角色扮演 + 工具调用 |
| ⚙️ 工作流节点 | 跨画布引用，模块化编排 |
| 📎 资产节点 | 本地文件上传 / Base64 直连 / URL 链接 / SM.MS 图床托管 |

### 🚀 零依赖分发（用户端）
- **一个 exe，双击即用** — 无需安装 Go、Node.js、Python
- 前后端编译合一，`go:embed` 嵌入静态资源
- 嵌入式 **SQLite** 数据库，零配置启动
- 自动创建 `data.db` + `uploads/` 目录，数据自包含

### 🔧 开发者友好
- **前后端分离架构**，开发模式支持热重载
- 后端代理 Vite dev server，联调体验丝滑
- RESTful API 设计，所有端点 `/api/` 前缀统一
- 节点配置页面化，管理员面板可动态调整模型参数

---

## 🚀 快速开始（用户）

1. 从 [Releases](./releases) 下载最新 `flowcanva.zip`
2. 解压到任意目录
3. 双击 `flowcanva.exe`
4. 浏览器自动打开 `http://localhost:6789`

> 无需安装 Go / Node.js / Python，一个 exe 全部搞定。

## 🔧 开发模式

```powershell
# 需要 Go 1.23+ 和 Node.js 18+
.\start-dev.ps1
```

| 服务 | 端口 |
|------|------|
| 统一入口 (后端) | `:6789` |
| 管理台 (Vite HMR) | `:5174` |
| 画布 (Vite HMR) | `:5173` |

## 📦 一键发布构建

```powershell
.\build-release.bat
```

构建产物在 `release/flowcanva/`，双击 `flowcanva.exe` 即可启动。

## 📁 项目结构

```
flowcanva/
├── backend/                # Go 后端 (Gin + SQLite)
│   ├── main.go             # 入口
│   ├── router.go           # 路由 + 静态文件服务
│   ├── embed.go            # go:embed 前端构建产物
│   ├── comfyui_handlers.go # ComfyUI 代理与结果提取
│   ├── llm_handler.go      # LLM 多模型对话代理
│   ├── image_handlers.go   # 图片生成
│   ├── video_handlers.go   # 视频生成
│   ├── asset_handlers.go   # 资产管理
│   ├── seed.json           # 节点预设 (模型/角色配置)
│   ├── admin-dist/         # [构建时] 管理台构建产物
│   └── canvas-dist/        # [构建时] 画布构建产物
├── frontend-admin/         # 管理台前端 (Vue 3 + daisyUI)
├── frontend-canvas/        # 画布前端 (Vue 3 + Three.js)
├── build-release.bat       # 一键发布构建
├── start-dev.ps1            # 开发模式启动
```

## 🔌 API 架构

```
:6789/api/
├── /canvases/*              # 画布 CRUD
├── /canvases/:cid/nodes/*   # 节点 CRUD
├── /canvases/:cid/edges/*   # 连线 CRUD
├── /llm/chat                # LLM 对话 (OpenAI 兼容)
├── /images/generate         # AI 图片生成
├── /video/generate          # AI 视频生成
├── /upload-image-host       # SM.MS 图床上传（免费图床，上传后返回公网 URL）
├── /comfyui/execute         # ComfyUI 工作流执行
├── /comfyui/result/:id      # ComfyUI 结果轮询
├── /comfyui/proxy-view      # ComfyUI 图片代理
└── /admin/*                 # 管理台配置 API
```

## 🔑 API Key 配置

每个节点可独立配置不同 API 渠道/Key/模型；文本和视频可通过 `ZHENGAPI_*` 提供全局中转，图片可通过 `GEEKNOW_*` 单独配置。

### 配置方式

启动后访问管理台（`http://localhost:6789`），每个节点类型页面可独立配置：

- **模型**：选择预设或自定义模型名
- **渠道**：OpenAI / DeepSeek / NXFL / 自定义
- **API Key**：该节点专用的密钥
- **Base URL**：API 端点地址
- **参数**：temperature、max_tokens 等

#### 内网 ComfyUI 生图 / 生视频

在“图片节点配置”或“视频节点配置”中添加模型，将调用协议选为“内网 ComfyUI 工作流”，在 Base URL 中填入运行 FlowCanva 后端可访问的 ComfyUI 地址（例如 `http://192.168.1.20:8188`），并粘贴从 ComfyUI 导出的 **API 格式**工作流 JSON。API Key 可留空。保存后，在画布相应节点的模型列表中选择该模型即可。

工作流 JSON 字符串可使用 `{{prompt}}`、`{{width}}`、`{{height}}`、`{{size}}`、`{{seconds}}`、`{{ratio}}`、`{{resolution}}`、`{{image}}`、`{{images}}`、`{{videos}}` 和 `{{audios}}` 占位符；尺寸、数量等占位符会替换成对应 JSON 值。ComfyUI 工作流需将结果输出为图片，或视频 / GIF 输出（例如 VHS 节点的 `gifs` 输出）。图片由 FlowCanva 代理显示，视频会缓存到本地 `uploads/`。

### 配置存储

```
管理台按节点类型配置
    └─→ SQLite node_configs 表（每个节点类型一条记录）
         └─→ 前端请求时携带该节点配置的 api_key
              └─→ 后端按 Key 选择完整配置对
                   ├─→ 节点有 Key：使用节点 Base URL + Key
                   └─→ 节点无 Key：文本/视频使用 ZHENGAPI_*，图片使用 GEEKNOW_*
```

> 节点自身配置了 API Key 时，始终使用该节点的 Key 与 Base URL。GeekNow 图片使用 `GEEKNOW_API_KEY`，GeekNow 视频使用独立的 `GEEKNOW_VIDEO_API_KEY`；其他节点按各自中转配置解析。

### GeekNow 图片模型

图片节点内置以下 GeekNow 模型：

- GPT Image：`gpt-image-2`、`gpt-image-2-pro`、`gpt-image-2.5-flare`、`gpt-image-2.5-sunburst`
- Seedream：`doubao-seedream-4-0-250828`、`doubao-seedream-4-5-251128`、`doubao-seedream-5-0-260128`

后端会按照所选模型、分辨率和宽高比转换成 GeekNow 要求的实际像素尺寸；图生图参考图会转换为 Base64 数组。

### 安全提醒

- `data.db` 中的 Key 为本地存储，建议设置文件权限
- 发布前确认 `seed.json` 中无真实 Key
- `.env` 可能包含中转站 API Key，禁止提交版本库或对外分享

## 🔧 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `:6789` | 服务端口 |
| `DB_PATH` | `./data.db` | SQLite 数据库路径 |
| `DEV_MODE` | `false` | 开发模式 (反向代理前端 dev server) |
| `EMBEDDED` | `false` | 嵌入模式 (从 exe 内置 FS 提供静态文件) |
| `LOG_LEVEL` | `info` | 日志级别 (debug/info) |
| `ZHENGAPI_BASE_URL` | 空 | 文本/视频中转站根地址 |
| `ZHENGAPI_API_KEY` | 空 | 文本/视频中转站密钥，仅在节点未配置 Key 时使用 |
| `ZHENGAPI_USE_DATA_URLS` | `false` | 视频参考图是否直接使用 data URL，避免上传第三方图床 |
| `GEEKNOW_BASE_URL` | 空 | 图片模型中转站根地址；自动补 `/v1` |
| `GEEKNOW_API_KEY` | 空 | 图片模型中转站密钥，仅在图片节点未配置 Key 时使用 |
| `GEEKNOW_VIDEO_API_KEY` | 空 | GeekNow 视频模型专用密钥，与图片密钥分离 |

## 💬 交流反馈

**QQ 群：894246232** —— 使用问题 / 功能建议 / Bug 反馈，欢迎加入交流。

## 📄 License

[MIT](./LICENSE.md)
