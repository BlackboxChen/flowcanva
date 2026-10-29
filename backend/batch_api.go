package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"time"
)

type batchTask struct {
	ID           string `json:"id"`
	Batch        string `json:"batch_id"`
	Canvas       string `json:"canvas_id"`
	Node         string `json:"node_id"`
	Snapshot     string `json:"-"`
	Dependencies string `json:"dependencies"`
	Status       string `json:"status"`
	Job          string `json:"job_id"`
	Error        string `json:"error"`
}

func (h *Handler) batchTasks(canvas string) ([]batchTask, error) {
	rows, err := h.db.Query(`SELECT id,batch_id,canvas_id,node_id,snapshot,dependencies,status,job_id,error FROM batch_tasks WHERE canvas_id=? ORDER BY created_at,id`, canvas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []batchTask{}
	for rows.Next() {
		var t batchTask
		if err = rows.Scan(&t.ID, &t.Batch, &t.Canvas, &t.Node, &t.Snapshot, &t.Dependencies, &t.Status, &t.Job, &t.Error); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
func (h *Handler) ListBatchTasks(c *gin.Context) {
	tasks, err := h.batchTasks(c.Param("cid"))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, tasks)
}
func (h *Handler) StopBatch(c *gin.Context) {
	admissionLock.Lock()
	defer admissionLock.Unlock()
	_, err := h.db.Exec(`UPDATE batch_tasks SET status='cancelled',error='停止后续调度；已提交任务继续执行' WHERE canvas_id=? AND status IN ('pending','review')`, c.Param("cid"))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.Status(204)
}

func (h *Handler) CreateBatch(c *gin.Context) {
	var body struct {
		Nodes []string `json:"node_ids"`
	}
	if c.ShouldBindJSON(&body) != nil || len(body.Nodes) == 0 || len(body.Nodes) > 100 {
		c.JSON(400, gin.H{"error": "请选择 1–100 个工作流节点"})
		return
	}
	admissionLock.Lock()
	defer admissionLock.Unlock()
	cid := c.Param("cid")
	var active int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM batch_tasks WHERE canvas_id=? AND status IN ('pending','review','running','submitting')`, cid).Scan(&active); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if active > 0 {
		c.JSON(409, gin.H{"error": "本画布已有批次，请先完成或停止后续调度"})
		return
	}
	tasks := []batchTask{}
	selected := map[string]bool{}
	batch := uuid.NewString()
	for _, id := range body.Nodes {
		if selected[id] {
			continue
		}
		selected[id] = true
		var raw string
		if err := h.db.QueryRow(`SELECT content FROM nodes WHERE id=? AND canvas_id=? AND node_type='workflow'`, id, cid).Scan(&raw); err != nil {
			c.JSON(400, gin.H{"error": "节点不存在或不是工作流: " + id})
			return
		}
		deps, err := h.workflowDependencies(cid, id, raw)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		encoded, _ := json.Marshal(deps)
		tasks = append(tasks, batchTask{ID: uuid.NewString(), Batch: batch, Canvas: cid, Node: id, Snapshot: raw, Dependencies: string(encoded), Status: "pending"})
	}
	if err := validateBatchDAG(tasks); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	for _, t := range tasks {
		if _, err = tx.Exec(`INSERT INTO batch_tasks(id,batch_id,canvas_id,node_id,snapshot,dependencies,status,created_at) VALUES(?,?,?,?,?,?,?,?)`, t.ID, t.Batch, t.Canvas, t.Node, t.Snapshot, t.Dependencies, t.Status, time.Now().Unix()); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	if err = tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"batch_id": batch, "tasks": tasks})
}

func validateBatchDAG(tasks []batchTask) error {
	graph := map[string][]string{}
	state := map[string]int{}
	for _, t := range tasks {
		var deps []string
		_ = json.Unmarshal([]byte(t.Dependencies), &deps)
		graph[t.Node] = deps
	}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return false
		}
		if state[id] == 2 {
			return true
		}
		state[id] = 1
		for _, d := range graph[id] {
			if _, ok := graph[d]; ok && !visit(d) {
				return false
			}
		}
		state[id] = 2
		return true
	}
	for id := range graph {
		if !visit(id) {
			return fmt.Errorf("工作流存在循环依赖，无法调度")
		}
	}
	return nil
}

func (h *Handler) workflowDependencies(cid, id, raw string) ([]string, error) {
	data := workflowDefinition{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, v := range data.Variables {
		if v.Source != "" {
			set[v.Source] = true
		}
	}
	// Workflow-to-workflow edges are execution dependencies; notes and review text are not.
	rows, err := h.db.Query(`SELECT e.source_node_id FROM edges e JOIN nodes n ON n.id=e.source_node_id WHERE e.canvas_id=? AND e.target_node_id=? AND n.node_type='workflow'`, cid, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var source string
		if rows.Scan(&source) == nil {
			set[source] = true
		}
	}
	deps := []string{}
	for source := range set {
		deps = append(deps, source)
	}
	return deps, rows.Err()
}
