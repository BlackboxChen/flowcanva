package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) ComfyUIWorkflowCapabilities(c *gin.Context) {
	c.JSON(200, gin.H{"workflow_variables": true, "image_variables": true, "keyframe_approval": true, "persistent_jobs": true, "durable_media": true})
}

// Resolve canvas media URLs before substituting them into a ComfyUI prompt.
func resolveWorkflowVariables(c *gin.Context, serverURL string, raw json.RawMessage, variables map[string]any, imageNames []string) (json.RawMessage, error) {
	values := map[string]any{"run_id": uuid.NewString()}
	for name, value := range variables {
		values[name] = value
	}
	for _, name := range imageNames {
		source, ok := values[name].(string)
		if !ok || source == "" {
			return nil, fmt.Errorf("图片变量 %s 为空，请选择已确认的关键帧", name)
		}
		// Existing ComfyUI input filenames need no upload.
		if !strings.HasPrefix(source, "/") && !strings.HasPrefix(source, "data:") && !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
			continue
		}
		files, err := uploadComfyImageReferences(c, serverURL, []string{source})
		if err != nil {
			return nil, fmt.Errorf("图片变量 %s: %w", name, err)
		}
		values[name] = files[0]
	}
	return replaceComfyWorkflowValues(string(raw), values)
}
