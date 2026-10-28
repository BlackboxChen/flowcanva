package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) createWorkflowSubmission(req comfyExecuteRequest) (workflowJob, error) {
	id := uuid.NewString()
	j := workflowJob{ID: id, Server: strings.TrimRight(req.ServerURL, "/"), Prompt: id, Canvas: req.CanvasID, Node: req.NodeID, Status: "submitting"}
	if j.Node != "" || j.Canvas != "" {
		var kind string
		if err := h.db.QueryRow(`SELECT node_type FROM nodes WHERE id=? AND canvas_id=?`, j.Node, j.Canvas).Scan(&kind); err != nil {
			return j, fmt.Errorf("绑定的画布节点不存在")
		}
		if kind != "workflow" {
			return j, fmt.Errorf("此任务必须绑定工作流节点")
		}
	}
	if err := h.insertWorkflowJob(j); err != nil {
		return j, err
	}
	if err := h.bindWorkflowJob(j); err != nil {
		h.setWorkflowSubmissionState(j, "failed", err.Error())
		return j, err
	}
	return j, nil
}

func (h *Handler) setWorkflowSubmissionState(j workflowJob, status, message string) {
	if _, err := h.db.Exec(`UPDATE workflow_jobs SET status=?,error=?,next_retry=? WHERE id=?`, status, message, time.Now().Unix(), j.ID); err != nil {
		h.log.Error("workflow submission state persist failed", "job_id", j.ID, "error", err)
	}
}

func (h *Handler) acceptWorkflowSubmission(j workflowJob, prompt string, seed *int64) error {
	if _, err := h.db.Exec(`UPDATE workflow_jobs SET prompt_id=?,status='waiting',error='' WHERE id=?`, prompt, j.ID); err != nil {
		return err
	}
	return h.mutateWorkflowNode(j, func(data map[string]any) bool {
		if data["workflowJobID"] != j.ID {
			return false
		}
		data["lastPromptId"] = prompt
		data["jobStatus"] = "waiting"
		if seed != nil {
			data["lastSeed"] = *seed
		}
		return true
	})
}

func (h *Handler) GetComfyUIResult(c *gin.Context) {
	server := strings.TrimRight(c.DefaultQuery("server_url", "http://localhost:8188"), "/")
	prompt := c.Param("prompt_id")
	j, err := h.readWorkflowJob(server, prompt)
	// The initially reserved prompt UUID is also the job ID; retain that alias
	// when recovering an upstream submission that returned a different prompt ID.
	if err == sql.ErrNoRows {
		var actual string
		if lookupErr := h.db.QueryRow(`SELECT prompt_id FROM workflow_jobs WHERE id=? AND server_url=?`, prompt, server).Scan(&actual); lookupErr == nil {
			j, err = h.readWorkflowJob(server, actual)
		}
	}
	if err == sql.ErrNoRows {
		j = workflowJob{ID: legacyWorkflowID(server, prompt), Server: server, Prompt: prompt, Status: "waiting"}
		err = h.insertWorkflowJob(j)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "读取持久化任务失败"})
		return
	}
	outputs := []map[string]string{}
	if j.Status == "done" {
		if err = json.Unmarshal([]byte(j.Local), &outputs); err != nil {
			c.JSON(500, gin.H{"error": "本地任务记录无效"})
			return
		}
		for _, o := range outputs {
			if o["kind"] != "text" {
				if _, err = localWorkflowFile(o["url"]); err != nil {
					h.retryWorkflowJob(j, "save_retry", fmt.Errorf("本地副本缺失，正在重新下载"))
					c.JSON(http.StatusAccepted, gin.H{"outputs": []any{}, "job_status": "save_retry", "job_id": j.ID})
					return
				}
			}
		}
	}
	if j.Status == "failed" {
		c.JSON(200, gin.H{"error": j.Error, "job_status": j.Status, "job_id": j.ID})
		return
	}
	c.JSON(200, gin.H{"outputs": outputs, "job_status": j.Status, "job_id": j.ID, "warning": j.Error, "durable_media": true})
}
