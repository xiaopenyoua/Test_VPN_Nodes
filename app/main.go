// ProxyLens —— 系统代理能力探测工具。
// 通过真实访问 / 下载 AI 站点资源，测量速度、稳定性与 AI 可用性。
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"proxylens/internal/engine"
	"proxylens/internal/server"
)

//go:embed all:web
var webAssets embed.FS

func main() {
	var (
		port   = flag.Int("port", 0, "监听端口，0 表示自动分配")
		noOpen = flag.Bool("no-open", false, "不自动打开浏览器")
		addr   = flag.String("addr", "127.0.0.1", "监听地址，出于安全考虑仅建议 127.0.0.1")
	)
	flag.Parse()

	if *addr != "127.0.0.1" && *addr != "localhost" {
		fmt.Fprintln(os.Stderr, "安全限制：仅允许绑定 127.0.0.1，避免把本地探测能力暴露到局域网")
		os.Exit(2)
	}

	webFS, err := fs.Sub(webAssets, "web")
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取内嵌资源失败:", err)
		os.Exit(1)
	}

	eng := engine.New()
	srv := server.New(eng, webFS, randomToken())

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *addr, *port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "端口监听失败:", err)
		os.Exit(1)
	}
	actual := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", actual, srv.Token())

	fmt.Printf("\n  ProxyLens %s  ·  %s/%s\n", engine.Version, runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  ➜  %s\n", url)
	fmt.Println("  按 Ctrl+C 退出")
	fmt.Println()

	if !*noOpen {
		go func() {
			time.Sleep(200 * time.Millisecond)
			openBrowser(url)
		}()
	}

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "服务异常退出:", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	fmt.Println("\n  正在退出…")
	_ = httpSrv.Close()
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
