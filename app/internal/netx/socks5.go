package netx

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// socks5Dial 实现了最小可用的 SOCKS5 客户端（RFC1928 / RFC1929），
// 不引入外部依赖。remoteDNS=true 时由代理解析域名（等价 socks5h），
// 否则本地解析后以 IP 连接（等价 socks5）。
func socks5Dial(ctx context.Context, network, address, proxyAddr, user, pass string, remoteDNS bool) (net.Conn, error) {
	d := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("连接代理 %s 失败: %w", proxyAddr, err)
	}

	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	defer func() {
		_ = conn.SetDeadline(time.Time{})
	}()

	// 1. 方法协商
	methods := []byte{0x00}
	if user != "" {
		methods = append(methods, 0x02)
	}
	buf := make([]byte, 0, 2+len(methods))
	buf = append(buf, 0x05, byte(len(methods)))
	buf = append(buf, methods...)
	if _, err := conn.Write(buf); err != nil {
		conn.Close()
		return nil, err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 协商失败: %w", err)
	}
	if resp[0] != 0x05 {
		conn.Close()
		return nil, errors.New("代理返回了非 SOCKS5 协议")
	}
	switch resp[1] {
	case 0x00:
		// 无需认证
	case 0x02:
		if user == "" {
			conn.Close()
			return nil, errors.New("代理要求用户名密码认证")
		}
		auth := []byte{0x01, byte(len(user))}
		auth = append(auth, user...)
		auth = append(auth, byte(len(pass)))
		auth = append(auth, pass...)
		if _, err := conn.Write(auth); err != nil {
			conn.Close()
			return nil, err
		}
		ar := make([]byte, 2)
		if _, err := io.ReadFull(conn, ar); err != nil {
			conn.Close()
			return nil, fmt.Errorf("SOCKS5 认证失败: %w", err)
		}
		if ar[1] != 0x00 {
			conn.Close()
			return nil, errors.New("代理认证被拒绝（用户名或密码错误）")
		}
	case 0xFF:
		conn.Close()
		return nil, errors.New("代理拒绝了所有认证方式")
	default:
		conn.Close()
		return nil, fmt.Errorf("代理要求不支持的认证方式 0x%02x", resp[1])
	}

	// 2. CONNECT 请求
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("目标地址非法: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("目标端口非法: %w", err)
	}

	req := []byte{0x05, 0x01, 0x00}
	ip := net.ParseIP(host)
	switch {
	case ip != nil && ip.To4() != nil:
		req = append(req, 0x01)
		req = append(req, ip.To4()...)
	case ip != nil:
		req = append(req, 0x04)
		req = append(req, ip.To16()...)
	case remoteDNS:
		if len(host) > 255 {
			conn.Close()
			return nil, errors.New("目标域名过长")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, host...)
	default:
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			conn.Close()
			return nil, fmt.Errorf("本地解析 %s 失败（可改用 socks5h 让代理解析）", host)
		}
		chosen := ips[0].IP
		for _, v := range ips {
			if v.IP.To4() != nil {
				chosen = v.IP
				break
			}
		}
		if v4 := chosen.To4(); v4 != nil {
			req = append(req, 0x01)
			req = append(req, v4...)
		} else {
			req = append(req, 0x04)
			req = append(req, chosen.To16()...)
		}
	}
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}

	// 3. 读取应答
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 应答失败: %w", err)
	}
	if head[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("代理拒绝连接目标: %s", socksReplyText(head[1]))
	}
	var skip int
	switch head[3] {
	case 0x01:
		skip = 4
	case 0x04:
		skip = 16
	case 0x03:
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			conn.Close()
			return nil, err
		}
		skip = int(l[0])
	default:
		conn.Close()
		return nil, errors.New("SOCKS5 应答地址类型无法识别")
	}
	if _, err := io.ReadFull(conn, make([]byte, skip+2)); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func socksReplyText(code byte) string {
	switch code {
	case 0x01:
		return "代理内部故障"
	case 0x02:
		return "规则不允许"
	case 0x03:
		return "网络不可达"
	case 0x04:
		return "主机不可达"
	case 0x05:
		return "连接被拒绝"
	case 0x06:
		return "TTL 过期"
	case 0x07:
		return "不支持的命令"
	case 0x08:
		return "不支持的地址类型"
	default:
		return fmt.Sprintf("未知错误码 0x%02x", code)
	}
}
