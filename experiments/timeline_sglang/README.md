# Timeline → 四卡 SGLang 隔离实验

## 第二章手动测试画布（2026-10-29）

### flowcanva-production 增量导入

`experiments/import_chapter2_production.py` 依项目技能增量补入制作区，保留所有原节点、用户修改、V11 已有结果和任务编号。导入状态文件避免重复创建。

- 新增 9 类资产（6 个 Qwen 生图工作流、3 个注明来源的历史道具资产）、23 个逐镜关键帧工作流、8 个待审核节点及声音/交付清单。
- 资产至关键帧的引用同时绑定 sourceNodeId、图像变量、工作流占位符和真实边；缺失资产输出留空，执行前需先生成资产。
- 原稿审核通过；23 个关键帧保留对应镜头原文、文件哈希；视频帧数通过技能计算规则校验。
- 新增关键帧尚未生成或确认，未提交任何新生成。
- **能力缺口**：既有八个四卡 T2VA 节点仍不消费新增关键帧。生产视频的关键帧绑定、确认门槛和锁定音频尚未接通，不能称完整图生视频制作链路已经完成。等待用户选择继续适配四卡或采用既有原 Timeline 路径；不使用假连线表示这些能力已可用。

- 画布：`http://127.0.0.1:16789/canvas/#/?canvas=cv_3fef3e02`。
- 8 个视频节点 V11–V18、23 个原稿镜头，裁尾后共 120 秒；未提交生成。
- 每个 V 节点包含实际 TimelinePlanner → SGLang 适配节点工作流，按镜头顺序独立生成，返回多个视频，尚不自动拼接。
- 当前为 T2VA 接口测试，512×288、4 步；无参考图身份锁定、无锁定音轨。对白和音效保留在提示词，准确性待实测。
- 每次执行随机种子；`run_token` 使用后端内置 `{{run_id}}`，正常新执行自动分配新目录。超时应查原 prompt_id，不要通过新执行恢复旧任务。
- 原稿一致性审核通过：8 段、23 镜。操作前快照、读回快照、逐镜裁尾表在本地 `.run/chapter2-sglang/`。
- 本地 `127.0.0.1:18299` 为 SSH 通道，已启动，不公开服务器端口。电脑重启或通道断开后，在项目目录运行 `python experiments/sglang_canvas_tunnel.py`；该进程需保持运行。不要在端口已监听时重复启动。
- 可重复导入脚本：`experiments/create_chapter2_sglang_canvas.py`，有状态文件时拒绝重复创建画布。此脚本不提交生成。

这是实验适配器，不是生产 Timeline 采样器的替代版本。

## 隔离边界

- 容器：`flowcanva-timeline-sglang-sandbox`。
- ComfyUI：服务器 `127.0.0.1:18299`，CPU 模式、无 GPU 设备、2 CPU / 6GB 内存限制。
- 独立目录：`/home/thao/timeline-sglang-sandbox`，输入、输出、临时目录、用户数据库均独立。
- 原 Timeline 插件只读挂载；未修改其文件，也未修改 8179 调度器、生产画布及 GPU 4–7 的服务。
- HTTP 推理目标：现有 SGLang `127.0.0.1:30010`，GPU 0–3 / TP=4。
- 不运行模型重载、显存释放、清空队列、取消其他任务等操作。

## 已验证

- `python experiments/test_timeline_sglang.py -v`：9 项离线测试通过。
- 实际 ComfyUI 调用原 `MiniMaxH3TimelinePlanner` → 新 `FlowTimelineSGLangSandbox` 的 inspect 模式成功，未提交 GPU 任务。
- 新版规划器的有限分段输出为 **第3个输出（API 索引2）**，不能误接索引0的原始时间线计划。
- `inspect_workflow_api.json` 明确设置 selection 和 segmentConfig，验证有限计划为73帧，避免继承旧工作流窗口长度。

## 当前能力和限制

- T2VA：从有限计划提取逐段提示词、时长、画幅与种子。
- FL2VA：仅单段、一个显式首帧 URI；该 URI 必须是 SGLang 可读取的服务器路径。
- 原参考图不能被 T2VA 悄悄忽略；多参考图不能被压成一张首帧。
- 默认拒绝锁定/参考音频。`external_post` 仅适用于明确同意外部后期配音的测试，不代表实现了锁音。
- 拒绝 latent overlap、二次采样和尚未实现的 REF2VA 模式。
- 多个独立段可以分别生成，但当前不做 latent 续接，不承诺跨段人物或运动连续性，也不自动拼接。
- 使用服务端实际 canonical 请求：`task`、`conditions`、`target`；不发送会被该 H3 接口拒绝的显式 fps/num_frames/CFG 字段。

## 首轮 GPU 样例

`experiments/run_sglang_gpu_probe.py` 已复制到服务器 `/sandbox/run_gpu_probe.py`，在独立 CPU 容器内运行。

- 先等待现有 SGLang 任务结束；同时刷新旧任务状态，避免把过期列表误认为永久排队。
- 两个样例：73帧、2步、短边288，分别为文本和已保存的 K03 首帧，**只作接口冒烟验证，不能作为画质验收**。
- 测试首帧单独复制到 SGLang 输出挂载中的 `timeline-sglang-sandbox` 子目录，不改模型或原有素材。
- 等待上限4小时；保存请求与任务 ID；提交响应不明时拒绝自动再提交。
- 下载并用 ffprobe 检查输出。默认任务等待上限30分钟，超时后保留请求记录用于恢复。
- 报告：宿主机 `/home/thao/timeline-sglang-sandbox/reports/gpu-probe-v1/` 下的 `probe.log`、`results.json`、逐任务 `request.json` / `status.json`。

GPU 样例需以实际 `results.json` 和视频复核为准，不能仅依据 health=ok、节点已加载或 inspect 成功宣布适配完成。

## 手动访问

先在 Windows 开 SSH 隧道（需要保持该终端打开）：

```powershell
ssh -N -L 18299:127.0.0.1:18299 thao@10.88.0.30
```

再访问 `http://127.0.0.1:18299/`。API 工作流 `inspect_workflow_api.json` 默认只检查计划；GPU 模式需显式改为 generate。

`run_token` 是防重复提交键：恢复同一任务时保持相同；新生成才使用新值。不要在提交状态不明时随意换 token 重试。

本目录仅为独立实验，不应复制节点到生产实例或替换生产采样器。REF2VA、多段续接与锁音需要后续分别验证。
