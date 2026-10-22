//go:build darwin

package netx

import (
	"fmt"
	"os/exec"
	"strings"
)

// DetectSystem 通过 scutil --proxy 读取系统代理设置。
func DetectSystem() (proxyURL string, detail string) {
	out, err := exec.Command("scutil", "--proxy").Output()
	if err != nil {
		return "", ""
	}
	kv := parseScutil(string(out))

	enabled := func(k string) bool { return kv[k] == "1" }

	if enabled("HTTPSEnable") && kv["HTTPSProxy"] != "" {
		port := kv["HTTPSPort"]
		if port == "" {
			port = "443"
		}
		return fmt.Sprintf("http://%s:%s", kv["HTTPSProxy"], port),
			fmt.Sprintf("HTTPS 代理 %s:%s", kv["HTTPSProxy"], port)
	}
	if enabled("HTTPEnable") && kv["HTTPProxy"] != "" {
		port := kv["HTTPPort"]
		if port == "" {
			port = "8080"
		}
		return fmt.Sprintf("http://%s:%s", kv["HTTPProxy"], port),
			fmt.Sprintf("HTTP 代理 %s:%s", kv["HTTPProxy"], port)
	}
	if enabled("SOCKSEnable") && kv["SOCKSProxy"] != "" {
		port := kv["SOCKSPort"]
		if port == "" {
			port = "1080"
		}
		return fmt.Sprintf("socks5://%s:%s", kv["SOCKSProxy"], port),
			fmt.Sprintf("SOCKS 代理 %s:%s", kv["SOCKSProxy"], port)
	}
	if enabled("ProxyAutoConfigEnable") && kv["ProxyAutoConfigURLString"] != "" {
		return "", "系统开启了 PAC 自动配置（" + kv["ProxyAutoConfigURLString"] + "），本工具不解析 PAC，按系统路由直出"
	}
	return "", ""
}

func parseScutil(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, " : ", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if k == "" || v == "" {
			continue
		}
		m[k] = v
	}
	return m
}
