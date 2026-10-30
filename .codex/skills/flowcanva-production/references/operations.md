# FlowCanva API 操作

以当前仓库 backend/router.go、canvas_handlers.go 和 comfyui_handlers.go 为最终依据，版本变化时先检查实现。

- GET/POST `/api/canvases`：列出/创建画布。
- GET `/api/canvases/{cid}`：完整节点和边。
- POST `/api/canvases/{cid}/nodes`：node_type、x、y、width、height、content、config。content/config 是字符串；结构化内容先 JSON 编码。
- PUT `/api/canvases/{cid}/nodes/{id}`：可仅传 content 等目标字段，避免重写布局。更新 content 需附 `expected_content`，更新 config 需附 `expected_config`，更新位置或尺寸需附 `expected_layout: [x,y,width,height]`，取自读取时的同一节点。409 时保留本地修改、重新读取并提示冲突，不盲目覆盖。不存在事务式批量更新，逐步保存完成记录。
- POST `/api/canvases/{cid}/edges`：source_node_id、target_node_id。先检查已有边，避免重复。
- POST `/api/admin/assets/upload`：multipart file。返回目标实例内的素材 URL；跨实例须上传文件，不直接复用另一个实例的 `/uploads/` 相对地址。
- 服务器节点默认并发上限、批次和服务状态以当前 `/api/collaboration/settings` 及对应画布 batch API 为准；批次只调度用户选择的工作流节点。服务端排队不代表 GPU 实际并行。
- POST `/api/comfyui/execute`：server_url、workflow_json、variables、image_variables、approval_required、keyframe_approved。具体 JSON 编码、种子和占位符处理参考 WorkflowNodePanel.vue 与 useWorkflowSeed.ts，不能绕过确认逻辑。
- GET `/api/comfyui/result/{prompt_id}?server_url=...`：恢复查询；先读取后端响应结构，不猜成功字段。

workflow content 常用字段：displayName、serverUrl、workflowJson（JSON字符串）、variables（name/type/value/sourceNodeId）、outputType、outputs、lastPromptId、seedMode、fixedSeed、lastSeed、approvalRequired、approvedFingerprint。完整保留其他字段。

打铁花参考位于 scripts/iron_flower_canvas/。create.py 依赖专用素材目录，并会覆盖其管理节点，不得直接用于刷新已有用户编辑画布。client.py 的状态路径和服务器默认值也需要项目化后才能复用。

写入后重新读取验证。保留每次修改前快照；恢复时逐字段恢复本轮变化，不能覆盖快照之后的用户修改。资产上传缓存应按文件内容哈希识别版本。
