// ProxyLens —— 系统代理能力探测工具。
// 通过真实访问 / 下载 AI 站点资源，测量速度、稳定性与 AI 可用性。
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
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
		// emitReady 给桌面外壳（Electron）用：起来后打一行机器可读的 JSON 到 stdout，
		// 外壳据此拿到真实端口与 token。不这么做就只能去正则匹配人类可读日志，
		// 一次改文案就把启动流程弄断。
		emitReady = flag.Bool("emit-ready", false, "启动就绪后向 stdout 输出一行 JSON（供桌面外壳解析）")
		// devDir 只服务于改样式的迭代：把前端资源改为从磁盘读取，刷新即生效。
		// 不传这个参数时走的仍然是内嵌资源，与发布产物完全一致。
		devDir = flag.String("dev", "", "开发模式：从指定目录读取前端资源，改了不用重新编译")
	)
	flag.Parse()

	// 只允许绑定回环。这个工具能在用户机器上对外发起真实请求并消耗带宽，
	// 绑到 0.0.0.0 等于把它变成局域网里的开放探测跳板。
	if *addr != "127.0.0.1" && *addr != "localhost" {
		fmt.Fprintln(os.Stderr, "安全限制：仅允许绑定 127.0.0.1，避免把本地探测能力暴露到局域网")
		os.Exit(2)
	}

	webFS, err := fs.Sub(webAssets, "web")
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取内嵌资源失败:", err)
		os.Exit(1)
	}
	// 开发模式下换用磁盘目录；os.DirFS 返回的已经是 web/ 的根，不能再套一层 Sub
	if *devDir != "" {
		webFS = os.DirFS(*devDir)
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

	if *emitReady {
		// 单行 JSON，外壳按行读取即可；不要换行美化，否则读取端要多写一层拼包逻辑。
		line, _ := json.Marshal(map[string]any{
			"url":     url,
			"port":    actual,
			"token":   srv.Token(),
			"version": engine.Version,
		})
		fmt.Println("PROXYLENS_READY " + string(line))
		// 机器模式下不打花哨 banner：外壳的 stdout 只应出现可解析的行
	} else {
		fmt.Printf("\n  ProxyLens %s  ·  %s/%s\n", engine.Version, runtime.GOOS, runtime.GOARCH)
		fmt.Printf("  ➜  %s\n", url)
		fmt.Println("  按 Ctrl+C 退出")
		fmt.Println()
	}

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
