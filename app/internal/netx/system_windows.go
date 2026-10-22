//go:build windows

package netx

import (
	"fmt"
	"os/exec"
	"strings"
)

// DetectSystem 通过注册表读取 WinINET 代理设置（浏览器/大多数桌面程序使用的设置）。
func DetectSystem() (proxyURL string, detail string) {
	const key = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

	enable := regQuery(key, "ProxyEnable")
	server := regQuery(key, "ProxyServer")
	if strings.Contains(enable, "0x1") && server != "" {
		server = strings.TrimSpace(server)
		// 可能是 host:port，也可能是 "http=host:port;https=host:port"
		if strings.Contains(server, "=") {
			for _, part := range strings.Split(server, ";") {
				kvp := strings.SplitN(part, "=", 2)
				if len(kvp) == 2 && (strings.EqualFold(kvp[0], "https") || strings.EqualFold(kvp[0], "http")) {
					u := ensureScheme(strings.TrimSpace(kvp[1]), "http")
					return u, "系统代理 " + u
				}
			}
			return "", "系统代理配置存在但无法解析：" + server
		}
		u := ensureScheme(server, "http")
		return u, "系统代理 " + u
	}

	// WinHTTP 级别的代理（服务/命令行程序使用）
	out, err := exec.Command("netsh", "winhttp", "show", "proxy").Output()
	if err == nil {
		s := string(out)
		if !strings.Contains(s, "Direct access") && !strings.Contains(s, "直接访问") {
			for _, line := range strings.Split(s, "\n") {
				line = strings.TrimSpace(line)
				if idx := strings.IndexAny(line, ":"); idx > 0 && !strings.Contains(line, "Proxy Server") {
					cand := strings.TrimSpace(line[idx+1:])
					if strings.Contains(cand, ":") && !strings.Contains(cand, " ") {
						u := ensureScheme(cand, "http")
						return u, "WinHTTP 代理 " + u
					}
				}
			}
		}
	}
	return "", ""
}

func regQuery(key, value string) string {
	out, err := exec.Command("reg", "query", key, "/v", value).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && strings.EqualFold(fields[0], value) {
			return strings.Join(fields[2:], " ")
		}
	}
	// 兜底：REG_SZ 值可能含空格
	if idx := strings.Index(string(out), value); idx >= 0 {
		rest := strings.TrimSpace(string(out)[idx+len(value):])
		if fields := strings.Fields(rest); len(fields) >= 2 {
			return strings.Join(fields[1:], " ")
		}
	}
	return ""
}

func ensureScheme(addr, scheme string) string {
	if strings.Contains(addr, "://") {
		return addr
	}
	return fmt.Sprintf("%s://%s", scheme, addr)
}
