package main

import (
	"errors"
	"strings"
)

var errAPIKeyNotConfigured = errors.New("api_key 未配置，请在管理台节点配置中设置，或配置 ZHENGAPI_API_KEY")

// resolveUpstream keeps a node's URL and key together. If a node has no key,
// the ZhengAPI URL and key take over as one pair to avoid leaking credentials.
func (h *Handler) resolveUpstream(requestBaseURL, requestAPIKey, defaultBaseURL string, versioned bool) (string, string, error) {
	if strings.TrimSpace(requestAPIKey) != "" {
		baseURL := strings.TrimRight(strings.TrimSpace(requestBaseURL), "/")
		if baseURL == "" {
			baseURL = defaultBaseURL
		}
		return baseURL, requestAPIKey, nil
	}

	if h.cfg.ZhengAPIBaseURL == "" || h.cfg.ZhengAPIAPIKey == "" {
		return "", "", errAPIKeyNotConfigured
	}

	return normalizeZhengAPIBaseURL(h.cfg.ZhengAPIBaseURL, versioned), h.cfg.ZhengAPIAPIKey, nil
}

func normalizeZhengAPIBaseURL(baseURL string, versioned bool) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	hasV1 := strings.HasSuffix(strings.ToLower(baseURL), "/v1")
	if versioned && !hasV1 {
		return baseURL + "/v1"
	}
	if !versioned && hasV1 {
		return baseURL[:len(baseURL)-len("/v1")]
	}
	return baseURL
}
