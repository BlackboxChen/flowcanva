package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestBatchDependenciesApprovalAndFailureIsolation(t *testing.T) {
	h, r := collaborationTestHandler(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		var data map[string]any
		json.NewDecoder(req.Body).Decode(&data)
		json.NewEncoder(w).Encode(map[string]any{"prompt_id": data["prompt_id"]})
	}))
	defer upstream.Close()
	for _, id := range []string{"a", "b", "child", "review", "failedchild"} {
		data, _ := json.Marshal(map[string]any{"serverUrl": upstream.URL, "workflowJson": `{"1":{"class_type":"Fake","inputs":{"seed":3}}}`, "approvalRequired": id == "review", "seedMode": "random"})
		testNode(t, h, id, "workflow", string(data))
	}
	for _, edge := range [][3]string{{"e1", "a", "child"}, {"e2", "a", "review"}, {"e3", "b", "failedchild"}} {
		if _, err := h.db.Exec(`INSERT INTO edges(id,canvas_id,source_node_id,target_node_id) VALUES(?,'cv',?,?)`, edge[0], edge[1], edge[2]); err != nil {
			t.Fatal(err)
		}
	}
	res := testRequest(r, "POST", "/api/canvases/cv/batch", map[string]any{"node_ids": []string{"a", "b", "child", "review", "failedchild"}})
	if res.Code != 201 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	tasks, _ := h.batchTasks("cv")
	for _, task := range tasks {
		if task.Node == "child" {
			h.advanceBatchTask(task, tasks)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("downstream ran before dependency")
	}
	for _, task := range tasks {
		if task.Node == "a" || task.Node == "b" {
			h.advanceBatchTask(task, tasks)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("roots not submitted")
	}
	if _, err := h.db.Exec(`UPDATE workflow_jobs SET status=CASE node_id WHEN 'a' THEN 'done' ELSE 'failed' END`); err != nil {
		t.Fatal(err)
	}
	tasks, _ = h.batchTasks("cv")
	for _, task := range tasks {
		if task.Node == "a" || task.Node == "b" {
			h.advanceBatchTask(task, tasks)
		}
	}
	tasks, _ = h.batchTasks("cv")
	for _, task := range tasks {
		if task.Node != "a" && task.Node != "b" {
			h.advanceBatchTask(task, tasks)
		}
	}
	tasks, _ = h.batchTasks("cv")
	states := map[string]string{}
	for _, task := range tasks {
		states[task.Node] = task.Status
	}
	if states["child"] != "running" || states["review"] != "review" || states["failedchild"] != "blocked" || calls.Load() != 3 {
		t.Fatalf("states=%v calls=%d", states, calls.Load())
	}
}

func TestBatchRejectsCycles(t *testing.T) {
	if validateBatchDAG([]batchTask{{Node: "a", Dependencies: `["b"]`}, {Node: "b", Dependencies: `["a"]`}}) == nil {
		t.Fatal("cycle accepted")
	}
}

func TestBatchChangedPromptIsNotSilentlyExecuted(t *testing.T) {
	h, r := collaborationTestHandler(t)
	testNode(t, h, "a", "workflow", `{"serverUrl":"http://unused","workflowJson":"{}","name":"old"}`)
	res := testRequest(r, "POST", "/api/canvases/cv/batch", map[string]any{"node_ids": []string{"a"}})
	if res.Code != 201 {
		t.Fatal(res.Body.String())
	}
	h.db.Exec(`UPDATE nodes SET content='{"serverUrl":"http://unused","workflowJson":"{}","name":"new"}' WHERE id='a'`)
	tasks, _ := h.batchTasks("cv")
	h.advanceBatchTask(tasks[0], tasks)
	tasks, _ = h.batchTasks("cv")
	if tasks[0].Status != "blocked" {
		t.Fatal(tasks[0])
	}
}
