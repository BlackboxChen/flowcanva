package main

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"sync"
	"time"
)

var batchRunning sync.Map

func (h *Handler) startBatchWorker() {
	// An interrupted dispatch may already have reached ComfyUI. Reconcile node job IDs first.
	_, err := h.db.Exec(`UPDATE batch_tasks SET status='pending' WHERE status='submitting'`)
	if err != nil {
		h.log.Error("batch recovery", "error", err)
	}
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			h.tickBatches()
		}
	}()
}
func (h *Handler) tickBatches() {
	rows, err := h.db.Query(`SELECT DISTINCT canvas_id FROM batch_tasks WHERE status IN ('pending','review','running')`)
	if err != nil {
		h.log.Error("batch list", "error", err)
		return
	}
	canvases := []string{}
	for rows.Next() {
		var cid string
		if rows.Scan(&cid) == nil {
			canvases = append(canvases, cid)
		}
	}
	rows.Close()
	for _, cid := range canvases {
		tasks, err := h.batchTasks(cid)
		if err != nil {
			continue
		}
		for _, t := range tasks {
			if t.Status != "pending" && t.Status != "review" && t.Status != "running" {
				continue
			}
			if _, busy := batchRunning.LoadOrStore(t.ID, true); busy {
				continue
			}
			go func(t batchTask) { defer batchRunning.Delete(t.ID); h.advanceBatchTask(t, tasks) }(t)
		}
	}
}
func (h *Handler) setBatch(t batchTask, status, job, message string) {
	if _, err := h.db.Exec(`UPDATE batch_tasks SET status=?,job_id=?,error=? WHERE id=? AND status!='cancelled'`, status, job, message, t.ID); err != nil {
		h.log.Error("batch state", "task", t.ID, "error", err)
	}
}
func (h *Handler) advanceBatchTask(t batchTask, tasks []batchTask) {
	var raw string
	if err := h.db.QueryRow(`SELECT content FROM nodes WHERE id=? AND canvas_id=?`, t.Node, t.Canvas).Scan(&raw); err != nil {
		h.setBatch(t, "failed", "", "节点不存在")
		return
	}
	var data, initial workflowDefinition
	if json.Unmarshal([]byte(raw), &data) != nil || json.Unmarshal([]byte(t.Snapshot), &initial) != nil {
		h.setBatch(t, "failed", "", "节点内容无效")
		return
	}
	// Pick up a manual approval/run, or a dispatch accepted just before process restart.
	if t.Job == "" && data.Job != "" && data.Job != initial.Job {
		t.Job = data.Job
	}
	if t.Job != "" {
		var status string
		if err := h.db.QueryRow(`SELECT status FROM workflow_jobs WHERE id=?`, t.Job).Scan(&status); err != nil {
			h.setBatch(t, "failed", t.Job, "任务记录不存在")
			return
		}
		switch status {
		case "done":
			h.setBatch(t, "done", t.Job, "")
		case "failed":
			h.setBatch(t, "failed", t.Job, "生成失败，请查询原任务")
		default:
			h.setBatch(t, "running", t.Job, status)
		}
		return
	}
	var deps []string
	_ = json.Unmarshal([]byte(t.Dependencies), &deps)
	for _, dep := range deps {
		selected := false
		for _, other := range tasks {
			if other.Batch == t.Batch && other.Node == dep {
				selected = true
				if other.Status == "failed" || other.Status == "blocked" || other.Status == "cancelled" {
					h.setBatch(t, "blocked", "", "上游失败或停止: "+dep)
					return
				}
				if other.Status != "done" {
					return
				}
			}
		}
		if !selected {
			var sourceRaw string
			if err := h.db.QueryRow(`SELECT content FROM nodes WHERE id=? AND canvas_id=?`, dep, t.Canvas).Scan(&sourceRaw); err != nil {
				h.setBatch(t, "blocked", "", "缺少上游节点")
				return
			}
			if _, busy := h.activeNodeJob(t.Canvas, dep); busy {
				return
			}
			var source map[string]any
			_ = json.Unmarshal([]byte(sourceRaw), &source)
			out, _ := source["outputs"].([]any)
			if len(out) == 0 && source["url"] == nil && source["dataUrl"] == nil {
				h.setBatch(t, "blocked", "", "未选中的上游没有结果，请先生成: "+dep)
				return
			}
		}
	}
	if data.Approval {
		h.setBatch(t, "review", "", "请打开节点检查最新关键帧，人工确认后单独执行")
		return
	}
	if batchEditableContent(raw) != batchEditableContent(t.Snapshot) {
		h.setBatch(t, "blocked", "", "排队后配置已修改，请停止此批次并重新选择执行")
		return
	}
	req, err := h.batchRequest(t, raw)
	if err != nil {
		h.setBatch(t, "blocked", "", err.Error())
		return
	}
	// Serialize stop-vs-dispatch decision, but not independent upstream requests.
	admissionLock.Lock()
	result, err := h.db.Exec(`UPDATE batch_tasks SET status='submitting' WHERE id=? AND status='pending'`, t.ID)
	admissionLock.Unlock()
	if err != nil {
		return
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return
	}
	body, _ := json.Marshal(req)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "http://127.0.0.1/api/comfyui/execute", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	h.ExecuteComfyUI(ctx)
	var response struct {
		Job   string `json:"job_id"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	if response.Job != "" {
		h.setBatch(t, "running", response.Job, response.Error)
		return
	}
	if recorder.Code == 429 {
		h.setBatch(t, "pending", "", "等待生成服务空闲")
		return
	}
	h.setBatch(t, "failed", "", response.Error)
}
