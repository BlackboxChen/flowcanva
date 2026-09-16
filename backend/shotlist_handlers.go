package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 分镜步骤的知识文件体量较大，响应也可能很长，用比普通对话更宽松的超时。
var shotlistClient = &http.Client{Timeout: 300 * time.Second}

type shotlistRunRequest struct {
	Step    string          `json:"step"`
	Input   json.RawMessage `json:"input"`
	Model   string          `json:"model"`
	Channel string          `json:"channel,omitempty"`
	BaseURL string          `json:"base_url,omitempty"`
	APIKey  string          `json:"api_key,omitempty"`
}

type shotlistRunResponse struct {
	Step   string          `json:"step"`
	Data   json.RawMessage `json:"data,omitempty"`
	Raw    string          `json:"raw"`
	Parsed bool            `json:"parsed"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ListSkills 返回内嵌的技能清单。
func (h *Handler) ListSkills(c *gin.Context) {
	resources, err := skillResourceManifest()
	if err != nil {
		h.log.Error("list skill resources failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items": []gin.H{{
			"key":         "shotlist",
			"name":        "导演分镜（Deepwhite v3.7）",
			"description": "把锁定剧本转译为导演文字分镜与 Seedance 2.5 视频提示词",
			"resources":   len(resources),
		}},
	})
}

// GetShotlistSteps 返回分镜技能的步骤定义、Gate 与每步需要的知识文件。
func (h *Handler) GetShotlistSteps(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": "3.7",
		"steps":   shotlistSteps,
		"states":  shotlistStateOrder,
	})
}

// RunShotlistStep 组装某一步的提示词、调用上游模型，并把结果解析成 JSON 返回。
func (h *Handler) RunShotlistStep(c *gin.Context) {
	var req shotlistRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	step, ok := findSkillStep(req.Step)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未知的分镜步骤: " + req.Step})
		return
	}
	if step.Kind != "llm" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "步骤 " + step.Key + " 不需要模型参与"})
		return
	}
	if len(req.Input) == 0 {
		req.Input = json.RawMessage("{}")
	}

	messages, err := buildShotlistMessages(step, req.Input)
	if err != nil {
		h.log.Error("build shotlist messages failed", "step", step.Key, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	baseURL, apiKey, err := h.resolveUpstream(req.BaseURL, req.APIKey, "https://api.openai.com/v1", true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	raw, err := h.callShotlistModel(req.Model, messages, baseURL, apiKey)
	if err != nil {
		h.log.Error("shotlist model call failed", "step", step.Key, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	resp := shotlistRunResponse{Step: step.Key, Raw: raw}
	if data, ok := extractJSONObject(raw); ok {
		resp.Data = json.RawMessage(data)
		resp.Parsed = true
	} else {
		h.log.Warn("shotlist response is not parseable json", "step", step.Key, "preview", truncate(raw, 200))
	}

	h.log.Info("shotlist step done", "step", step.Key, "model", req.Model, "parsed", resp.Parsed, "bytes", len(raw))
	c.JSON(http.StatusOK, resp)
}

// ValidateShotlist 对分镜台节点的 content 做结构校验。
func (h *Handler) ValidateShotlist(c *gin.Context) {
	content, err := bindShotlistContent(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, validateShotlist(content))
}

// ExportShotlist 把分镜台节点的 content 渲染成完整 Markdown 交付文件。
func (h *Handler) ExportShotlist(c *gin.Context) {
	content, err := bindShotlistContent(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	markdown := renderShotlistMarkdown(content)
	c.JSON(http.StatusOK, gin.H{
		"filename": shotlistExportFilename(content),
		"markdown": markdown,
	})
}

// ─── 内部实现 ──────────────────────────────────────────────────────

// bindShotlistContent 接受 {content: "<JSON 字符串>"} 或直接的分镜内容对象。
func bindShotlistContent(c *gin.Context) (*ShotlistContent, error) {
	var body struct {
		Content string          `json:"content"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		return nil, err
	}

	raw := body.Content
	if raw == "" {
		raw = string(body.Payload)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, errEmptyShotlistContent
	}

	var content ShotlistContent
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return nil, err
	}
	return &content, nil
}

func (h *Handler) callShotlistModel(model string, messages []ChatMessage, baseURL, apiKey string) (string, error) {
	bodyMap := map[string]any{
		"model":       model,
		"messages":    messages,
		"temperature": 0.4,
	}
	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := shotlistClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", &upstreamError{Status: resp.StatusCode, Body: truncate(string(respBody), 400)}
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != nil {
		return "", &upstreamError{Status: resp.StatusCode, Body: parsed.Error.Message}
	}
	if len(parsed.Choices) == 0 {
		return "", &upstreamError{Status: resp.StatusCode, Body: "上游没有返回任何结果"}
	}
	return parsed.Choices[0].Message.Content, nil
}

type upstreamError struct {
	Status int
	Body   string
}

func (e *upstreamError) Error() string {
	return "上游返回错误（" + http.StatusText(e.Status) + "）：" + e.Body
}

var errEmptyShotlistContent = &upstreamError{Status: http.StatusBadRequest, Body: "分镜内容为空"}

// extractJSONObject 从模型输出里取出 JSON 对象。
// 依次尝试：直接解析 → 去掉 Markdown 围栏 → 截取首个 { 到末个 }。
func extractJSONObject(raw string) (string, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", false
	}
	if json.Valid([]byte(text)) && strings.HasPrefix(text, "{") {
		return text, true
	}

	stripped := stripCodeFence(text)
	if json.Valid([]byte(stripped)) && strings.HasPrefix(stripped, "{") {
		return stripped, true
	}

	start := strings.Index(stripped, "{")
	end := strings.LastIndex(stripped, "}")
	if start >= 0 && end > start {
		candidate := stripped[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate, true
		}
	}
	return "", false
}

func stripCodeFence(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	if idx := strings.Index(trimmed, "\n"); idx >= 0 {
		trimmed = trimmed[idx+1:]
	}
	if idx := strings.LastIndex(trimmed, "```"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.TrimSpace(trimmed)
}

func shotlistExportFilename(content *ShotlistContent) string {
	scope := strings.TrimSpace(content.Project.Title)
	if scope == "" {
		scope = "未命名"
	}
	scope = strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(scope)
	return "Shotlist_" + scope + "_Seedance2_5_ZH.md"
}
