package main

import (
	"cfip/better"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
)

//go:embed web/*
var webFS embed.FS

// getWebFS 获取 web 子目录的文件系统
func getWebFS() http.FileSystem {
	webSubdir, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	return http.FS(webSubdir)
}

// ── Scan Manager ──

type scanManager struct {
	mu      sync.Mutex
	running bool
	result  string
}

var manager scanManager

// scanRequest 扫描请求参数
type scanRequest struct {
	V4    bool   `json:"v4"`
	TLS   bool   `json:"tls"`
	BW    int    `json:"bw"`
	DC    string `json:"dc"`
	Count int    `json:"count"`
}

type scanResponse struct {
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
	Result  string `json:"result,omitempty"`
}

type progressResponse struct {
	Running  bool   `json:"running"`
	Progress string `json:"progress"`
	Result   string `json:"result,omitempty"`
}

// ── API Handlers ──

func handleDCs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dcs := better.GetDataCenters()
	if dcs == "" {
		dcs = "[]"
	}
	w.Write([]byte(dcs))
}

func handleScan(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	manager.mu.Lock()
	if manager.running {
		manager.mu.Unlock()
		json.NewEncoder(w).Encode(scanResponse{Running: true, Error: "扫描正在进行中"})
		return
	}
	manager.running = true
	manager.result = ""
	manager.mu.Unlock()

	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = scanRequest{V4: true, TLS: true, BW: 50, Count: 1}
	}
	if req.BW <= 0 {
		req.BW = 50
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Count > 5 {
		req.Count = 5
	}

	better.SetDataCenterFilter(req.DC)

	json.NewEncoder(w).Encode(scanResponse{Running: true})

	go func() {
		result := better.GetIPs(req.V4, req.TLS, req.BW, req.Count)
		manager.mu.Lock()
		manager.result = result
		manager.running = false
		manager.mu.Unlock()
	}()
}

func handleProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	manager.mu.Lock()
	running := manager.running
	result := manager.result
	manager.mu.Unlock()

	progress := better.GetProgress()

	json.NewEncoder(w).Encode(progressResponse{
		Running:  running,
		Progress: progress,
		Result:   result,
	})
}

type statusResponse struct {
	Running bool   `json:"running"`
	Result  string `json:"result,omitempty"`
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	manager.mu.Lock()
	json.NewEncoder(w).Encode(statusResponse{
		Running: manager.running,
		Result:  manager.result,
	})
	manager.mu.Unlock()
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	better.CancelScan()
	manager.mu.Lock()
	manager.running = false
	manager.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]bool{"cancelled": true})
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	better.UpdateData()
	json.NewEncoder(w).Encode(map[string]bool{"updated": true})
}

// ── Main ──

func main() {
	log.SetFlags(0)

	better.SetApiServer("https://cfip.989920.xyz")

	// 获取 web 子目录的文件系统
	webFiles := getWebFS()

	// 创建路由
	mux := http.NewServeMux()

	// 根路径直接提供 index.html
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			// 读取并返回 index.html
			data, err := webFS.ReadFile("web/index.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		// 其他路径使用文件服务器
		http.FileServer(webFiles).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/dcs", handleDCs)
	mux.HandleFunc("/api/scan", handleScan)
	mux.HandleFunc("/api/progress", handleProgress)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/cancel", handleCancel)
	mux.HandleFunc("/api/update", handleUpdate)

	// Find free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("无法启动服务: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	log.Printf("  CF IP 优选已启动")
	log.Printf("  打开浏览器访问: http://127.0.0.1:%d", port)

	// Windows: 使用 cmd /c start 打开浏览器
	url := "http://127.0.0.1:" + strconv.Itoa(port)
	exec.Command("cmd", "/c", "start", url).Start()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("正在停止...")
		better.CancelScan()
		os.Exit(0)
	}()

	log.Fatal(http.Serve(listener, mux))
}
