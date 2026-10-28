package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type comfyExecuteRequest struct {
	ServerURL        string          `json:"server_url"`
	WorkflowJSON     json.RawMessage `json:"workflow_json"`
	Variables        map[string]any  `json:"variables"`
	ImageVariables   []string        `json:"image_variables"`
	ApprovalRequired bool            `json:"approval_required"`
	KeyframeApproved bool            `json:"keyframe_approved"`
	CanvasID         string          `json:"canvas_id"`
	NodeID           string          `json:"node_id"`
	Seed             *int64          `json:"seed"`
}

type comfyPromptResponse struct {
	PromptID   string          `json:"prompt_id"`
	Error      json.RawMessage `json:"error,omitempty"`
	NodeErrors json.RawMessage `json:"node_errors,omitempty"`
}

type comfyResultRequest struct {
	ServerURL string `form:"server_url"`
}

func (h *Handler) ExecuteComfyUI(c *gin.Context) {
	var req comfyExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ServerURL == "" {
		req.ServerURL = "http://localhost:8188"
	}
	if req.ApprovalRequired && !req.KeyframeApproved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先确认关键帧，再执行视频工作流"})
		return
	}

	// Remove leading "api/" from workflow if it's a full API workflow
	var workflowJSON json.RawMessage
	var resolveErr error
	workflowJSON, resolveErr = resolveWorkflowVariables(c, req.ServerURL, req.WorkflowJSON, req.Variables, req.ImageVariables)
	if resolveErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": resolveErr.Error()})
		return
	}

	// POST to ComfyUI prompt endpoint
	job, jobErr := h.createWorkflowSubmission(req)
	if jobErr != nil {
		c.JSON(500, gin.H{"error": jobErr.Error()})
		return
	}
	body := map[string]interface{}{
		"prompt":    workflowJSON,
		"prompt_id": job.Prompt, "client_id": job.ID, "extra_data": map[string]string{"flowcanva_job_id": job.ID},
	}
	jsonBody, _ := json.Marshal(body)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("%s/prompt", req.ServerURL),
		"application/json",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		h.setWorkflowSubmissionState(job, "submit_unknown", "提交响应未确认；后台只查询，不会重新提交")
		c.JSON(http.StatusBadGateway, gin.H{"error": "提交响应未确认，后台正在核查，请勿重复生成", "job_id": job.ID, "prompt_id": job.Prompt})
		return
	}
	defer resp.Body.Close()

	var promptResp comfyPromptResponse
	if err := json.NewDecoder(resp.Body).Decode(&promptResp); err != nil {
		h.setWorkflowSubmissionState(job, "submit_unknown", "无法解析提交响应；请勿重复生成")
		respBody, _ := io.ReadAll(resp.Body)
		h.log.Error("comfyui execute parse error", "body", string(respBody))
		c.JSON(http.StatusBadGateway, gin.H{"error": "无法解析ComfyUI响应"})
		return
	}

	if resp.StatusCode >= 400 || promptResp.Error != nil {
		h.setWorkflowSubmissionState(job, "failed", string(promptResp.Error))
		c.JSON(http.StatusBadGateway, gin.H{"error": string(promptResp.Error) + " node_errors: " + string(promptResp.NodeErrors)})
		return
	}

	if promptResp.PromptID == "" {
		h.setWorkflowSubmissionState(job, "submit_unknown", "上游未返回任务编号，请勿重复生成")
		c.JSON(502, gin.H{"error": "提交状态待确认，后台继续查询，请勿重复生成", "job_id": job.ID})
		return
	}
	if err := h.acceptWorkflowSubmission(job, promptResp.PromptID, req.Seed); err != nil {
		h.log.Error("workflow submission accepted but binding failed", "job_id", job.ID, "error", err)
		c.JSON(502, gin.H{"error": "上游已接受任务，记录更新失败，请勿重复生成", "job_id": job.ID})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prompt_id": promptResp.PromptID, "job_id": job.ID, "durable_media": true})
}

func (h *Handler) ProxyComfyUIImage(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少url参数"})
		return
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("获取图片失败: %v", err)})
		return
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	c.DataFromReader(http.StatusOK, resp.ContentLength, ct, resp.Body, nil)
}
