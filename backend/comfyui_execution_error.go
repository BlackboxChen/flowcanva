package main

import "fmt"

// Report the node error without exposing the full task inputs or tensor dumps.
func comfyExecutionError(status map[string]interface{}) string {
	messages, _ := status["messages"].([]interface{})
	for i := len(messages) - 1; i >= 0; i-- {
		entry, ok := messages[i].([]interface{})
		if !ok || len(entry) < 2 || entry[0] != "execution_error" {
			continue
		}
		details, ok := entry[1].(map[string]interface{})
		if !ok {
			continue
		}
		message, _ := details["exception_message"].(string)
		if len(message) > 2000 {
			message = message[:2000]
		}
		if message != "" {
			return fmt.Sprintf("ComfyUI 节点 %v（%v）执行失败：%s", details["node_id"], details["node_type"], message)
		}
	}
	return "ComfyUI 任务执行失败，请查看服务器执行日志"
}
