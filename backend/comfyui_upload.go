package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func uploadComfyImageReferences(c *gin.Context, serverURL string, sources []string) ([]string, error) {
	if len(sources) == 0 {
		return []string{}, nil
	}
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	uploaded := make([]string, 0, len(sources))
	for _, source := range sources {
		content, filename, err := readComfyImageSource(c, source)
		if err != nil {
			return nil, fmt.Errorf("读取参考图失败: %w", err)
		}
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		file, err := writer.CreateFormFile("image", filename)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write(content); err != nil {
			return nil, err
		}
		_ = writer.WriteField("overwrite", "false")
		if err := writer.Close(); err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, serverURL+"/upload/image", &body)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err := llmClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("上传参考图到 ComfyUI 失败: %w", err)
		}
		var result struct {
			Name      string `json:"name"`
			Subfolder string `json:"subfolder"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil || resp.StatusCode >= 400 || result.Name == "" {
			return nil, fmt.Errorf("ComfyUI 图片上传失败 (%d)", resp.StatusCode)
		}
		if result.Subfolder != "" {
			result.Name = path.Join(result.Subfolder, result.Name)
		}
		uploaded = append(uploaded, result.Name)
	}
	return uploaded, nil
}

func readComfyImageSource(c *gin.Context, source string) ([]byte, string, error) {
	if strings.HasPrefix(source, "/uploads/") {
		file, err := localWorkflowFile(source)
		if err != nil {
			return nil, "", err
		}
		data, err := os.ReadFile(file)
		return data, filepath.Base(file), err
	}
	if strings.HasPrefix(strings.ToLower(source), "data:") {
		parts := strings.SplitN(source, ",", 2)
		if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
			return nil, "", fmt.Errorf("不支持的图片 data URL")
		}
		mediaType, _, _ := mime.ParseMediaType(strings.TrimSuffix(strings.TrimPrefix(parts[0], "data:"), ";base64"))
		data, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, "", err
		}
		return data, "reference" + imageExtension(mediaType), nil
	}
	target := source
	if strings.HasPrefix(source, "/") {
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		target = scheme + "://" + c.Request.Host + source
	}
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, "", fmt.Errorf("参考图地址必须是 HTTP(S) URL 或 data URL")
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := llmClient.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("读取参考图返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	filename := path.Base(parsed.Path)
	// ComfyUI /view serves the image named in the query, not a file named view.
	if queryName := parsed.Query().Get("filename"); queryName != "" {
		filename = path.Base(queryName)
	}
	if path.Ext(filename) == "" {
		extensions, _ := mime.ExtensionsByType(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
		if len(extensions) > 0 {
			filename += extensions[0]
		} else {
			filename += ".png"
		}
	}
	if filename == "." || filename == "/" || filename == "" {
		filename = "reference.png"
	}
	return data, filename, nil
}
