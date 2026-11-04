package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// probeTimeout 是探测上游的超时：服务商接口慢的时候不该把界面卡住。
const probeTimeout = 20 * time.Second

// httpClient 是探测专用的客户端；自带超时，不复用默认客户端。
var httpClient = &http.Client{Timeout: probeTimeout}

// Probe 向上游问一次模型清单：三种协议各有自己的端点与鉴权头，这里只做「列出模型」。
// 界面点「测试连接」要的是「通不通、能看到几个模型」，不是完整代理。
func (m *Models) Probe(req domain.ProviderProbeREQ) domain.ProviderProbeRESP {
	baseUrl, apiProtocol, apiKey := m.resolveProbe(req)
	if baseUrl == "" {
		return domain.ProviderProbeRESP{ProbeMessage: "没有可用的接口地址：请先填写或保存服务商配置"}
	}
	url, headers := probeRequest(baseUrl, apiProtocol, apiKey)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return domain.ProviderProbeRESP{ProbeMessage: "接口地址不合法：" + err.Error()}
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return domain.ProviderProbeRESP{ProbeMessage: "连不上：" + err.Error()}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.ProviderProbeRESP{
			ProbeMessage: "上游返回 " + response.Status + "：" + pkg.Truncate(strings.TrimSpace(string(body)), 300),
		}
	}
	models := parseModelList(body)
	return domain.ProviderProbeRESP{
		ProbeOk:      true,
		ModelCount:   len(models),
		Models:       models,
		ProbeMessage: "连通，可见 " + itoa(len(models)) + " 个模型",
	}
}

// probeRequest 按协议给模型清单的地址与鉴权头。
func probeRequest(baseUrl, apiProtocol, apiKey string) (string, map[string]string) {
	trimmed := strings.TrimSuffix(baseUrl, "/")
	headers := map[string]string{"Accept": "application/json"}
	switch apiProtocol {
	case "anthropic":
		url := trimmed + "/v1/models"
		if apiKey != "" {
			headers["x-api-key"] = apiKey
			headers["anthropic-version"] = "2023-06-01"
		}
		return url, headers
	case "google":
		url := trimmed + "/v1beta/models"
		if apiKey != "" {
			url += "?key=" + apiKey
		}
		return url, headers
	default:
		url := trimmed + "/v1/models"
		if apiKey != "" {
			headers["Authorization"] = "Bearer " + apiKey
		}
		return url, headers
	}
}

// parseModelList 兼容三种协议的返回形状：data[].id / models[].name / 裸数组。
func parseModelList(body []byte) []domain.ProviderModelEntryVO {
	var envelope struct {
		Data   []map[string]any `json:"data"`
		Models []map[string]any `json:"models"`
	}
	out := make([]domain.ProviderModelEntryVO, 0, 16)
	if err := json.Unmarshal(body, &envelope); err == nil {
		for _, item := range envelope.Data {
			out = append(out, modelEntryFromList(item, "id"))
		}
		for _, item := range envelope.Models {
			out = append(out, modelEntryFromList(item, "name"))
		}
		if len(out) > 0 {
			return out
		}
	}
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err == nil {
		for _, item := range list {
			out = append(out, modelEntryFromList(item, "id"))
		}
	}
	return out
}

// modelEntryFromList 从一条上游模型记录里取我们要的两个字段。
func modelEntryFromList(item map[string]any, idKey string) domain.ProviderModelEntryVO {
	id := stringField(item, idKey)
	// google 的 name 形如 models/gemini-1.5-pro：去掉前缀才是模型 ID。
	if idx := strings.LastIndex(id, "/"); idx >= 0 {
		id = id[idx+1:]
	}
	entry := domain.ProviderModelEntryVO{ModelId: id, ModelName: stringField(item, "displayName")}
	if entry.ModelName == "" {
		entry.ModelName = id
	}
	return entry
}

// itoa 十进制转换（避免为一个数字引入 strconv）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 12)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
