package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type workflowJob struct {
	ID, Server, Prompt, Canvas, Node, Status, Remote, Local, Error string
	Attempts                                                       int
}

func (h *Handler) readWorkflowJob(server, prompt string) (workflowJob, error) {
	var j workflowJob
	err := h.db.QueryRow(`SELECT id,server_url,prompt_id,canvas_id,node_id,status,remote_json,local_json,error,attempts FROM workflow_jobs WHERE server_url=? AND prompt_id=?`, strings.TrimRight(server, "/"), prompt).Scan(&j.ID, &j.Server, &j.Prompt, &j.Canvas, &j.Node, &j.Status, &j.Remote, &j.Local, &j.Error, &j.Attempts)
	return j, err
}

func (h *Handler) insertWorkflowJob(j workflowJob) error {
	_, err := h.db.Exec(`INSERT OR IGNORE INTO workflow_jobs(id,server_url,prompt_id,canvas_id,node_id,status,created_at) VALUES(?,?,?,?,?,?,?)`, j.ID, strings.TrimRight(j.Server, "/"), j.Prompt, j.Canvas, j.Node, j.Status, time.Now().Unix())
	return err
}

func legacyWorkflowID(server, prompt string) string {
	hash := sha256.Sum256([]byte(strings.TrimRight(server, "/") + "\n" + prompt))
	return "legacy_" + hex.EncodeToString(hash[:])
}

// CAS updates preserve concurrent prompt edits and never replace an entire stale node.
func (h *Handler) mutateWorkflowNode(j workflowJob, change func(map[string]any) bool) error {
	if j.Node == "" {
		return nil
	}
	for attempt := 0; attempt < 5; attempt++ {
		var raw string
		if err := h.db.QueryRow(`SELECT content FROM nodes WHERE id=? AND canvas_id=?`, j.Node, j.Canvas).Scan(&raw); err != nil {
			return err
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			return err
		}
		if !change(data) {
			return nil
		}
		encoded, err := json.Marshal(data)
		if err != nil {
			return err
		}
		result, err := h.db.Exec(`UPDATE nodes SET content=?,updated_at=datetime('now','localtime') WHERE id=? AND canvas_id=? AND content=?`, string(encoded), j.Node, j.Canvas, raw)
		if err != nil {
			return err
		}
		count, _ := result.RowsAffected()
		if count == 1 {
			return nil
		}
	}
	return fmt.Errorf("节点正在被修改，稍后重试回填")
}

func (h *Handler) bindWorkflowJob(j workflowJob) error {
	return h.mutateWorkflowNode(j, func(data map[string]any) bool {
		data["workflowJobID"] = j.ID
		data["lastPromptId"] = j.Prompt
		data["jobStatus"] = j.Status
		return true
	})
}

func (h *Handler) publishWorkflowJob(j workflowJob, outputs []map[string]string) error {
	return h.mutateWorkflowNode(j, func(data map[string]any) bool {
		if data["workflowJobID"] != j.ID && !(data["workflowJobID"] == nil && data["lastPromptId"] == j.Prompt) {
			return false
		}
		published := []map[string]string{}
		for i, o := range outputs {
			kind := o["kind"]
			value := o["url"]
			if kind == "text" {
				value = o["value"]
			}
			published = append(published, map[string]string{"name": fmt.Sprintf("输出%d", i+1), "type": kind, "value": value, "sourceURL": o["source_url"]})
		}
		data["outputs"] = published
		data["lastPromptId"] = j.Prompt
		data["workflowJobID"] = j.ID
		data["lastCompletedPromptId"] = j.Prompt
		data["jobStatus"] = "done"
		return true
	})
}

func (h *Handler) importWorkflowJobs() error {
	rows, err := h.db.Query(`SELECT id,canvas_id,content FROM nodes WHERE node_type='workflow'`)
	if err != nil {
		return err
	}
	jobs := []workflowJob{}
	for rows.Next() {
		var id, canvas, raw string
		if err = rows.Scan(&id, &canvas, &raw); err != nil {
			rows.Close()
			return err
		}
		var data map[string]any
		if json.Unmarshal([]byte(raw), &data) != nil {
			continue
		}
		prompt, _ := data["lastPromptId"].(string)
		server, _ := data["serverUrl"].(string)
		if prompt == "" || server == "" {
			continue
		}
		jobs = append(jobs, workflowJob{ID: legacyWorkflowID(server, prompt), Server: server, Prompt: prompt, Node: id, Canvas: canvas, Status: "waiting"})
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, j := range jobs {
		if err = h.insertWorkflowJob(j); err != nil {
			return err
		}
	}
	return nil
}

// Preserve job ownership and completed outputs against stale/old frontend saves.
func (h *Handler) protectWorkflowJobWrite(previous, incoming string) (string, error) {
	var old, data map[string]any
	if json.Unmarshal([]byte(previous), &old) != nil || json.Unmarshal([]byte(incoming), &data) != nil {
		return incoming, nil
	}
	id, _ := old["workflowJobID"].(string)
	if id == "" {
		return incoming, nil
	}
	var status, local, prompt string
	if err := h.db.QueryRow(`SELECT status,local_json,prompt_id FROM workflow_jobs WHERE id=?`, id).Scan(&status, &local, &prompt); err != nil {
		return "", err
	}
	data["workflowJobID"] = id
	data["lastPromptId"] = prompt
	data["jobStatus"] = status
	if status == "done" {
		var output []map[string]string
		if err := json.Unmarshal([]byte(local), &output); err != nil {
			return "", err
		}
		published := []map[string]string{}
		for i, o := range output {
			value := o["url"]
			if o["kind"] == "text" {
				value = o["value"]
			}
			published = append(published, map[string]string{"name": fmt.Sprintf("输出%d", i+1), "type": o["kind"], "value": value, "sourceURL": o["source_url"]})
		}
		data["outputs"] = published
		data["lastCompletedPromptId"] = prompt
	} else {
		data["outputs"] = old["outputs"]
	}
	encoded, err := json.Marshal(data)
	return string(encoded), err
}
