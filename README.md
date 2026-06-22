# Cloudflare-IP-Pool

> Cloudflare IP 优选工具 - 云端增强版 (Cloud-Powered IP Optimizer)

基于云端智能IP池的 Cloudflare 优选工具，提供美观的 Web 界面。

## ✨ 功能特性

- **云端IP池**：从云端API获取预筛选的高质量IP池，无需本地维护
- **多结果展示**：支持返回 1 个或 5 个优选结果
- **数据中心过滤**：可按数据中心筛选测试节点
- **实时进度**：扫描过程实时显示进度
- **历史记录**：保存最近 10 条结果，点击 IP 可复制

## 🚀 快速开始

### 方式一：直接运行（需要 Go 环境）

```bash
git clone https://github.com/wt20230521/Cloudflare-IP-Pool.git
cd Cloudflare-IP-Pool
go run main.go
```

浏览器访问显示的地址即可。

### 方式二：编译成独立 EXE

```bash
go build -o Cloudflare-IP-Pool.exe main.go
```

双击 `Cloudflare-IP-Pool.exe` 即可运行。

## 📁 项目结构

```
Cloudflare-IP-Pool/
├── better/
│   ├── better.go      # 核心接口与数据结构
│   └── scan.go        # 扫描引擎（含云端API调用）
├── web/
│   └── index.html     # 前端页面
├── main.go            # 主程序入口
├── go.mod             # Go 模块文件
└── go.sum             # 依赖校验文件（自动生成）
```

## 📄 许可证

MIT License
