# CF IP 优选 - Windows 版

基于 Go 核心引擎的 Windows 版 Cloudflare IP 优选测速工具。

**架构：** Go + 内嵌 Web UI（单文件可执行程序，无需安装依赖）

## 使用

```bash
# 直接运行（需要 Go 环境）
go run main.go

# 编译成独立 EXE
go build -o cfipgui.exe
```

启动后自动打开浏览器访问 `http://127.0.0.1:随机端口`。

## 功能

- IPv4 / IPv6 协议切换
- TLS 连接验证开关
- 期望带宽设置（Mbps）
- 数据中心过滤（下拉列表，从 API 自动获取）
- 结果数量（1 个 / 5 个）
- 实时进度显示
- 多结果排名展示
- 历史记录（本地存储，点击 IP 复制）
- 深色主题 Web UI

## 数据源

后端 API 服务：`https://cfip.989920.xyz`

## 构建

```bash
# Windows 64位
go build -o cfipgui.exe

# 或者交叉编译
GOOS=windows GOARCH=amd64 go build -o cfipgui.exe
```

## 技术栈

- **Go 核心引擎** — 从 API 获取 IP 池 → RTT 测速 → 带宽测试 → 输出最佳结果
- **内嵌 HTTP 服务** — 启动时自动分配可用端口
- **前端** — 纯 HTML/CSS/JS，无外部依赖
