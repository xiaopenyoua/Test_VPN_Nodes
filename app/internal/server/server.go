// Package server 提供本地 HTTP API 与静态资源服务。
// 安全基线：仅绑定 127.0.0.1、校验 Host 防 DNS rebinding、所有 API 需启动令牌。
package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"proxylens/internal/config"
	"proxylens/internal/engine"
)

// Server 封装路由与令牌。
type Server struct {
	eng   *engine.Engine
	token string
	web   fs.FS
	mux   *http.ServeMux
}

// New 构建服务。
func New(eng *engine.Engine, webFS fs.FS, token string) *Server {
	s := &Server{eng: eng, token: token, web: webFS, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler 返回根路由。
func (s *Server) Handler() http.Handler { return s.mux }

// Token 返回本次启动令牌（用于终端打印的访问链接）。
func (s *Server) Token() string { return s.token }

func (s *Server) routes() {
	s.mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]interface{}{"ok": true, "version": engine.Version})
	})
	s.mux.HandleFunc("/api/state", s.guard(s.handleState))
	s.mux.HandleFunc("/api/run", s.guard(s.handleRun))
	s.mux.HandleFunc("/api/cancel", s.guard(s.handleCancel))
	s.mux.HandleFunc("/api/events", s.guard(s.handleEvents))
	s.mux.HandleFunc("/api/report", s.guard(s.handleReport))
	s.mux.HandleFunc("/", s.handleStatic)
}

// guard 做 Host 白名单 + 令牌校验，并统一拒绝跨站写入。
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host) {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		// 跨站请求防护：拒绝带 Origin 且 Origin 与 Host 不一致的请求。
		// 必须按完整的 host:port 精确比对——用 strings.Contains 会把
		// "http://127.0.0.1:18791.evil.com" 判成同源（它确实"包含"本机 host），
		// 攻击者只要注册一个以本机地址为前缀的域名就能绕过。
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
				http.Error(w, "cross-origin blocked", http.StatusForbidden)
				return
			}
		}
		token := r.Header.Get("X-ProxyLens-Token")
		if token == "" {
			token = r.URL.Query().Get("t")
		}
		if token == "" {
			// EventSource 无法设置自定义请求头，用同源 Strict Cookie 兜底
			if c, err := r.Cookie("pl_sess"); err == nil {
				token = c.Value
			}
		}
		if token != s.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func hostAllowed(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	switch h {
	case "127.0.0.1", "localhost", "[::1]", "::1":
		return true
	}
	return false
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if !onlyMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, 200, s.eng.Snapshot())
}

// onlyMethod 统一只读接口的方法白名单。缺了它，OPTIONS / PUT 之类也会被
// 当成正常 GET 处理并返回 200，既不符合语义，也放大了预检请求的暴露面。
func onlyMethod(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want || (want == http.MethodGet && r.Method == http.MethodHead) {
		return true
	}
	w.Header().Set("Allow", want)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var cfg config.Config
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&cfg); err != nil {
		writeJSON(w, 400, map[string]string{"error": "请求参数解析失败"})
		return
	}
	id, err := s.eng.Start(cfg)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"runId": id})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, 200, map[string]bool{"cancelled": s.eng.Cancel()})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !onlyMethod(w, r, http.MethodGet) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)

	ch, cancel := s.eng.Subscribe()
	defer cancel()

	// 首帧补一个快照，避免刷新后丢状态
	writeSSE(w, "snapshot", s.eng.Snapshot())
	flusher.Flush()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, ev.Type, ev.Payload)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if !onlyMethod(w, r, http.MethodGet) {
		return
	}
	st := s.eng.Snapshot()
	switch r.URL.Query().Get("format") {
	case "md":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="proxylens-report.md"`)
		fmt.Fprint(w, engine.MarkdownReport(st))
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="proxylens-report.json"`)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(st)
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		// 首次访问若没带令牌，补上令牌后重定向；同时种下同源 Cookie 供 SSE 使用
		if r.URL.Query().Get("t") == "" {
			http.SetCookie(w, &http.Cookie{
				Name: "pl_sess", Value: s.token, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/?t="+s.token, http.StatusFound)
			return
		}
	}
	// 静态资源不需要令牌（只读、无副作用），但 Host 仍受限
	if !hostAllowed(r.Host) {
		http.Error(w, "invalid host", http.StatusForbidden)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := s.web.Open(p); err != nil {
		http.NotFound(w, r)
		return
	} else {
		f.Close()
	}
	w.Header().Set("Cache-Control", "no-store")
	http.FileServerFS(s.web).ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeSSE(w http.ResponseWriter, event string, payload interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}
