# MiniMax H3 Timeline 帧数

适用当前 MiniMaxH3FiniteSegmentVideoSampler 与 TimelinePlanner 组合。服务端错误和插件实现为依据，升级时重新确认约束。

- 每段 length = endFrame - startFrame，必须为整数且满足 5 ≤ length ≤ 3592、(length - 5) % 17 == 0。
- 第一段从 0 开始，后续段不能留空隙。这里采用不重叠、连续排列，避免额外重叠对齐约束。
- 所有分段都要有非空提示词，count 必须等于 segments 数量。
- 为保留原稿镜头时长，向上补齐每段，然后逐镜裁尾再拼接。不能只截整段视频尾部，那会错移镜头切点。
- 使用 scripts/minimax_frames.py 的 plan_frames / validate_segments；返回 generated_start、generated_end 为生成视频里的帧区间，keep_frames 为该区间从起点保留的帧数。
- 24fps 示例：6/5/4 秒对应 158/124/107 生成帧，共389帧，最终分别保留144/120/96帧，共360帧。选择区间与 generation_seconds 同步为389/24秒；最终交付仍为15秒。
- 修改时间线会使既有确认失效，保留旧任务记录与输出，清除当前确认，重读核验后报告“帧数约束检查通过，未生成实测”。

参考实现：https://github.com/Songssx/ComfyUI-MiniMaxH3-TimelineDirector/blob/main/minimax_h3_finite_segments.py 中 _prepare_timeline_segments。

## 音频解码兼容

本地服务器曾只在 ComfyUI-MiniMaxH3 原采样器中修复音频解码，TimelineDirector 的有限分段采样器直接调用 ComfyUI vae_decode_audio，未经过修复，可能产生 NaN 音频并在封装时报 avcodec_send_frame 错误。

已部署的有限分段修复使用同目录 h3_finite_audio_decode.py，调用相邻 ComfyUI-MiniMaxH3/audio_decode_worker.py 在独立 CPU 进程中读取 FP32 音频 VAE；补丁源码在项目 scripts/h3_finite_audio_decode.py。明确检查 latent 与音频波形是否有限，不清零或静音替代错误。升级插件后检查导入是否保留；八实例需一致部署。修复测试为两段22帧、320×192，确认 H.264/AAC 32kHz 双声道封装完成，不代表长片语音内容和口型验收通过。

独立解码的 VAE 加载器也必须正确融合原始 checkpoint 的 weight_g/weight_v。卷积权重应沿所有非输出维度求范数，不能只用 dim=1；否则虽然没有 NaN，仍会产生严重失真。修复工具：项目 scripts/patch_h3_weightnorm.py。质量测试：用 ComfyUI venv 执行 scripts/verify_h3_audio_vae.py，传入实际 worker 和模型路径，它会对440Hz已知音频编码并调用真实子进程解码，要求相关性>0.98且RMS接近原音。已部署测试相关性0.999519、RMS 0.139706。此检查不能代替对生成对白内容、说话人及口型的试听验收。
