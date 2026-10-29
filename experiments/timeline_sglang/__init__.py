from .node import TimelineSGLangSandbox
from .keyframe_node import TimelineSGLangKeyframes

NODE_CLASS_MAPPINGS = {'FlowTimelineSGLangSandbox': TimelineSGLangSandbox, 'FlowTimelineSGLangKeyframes': TimelineSGLangKeyframes}
NODE_DISPLAY_NAME_MAPPINGS = {'FlowTimelineSGLangSandbox': 'Timeline → SGLang 四卡（隔离实验）', 'FlowTimelineSGLangKeyframes': 'Timeline → 四卡 SGLang 逐镜首帧（实验）'}
