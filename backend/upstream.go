package main

import (
	"errors"
	"strings"
)

var errAPIKeyNotConfigured = errors.New("api_key 未配置，请在管理台节点配置中设置，或配置 ZHENGAPI_API_KEY")
var errImageAPIKeyNotConfigured = errors.New("图片 api_key 未配置，请在管理台节点配置中设置，或配置 GEEKNOW_API_KEY")
var errMegaBYAPIKeyNotConfigured = errors.New("MegaBY 视频 api_key 未配置，请在管理台节点配置中设置，或配置 MEGABY_API_KEY")

func (h *Handler) resolveVideoUpstream(req VideoGenRequest) (string, string, error) {
	if isMegaBYVideoModel(req.Model) || req.Protocol == "megaby_video" {
		if strings.TrimSpace(req.APIKey) != "" {
			baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
			if baseURL == "" {
				baseURL = "https://newapi.megabyai.cc"
			}
			return baseURL, req.APIKey, nil
		}
		if h.cfg.MegaBYBaseURL == "" || h.cfg.MegaBYAPIKey == "" {
			return "", "", errMegaBYAPIKeyNotConfigured
		}
		return strings.TrimRight(h.cfg.MegaBYBaseURL, "/"), h.cfg.MegaBYAPIKey, nil
	}
	return h.resolveUpstream(req.BaseURL, req.APIKey, "https://api.nxfl.cc", false)
}

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

func (h *Handler) resolveImageUpstream(requestBaseURL, requestAPIKey string) (string, string, error) {
	if strings.TrimSpace(requestAPIKey) != "" {
		baseURL := strings.TrimRight(strings.TrimSpace(requestBaseURL), "/")
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return baseURL, requestAPIKey, nil
	}
	if h.cfg.GeekNowBaseURL == "" || h.cfg.GeekNowAPIKey == "" {
		return "", "", errImageAPIKeyNotConfigured
	}
	return normalizeVersionedBaseURL(h.cfg.GeekNowBaseURL), h.cfg.GeekNowAPIKey, nil
}

func normalizeVersionedBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if !strings.HasSuffix(strings.ToLower(baseURL), "/v1") {
		baseURL += "/v1"
	}
	return baseURL
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
