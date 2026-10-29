package main

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strings"
	"sync"
)

var admissionLock sync.Mutex

// Task progress is server-owned and may advance while a user edits a prompt.
func editableContent(raw string) string {
	var data map[string]any
	if json.Unmarshal([]byte(raw), &data) != nil {
		return raw
	}
	for _, key := range []string{"workflowJobID", "lastPromptId", "jobStatus", "outputs", "lastCompletedPromptId", "lastSeed", "jobError"} {
		delete(data, key)
	}
	result, _ := json.Marshal(data)
	return string(result)
}

func (h *Handler) collaborationMigration() error {
	for _, sql := range []string{
		`CREATE TABLE IF NOT EXISTS collaboration_settings (server TEXT PRIMARY KEY, max_active INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS asset_trash (asset_id TEXT PRIMARY KEY, deleted_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS asset_projects (asset_id TEXT PRIMARY KEY, project TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS canvas_projects (canvas_id TEXT PRIMARY KEY, project TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS batch_tasks (id TEXT PRIMARY KEY, batch_id TEXT NOT NULL, canvas_id TEXT NOT NULL, node_id TEXT NOT NULL, snapshot TEXT NOT NULL, dependencies TEXT NOT NULL, status TEXT NOT NULL, job_id TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL)`,
	} {
		if _, err := h.db.Exec(sql); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) CollaborationSettings(c *gin.Context) {
	if c.Request.Method == "PUT" {
		var body struct {
			Server string `json:"server"`
			Max    int    `json:"max_active"`
		}
		if c.ShouldBindJSON(&body) != nil || body.Server == "" || body.Max < 1 || body.Max > 8 {
			c.JSON(400, gin.H{"error": "并发上限须为 1–8"})
			return
		}
		body.Server = strings.TrimRight(strings.TrimSpace(body.Server), "/")
		if _, err := h.db.Exec(`INSERT INTO collaboration_settings VALUES(?,?) ON CONFLICT(server) DO UPDATE SET max_active=excluded.max_active`, body.Server, body.Max); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	rows, err := h.db.Query(`SELECT server,max_active FROM collaboration_settings`)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	limits := map[string]int{}
	for rows.Next() {
		var server string
		var max int
		if rows.Scan(&server, &max) == nil {
			limits[server] = max
		}
	}
	c.JSON(200, gin.H{"limits": limits, "default_max_active": 2})
}

func (h *Handler) activeNodeJob(canvas, node string) (workflowJob, bool) {
	var server, prompt string
	err := h.db.QueryRow(`SELECT server_url,prompt_id FROM workflow_jobs WHERE canvas_id=? AND node_id=? AND status NOT IN ('done','failed') AND id NOT LIKE 'legacy_%' ORDER BY created_at DESC LIMIT 1`, canvas, node).Scan(&server, &prompt)
	if err != nil {
		return workflowJob{}, false
	}
	j, err := h.readWorkflowJob(server, prompt)
	return j, err == nil
}

// Reserve a persistent slot before resolving uploads or sending an upstream request.
func (h *Handler) admitSubmission(c *gin.Context, req comfyExecuteRequest) (workflowJob, bool) {
	admissionLock.Lock()
	defer admissionLock.Unlock()
	if req.NodeID != "" {
		if j, ok := h.activeNodeJob(req.CanvasID, req.NodeID); ok {
			c.JSON(200, gin.H{"prompt_id": j.Prompt, "job_id": j.ID, "reused": true, "server_url": j.Server, "job_status": j.Status})
			return j, false
		}
	}
	limit := 2
	_ = h.db.QueryRow(`SELECT max_active FROM collaboration_settings WHERE server=?`, req.ServerURL).Scan(&limit)
	var active int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM workflow_jobs WHERE server_url=? AND status NOT IN ('done','failed') AND id NOT LIKE 'legacy_%'`, req.ServerURL).Scan(&active); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return workflowJob{}, false
	}
	if active >= limit {
		c.JSON(429, gin.H{"error": "该生成服务已达到并发上限，请等待现有任务完成"})
		return workflowJob{}, false
	}
	j, err := h.createWorkflowSubmission(req)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return j, false
	}
	return j, true
}
