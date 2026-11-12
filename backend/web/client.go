// Package web 是联网能力的底层：抓取与解析。
// 它只依赖标准库与 pkg，不知道模型、工具与界面的存在。
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"WorkBaby/backend/pkg"
)

// 默认参数：抓一个网页不该让模型等太久，也不该把整本书塞进上下文。
const (
	// DefaultTimeout 是单次请求的总超时
	DefaultTimeout = 30 * time.Second
	// MaxResponseBytes 是响应体的读取上限
	MaxResponseBytes = 8 << 20
	// MaxRedirects 是跟随重定向的次数上限
	MaxRedirects = 5
)

// Client 是联网抓取客户端。
// 复用连接池：一次搜索往往要连好几个站点，复用连接省掉重复握手。
type Client struct {
	// http 底层客户端
	http *http.Client
	// userAgent 请求标识
	userAgent string
}

// NewClient 造一个联网客户端。
func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: DefaultTimeout,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
				DialContext:         safeDial,
			},
			// 重定向自己处理：默认实现会跟到内网地址，
			// 而我们要逐跳校验目标地址是否安全。
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= MaxRedirects {
					return fmt.Errorf("重定向次数过多（上限 %d）", MaxRedirects)
				}
				return nil
			},
		},
		userAgent: "WorkBaby/1.0 (+https://github.com/workbaby)",
	}
}

// Response 是一次抓取的结果。
type Response struct {
	// StatusCode HTTP 状态码
	StatusCode int `json:"status_code"`
	// Body 响应正文（已按上限截断）
	Body string `json:"body"`
	// ContentType 响应类型
	ContentType string `json:"content_type"`
	// FinalURL 跟随重定向后的最终地址
	FinalURL string `json:"final_url"`
	// Truncated 正文是否被截断
	Truncated bool `json:"truncated"`
}

// Get 抓取一个地址。
// 每一跳重定向都要过 SSRF 校验：只查首地址的话，
// 公网地址跳到 127.0.0.1 就绕过去了。
func (c *Client) Get(ctx context.Context, rawURL string) (Response, error) {
	current := strings.TrimSpace(rawURL)
	for hop := 0; hop <= MaxRedirects; hop++ {
		if err := ValidateURL(current); err != nil {
			return Response{}, err
		}
		response, err := c.get(ctx, current, map[string]string{
			"Accept": "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8",
		})
		if err != nil {
			return Response{}, err
		}
		// 有重定向就自己跳，不让默认实现接手。
		if location := response.Header.Get("Location"); location != "" &&
			response.StatusCode >= 300 && response.StatusCode < 400 {
			response.Body.Close()
			next, err := response.Location()
			if err != nil {
				return Response{}, pkg.Err(pkg.CodeProvision, "重定向地址不合法", err.Error())
			}
			current = next.String()
			continue
		}
		defer response.Body.Close()
		body, truncated, err := readCapped(response.Body)
		if err != nil {
			return Response{}, pkg.Err(pkg.CodeProvision, "读取响应失败", err.Error())
		}
		return Response{
			StatusCode:  response.StatusCode,
			Body:        body,
			ContentType: response.Header.Get("Content-Type"),
			FinalURL:    current,
			Truncated:   truncated,
		}, nil
	}
	return Response{}, pkg.Err(pkg.CodeProvision, "重定向次数过多", "")
}

// get 发一次 GET 并交回原始响应；body 由调用方关闭。
func (c *Client) get(ctx context.Context, rawURL string, headers map[string]string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, pkg.Err(pkg.CodeBadRequest, "地址不合法", err.Error())
	}
	request.Header.Set("User-Agent", c.userAgent)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, pkg.Err(pkg.CodeProvision, "抓取失败", err.Error())
	}
	return response, nil
}

// GetJSON 抓取 JSON。
func (c *Client) GetJSON(ctx context.Context, rawURL string, headers map[string]string) (Response, error) {
	if err := ValidateURL(rawURL); err != nil {
		return Response{}, err
	}
	merged := map[string]string{"Accept": "application/json"}
	maps.Copy(merged, headers)
	response, err := c.get(ctx, rawURL, merged)
	if err != nil {
		return Response{}, err
	}
	defer response.Body.Close()
	body, truncated, err := readCapped(response.Body)
	if err != nil {
		return Response{}, pkg.Err(pkg.CodeProvision, "读取响应失败", err.Error())
	}
	return Response{
		StatusCode:  response.StatusCode,
		Body:        body,
		ContentType: response.Header.Get("Content-Type"),
		FinalURL:    rawURL,
		Truncated:   truncated,
	}, nil
}

// PostJSON 提交一份 JSON 并读回 JSON。
func (c *Client) PostJSON(ctx context.Context, rawURL string, payload any, headers map[string]string) (Response, error) {
	if err := ValidateURL(rawURL); err != nil {
		return Response{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Response{}, pkg.Err(pkg.CodeBadRequest, "请求体编码失败", err.Error())
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(string(encoded)))
	if err != nil {
		return Response{}, pkg.Err(pkg.CodeBadRequest, "请求构造失败", err.Error())
	}
	request.Header.Set("User-Agent", c.userAgent)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return Response{}, pkg.Err(pkg.CodeProvision, "请求失败", err.Error())
	}
	defer response.Body.Close()
	body, truncated, err := readCapped(response.Body)
	if err != nil {
		return Response{}, pkg.Err(pkg.CodeProvision, "读取响应失败", err.Error())
	}
	return Response{
		StatusCode:  response.StatusCode,
		Body:        body,
		ContentType: response.Header.Get("Content-Type"),
		FinalURL:    rawURL,
		Truncated:   truncated,
	}, nil
}

// readCapped 读响应体并截断到上限。
// 截断而不是报错：半个页面对模型也有用，报错则什么都拿不到。
func readCapped(reader io.Reader) (string, bool, error) {
	buffer := make([]byte, 0, 64<<10)
	chunk := make([]byte, 32<<10)
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			buffer = append(buffer, chunk[:n]...)
			if len(buffer) >= MaxResponseBytes {
				return string(buffer[:MaxResponseBytes]), true, nil
			}
		}
		if err != nil {
			// 读完了不是错误：正常收尾与截断都要拿到内容。
			if errors.Is(err, io.EOF) {
				return string(buffer), false, nil
			}
			return string(buffer), false, err
		}
	}
}

// ValidateURL 校验出站地址：只允许 http/https，且主机不是内网地址。
// 这是 SSRF 的第一道门：模型给的地址来自对话内容，不能让它读到本机服务。
func ValidateURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return pkg.Err(pkg.CodeBadRequest, "地址不能为空")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return pkg.Err(pkg.CodeBadRequest, "地址不合法", err.Error())
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return pkg.Err(pkg.CodeBadRequest, "只支持 http 与 https 地址", "协议："+parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "" {
		return pkg.Err(pkg.CodeBadRequest, "地址里没有主机名")
	}
	if IsPrivateHost(host) {
		return pkg.Err(pkg.CodePathOutside, "不允许访问内网地址", host)
	}
	return nil
}

// IsPrivateHost 判断主机是否指向内网或本机。
// 域名先做一次解析：域名可以指向 127.0.0.1，光看字符串拦不住。
func IsPrivateHost(host string) bool {
	lowered := strings.ToLower(strings.TrimSpace(host))
	if lowered == "localhost" || strings.HasSuffix(lowered, ".localhost") ||
		lowered == "localhost.localdomain" {
		return true
	}
	if ip := net.ParseIP(lowered); ip != nil {
		return isPrivateIP(ip)
	}
	// 域名不在这里判：解析结果由 safeDial 在真正连接时复核。
	// 在这里解析会引入一次阻塞 DNS 往返，而连接时本来就要解析一次。
	return false
}

// isPrivateIP 判断一个 IP 是否属于内网、环回、链路本地或保留段。
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// CGNAT 段（100.64.0.0/10）常被用来做内网反代，一并拦掉。
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return false
}

// safeDial 是带内网复核的拨号器。
// 这里才是 SSRF 的最终闸门：域名可能解析到内网地址，
// 而复核的实际连接地址骗不过去。
func safeDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return nil, fmt.Errorf("拒绝连接内网地址 %s", host)
		}
		return dialer.DialContext(ctx, network, address)
	}
	addresses, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range addresses {
		if isPrivateIP(ip) {
			lastErr = fmt.Errorf("域名 %s 解析到内网地址 %s，拒绝连接", host, ip)
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("域名 %s 没有可用地址", host)
	}
	return nil, lastErr
}
