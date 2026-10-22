// Package netx 负责"出口路径"的解析：系统代理探测、手动代理、直连，
// 并据此构建带分阶段超时的 http.Transport。
package netx

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Settings 描述本次运行实际的出网方式。
type Settings struct {
	Mode      string   `json:"mode"`
	Effective string   `json:"effective"` // 空表示直连
	Source    string   `json:"source"`    // os | manual | direct
	Detail    string   `json:"detail"`    // 给人看的一句话
	Warnings  []string `json:"warnings"`
}

// Resolve 依据模式解析出实际代理。
//
// 关于环境变量：系统模式只认操作系统代理设置，刻意忽略 HTTP_PROXY/HTTPS_PROXY，
// 以免把 CLI/终端里注入的变量误当成"系统代理"，导致测量口径与用户认知不一致。
func Resolve(mode, raw string) (Settings, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	s := Settings{Mode: mode, Warnings: []string{}}

	if env := firstEnvProxy(); env != "" {
		s.Warnings = append(s.Warnings,
			"检测到环境变量代理 "+maskURL(env)+"，系统模式下不会使用它（避免误测终端注入的代理），如需测试请切到自定义代理")
	}

	switch mode {
	case "custom":
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			return s, fmt.Errorf("代理地址无法解析: %w", err)
		}
		s.Effective = u.String()
		s.Source = "manual"
		s.Detail = fmt.Sprintf("手动代理 · %s · %s", strings.ToLower(u.Scheme), maskURL(u.String()))
		return s, nil
	case "direct":
		s.Effective = ""
		s.Source = "direct"
		s.Detail = "强制直连（绕过所有代理），用于与代理结果对照"
		return s, nil
	default:
		found, detail := DetectSystem()
		if found != "" {
			s.Effective = found
			s.Source = "os"
			s.Detail = "系统代理 · " + detail
			return s, nil
		}
		s.Effective = ""
		s.Source = "direct"
		s.Detail = "未检测到系统代理设置，按系统默认路由直接出网（TUN / 透明代理场景下属正常）"
		return s, nil
	}
}

func firstEnvProxy() string {
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// maskURL 把 URL 中的密码打码，避免进入日志/界面/报告。
func maskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.User != nil {
		user := u.User.Username()
		if _, ok := u.User.Password(); ok {
			u.User = url.UserPassword(user, "***")
		}
	}
	return u.String()
}

// Mask 导出给上层使用。
func Mask(raw string) string { return maskURL(raw) }

// NewTransport 依据出口设置构建 Transport。
// insecure 始终为 false：证书异常必须暴露给用户，不能被静默忽略。
func NewTransport(s Settings) (*http.Transport, error) {
	t := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	if s.Effective == "" {
		t.Proxy = nil // 显式直连，绝不回落到环境变量
		return t, nil
	}

	u, err := url.Parse(s.Effective)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h":
		remoteDNS := strings.ToLower(u.Scheme) == "socks5h"
		user, pass := "", ""
		if u.User != nil {
			user = u.User.Username()
			pass, _ = u.User.Password()
		}
		addr := u.Host
		if u.Port() == "" {
			addr = net.JoinHostPort(u.Hostname(), "1080")
		}
		t.Proxy = nil
		t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return socks5Dial(ctx, network, address, addr, user, pass, remoteDNS)
		}
	default: // http / https 代理
		t.Proxy = http.ProxyURL(u)
	}
	return t, nil
}
