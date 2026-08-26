package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (h *Handler) uploadDataURLToOSS(dataURL string) (string, error) {
	comma := strings.Index(dataURL, ",")
	if comma < 0 {
		return "", fmt.Errorf("invalid data URL")
	}
	meta, encoded := dataURL[:comma], dataURL[comma+1:]
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode data URL: %w", err)
	}
	contentType := "application/octet-stream"
	if strings.HasPrefix(meta, "data:") && strings.Contains(meta, ";") {
		contentType = strings.TrimPrefix(strings.SplitN(meta, ";", 2)[0], "data:")
	}
	ext := ".bin"
	switch contentType {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	}
	key := fmt.Sprintf("flowcanva/%d%s", time.Now().UnixNano(), ext)
	// OSS 使用桶三级域名访问，但签名规范仍包含 bucket/object 资源路径。
	resource := "/" + h.cfg.OSSBucket + "/" + key
	date := time.Now().UTC().Format(http.TimeFormat)
	md5sum := md5.Sum(data)
	contentMD5 := base64.StdEncoding.EncodeToString(md5sum[:])
	stringToSign := "PUT\n" + contentMD5 + "\n" + contentType + "\n" + date + "\n" + resource
	mac := hmac.New(sha1.New, []byte(h.cfg.OSSAccessKeySecret))
	_, _ = mac.Write([]byte(stringToSign))
	auth := "OSS " + h.cfg.OSSAccessKeyID + ":" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	endpoint := strings.TrimRight(h.cfg.OSSEndpoint, "/")
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || parsedEndpoint.Host == "" {
		return "", fmt.Errorf("invalid OSS endpoint")
	}
	requestHost := h.cfg.OSSBucket + "." + parsedEndpoint.Host
	requestURL := parsedEndpoint.Scheme + "://" + requestHost + "/" + key
	req, err := http.NewRequest(http.MethodPut, requestURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-MD5", contentMD5)
	req.Header.Set("Date", date)
	req.Header.Set("Authorization", auth)
	resp, err := llmClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("OSS 返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return requestURL, nil
}
