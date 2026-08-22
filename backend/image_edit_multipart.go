package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxImageReferences   = 16
	maxImagePayloadBytes = int64(512 * 1024 * 1024)
)

type imageReference struct {
	reader      io.ReadCloser
	contentType string
	extension   string
}

func writeImageEditMultipart(ctx context.Context, req ImageGenRequest, references []string) (*os.File, string, error) {
	if len(references) == 0 {
		return nil, "", fmt.Errorf("图片编辑至少需要一张参考图")
	}
	if len(references) > maxImageReferences {
		return nil, "", fmt.Errorf("参考图最多支持 %d 张", maxImageReferences)
	}

	tmp, err := os.CreateTemp("", "flowcanva-image-edit-*.multipart")
	if err != nil {
		return nil, "", err
	}
	removeOnError := func(cause error) (*os.File, string, error) {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, "", cause
	}

	writer := multipart.NewWriter(tmp)
	fields := map[string]string{
		"model": req.Model, "prompt": req.Prompt, "size": req.Size,
		"n": fmt.Sprintf("%d", max(req.N, 1)), "response_format": "url",
	}
	for key, value := range fields {
		if value != "" {
			if err := writer.WriteField(key, value); err != nil {
				return removeOnError(err)
			}
		}
	}

	var total int64
	for index, value := range references {
		ref, err := openImageReference(ctx, strings.TrimSpace(value))
		if err != nil {
			return removeOnError(fmt.Errorf("读取参考图 %d 失败: %w", index+1, err))
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="input-%d%s"`, index+1, ref.extension))
		header.Set("Content-Type", ref.contentType)
		part, partErr := writer.CreatePart(header)
		if partErr == nil {
			remaining := maxImagePayloadBytes - total
			if remaining <= 0 {
				partErr = fmt.Errorf("参考图总大小超过 512 MiB")
			} else {
				written, copyErr := io.Copy(part, io.LimitReader(ref.reader, remaining+1))
				total += written
				if copyErr != nil {
					partErr = copyErr
				} else if total > maxImagePayloadBytes {
					partErr = fmt.Errorf("参考图总大小超过 512 MiB")
				}
			}
		}
		_ = ref.reader.Close()
		if partErr != nil {
			return removeOnError(partErr)
		}
	}
	if err := writer.Close(); err != nil {
		return removeOnError(err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return removeOnError(err)
	}
	return tmp, writer.FormDataContentType(), nil
}

func openImageReference(ctx context.Context, value string) (imageReference, error) {
	if strings.HasPrefix(value, "data:") {
		return openDataImage(value)
	}
	if strings.HasPrefix(value, "/uploads/") {
		return openLocalImage(value)
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return imageReference{}, fmt.Errorf("只支持 HTTP(S) 图片地址或完整 Data URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
	if err != nil {
		return imageReference{}, err
	}
	response, err := imageGenerationClient.Do(request)
	if err != nil {
		return imageReference{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return imageReference{}, fmt.Errorf("图片服务器返回 %d", response.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if !strings.HasPrefix(contentType, "image/") {
		response.Body.Close()
		return imageReference{}, fmt.Errorf("响应不是图片: %s", contentType)
	}
	return imageReference{reader: response.Body, contentType: contentType, extension: imageExtension(contentType)}, nil
}

func openLocalImage(value string) (imageReference, error) {
	relative := filepath.Clean(strings.TrimPrefix(value, "/uploads/"))
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return imageReference{}, fmt.Errorf("无效的本地图片路径")
	}
	file, err := os.Open(filepath.Join("uploads", relative))
	if err != nil {
		return imageReference{}, err
	}
	buffer := make([]byte, 512)
	read, readErr := file.Read(buffer)
	if readErr != nil && readErr != io.EOF {
		file.Close()
		return imageReference{}, readErr
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return imageReference{}, err
	}
	contentType := http.DetectContentType(buffer[:read])
	if !strings.HasPrefix(contentType, "image/") {
		file.Close()
		return imageReference{}, fmt.Errorf("本地文件不是图片")
	}
	return imageReference{reader: file, contentType: contentType, extension: imageExtension(contentType)}, nil
}

func openDataImage(value string) (imageReference, error) {
	comma := strings.IndexByte(value, ',')
	if comma < 0 {
		return imageReference{}, fmt.Errorf("Data URL 缺少数据部分")
	}
	metadata, encoded := value[5:comma], value[comma+1:]
	parts := strings.Split(metadata, ";")
	contentType := strings.ToLower(parts[0])
	if !strings.HasPrefix(contentType, "image/") {
		return imageReference{}, fmt.Errorf("Data URL 不是图片")
	}
	if len(parts) < 2 || parts[len(parts)-1] != "base64" {
		return imageReference{}, fmt.Errorf("图片 Data URL 必须使用 base64 编码")
	}
	reader := io.NopCloser(base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded)))
	return imageReference{reader: reader, contentType: contentType, extension: imageExtension(contentType)}, nil
}

func imageExtension(contentType string) string {
	if extensions, _ := mime.ExtensionsByType(contentType); len(extensions) > 0 {
		return extensions[0]
	}
	if contentType == "image/jpeg" {
		return ".jpg"
	}
	return ".png"
}
