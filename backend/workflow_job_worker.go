package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

var workflowJobsRunning sync.Map

func (h *Handler) startWorkflowJobs() error {
	if _, err := h.db.Exec(`UPDATE workflow_jobs SET status='submit_unknown' WHERE status='submitting'`); err != nil {
		return err
	}
	if err := h.importWorkflowJobs(); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		slots := make(chan struct{}, 2)
		for range ticker.C {
			rows, err := h.db.Query(`SELECT server_url,prompt_id FROM workflow_jobs WHERE status NOT IN ('done','failed','submitting') AND next_retry<=? ORDER BY next_retry,created_at LIMIT 100`, time.Now().Unix())
			if err != nil {
				h.log.Error("workflow scheduler query failed", "error", err)
				continue
			}
			jobs := [][2]string{}
			for rows.Next() {
				var server, prompt string
				if rows.Scan(&server, &prompt) == nil {
					jobs = append(jobs, [2]string{server, prompt})
				}
			}
			rows.Close()
			for _, pair := range jobs {
				j, err := h.readWorkflowJob(pair[0], pair[1])
				if err != nil {
					continue
				}
				if _, busy := workflowJobsRunning.LoadOrStore(j.ID, true); busy {
					continue
				}
				select {
				case slots <- struct{}{}:
					go func(job workflowJob) {
						defer func() { <-slots; workflowJobsRunning.Delete(job.ID) }()
						h.runWorkflowJob(job)
					}(j)
				default:
					workflowJobsRunning.Delete(j.ID)
				}
			}
		}
	}()
	return nil
}

func (h *Handler) retryWorkflowJob(j workflowJob, status string, err error) {
	message := ""
	if err != nil {
		message = err.Error()
		h.log.Warn("workflow job waiting", "job_id", j.ID, "state", status, "error", message)
	}
	delay := 5 + j.Attempts*5
	if delay > 60 {
		delay = 60
	}
	if _, dbErr := h.db.Exec(`UPDATE workflow_jobs SET status=?,error=?,attempts=attempts+1,next_retry=? WHERE id=?`, status, message, time.Now().Unix()+int64(delay), j.ID); dbErr != nil {
		h.log.Error("workflow retry persist failed", "job_id", j.ID, "error", dbErr)
	}
}

func (h *Handler) runWorkflowJob(j workflowJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if j.Status == "submit_unknown" {
		prompt, err := findSubmittedWorkflow(ctx, j)
		if err != nil || prompt == "" {
			if err == nil {
				err = fmt.Errorf("提交状态待确认；不会自动重新提交生成任务")
			}
			h.retryWorkflowJob(j, "submit_unknown", err)
			return
		}
		if _, err = h.db.Exec(`UPDATE workflow_jobs SET prompt_id=?,status='waiting' WHERE id=?`, prompt, j.ID); err != nil {
			h.retryWorkflowJob(j, "submit_unknown", err)
			return
		}
		j.Prompt = prompt
		j.Status = "waiting"
	}
	var local []map[string]string
	if json.Unmarshal([]byte(j.Local), &local) == nil && len(local) > 0 {
		valid := true
		for _, o := range local {
			if o["kind"] != "text" {
				if _, err := localWorkflowFile(o["url"]); err != nil {
					valid = false
					break
				}
			}
		}
		if valid {
			h.finishWorkflowJob(j, local)
			return
		}
	}
	var remote []map[string]string
	_ = json.Unmarshal([]byte(j.Remote), &remote)
	if len(remote) == 0 {
		result, done, err := fetchWorkflowHistory(ctx, j)
		if err != nil {
			if done {
				_, dbErr := h.db.Exec(`UPDATE workflow_jobs SET status='failed',error=? WHERE id=?`, err.Error(), j.ID)
				if dbErr != nil {
					h.log.Error("workflow failure persist failed", "error", dbErr)
				}
			} else {
				h.retryWorkflowJob(j, "waiting", err)
			}
			return
		}
		if !done {
			h.retryWorkflowJob(j, "waiting", nil)
			return
		}
		remote = result
		encoded, _ := json.Marshal(remote)
		if _, err = h.db.Exec(`UPDATE workflow_jobs SET remote_json=?,status='saving',error='' WHERE id=?`, string(encoded), j.ID); err != nil {
			h.retryWorkflowJob(j, "waiting", err)
			return
		}
	}
	local = []map[string]string{}
	for _, output := range remote {
		copy := map[string]string{}
		for k, v := range output {
			copy[k] = v
		}
		if output["kind"] != "text" {
			url, err := h.persistWorkflowURL(ctx, output["url"])
			if err != nil {
				h.retryWorkflowJob(j, "save_retry", err)
				return
			}
			copy["source_url"] = output["url"]
			copy["url"] = url
		}
		local = append(local, copy)
	}
	encoded, _ := json.Marshal(local)
	if _, err := h.db.Exec(`UPDATE workflow_jobs SET local_json=?,status='saving' WHERE id=?`, string(encoded), j.ID); err != nil {
		h.retryWorkflowJob(j, "save_retry", err)
		return
	}
	h.finishWorkflowJob(j, local)
}

func (h *Handler) finishWorkflowJob(j workflowJob, outputs []map[string]string) {
	// A deleted node must not discard its durable job results.
	var exists int
	if j.Node != "" {
		if err := h.db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id=? AND canvas_id=?`, j.Node, j.Canvas).Scan(&exists); err != nil {
			h.retryWorkflowJob(j, "save_retry", err)
			return
		}
	}
	if exists > 0 {
		if err := h.publishWorkflowJob(j, outputs); err != nil {
			h.retryWorkflowJob(j, "save_retry", err)
			return
		}
	}
	if _, err := h.db.Exec(`UPDATE workflow_jobs SET status='done',error='',next_retry=0 WHERE id=?`, j.ID); err != nil {
		h.log.Error("workflow completion persist failed", "job_id", j.ID, "error", err)
		return
	}
	h.log.Info("workflow job durably completed", "job_id", j.ID, "outputs", len(outputs))
}
