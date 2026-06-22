package better

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ----------------------- API 服务端 -----------------------

var apiServerURL = "https://cfip.989920.xyz" // Worker 地址

// 免费 API 源列表
var freeAPIs = []string{
	"https://bestcf.pages.dev/lajiao/all.txt",
	"https://zip.cm.edu.kg/all.txt",
	"https://raw.githubusercontent.com/qwer-search/bestip/refs/heads/main/addressesapi.txt",
	"https://raw.githubusercontent.com/xgonce/Cloudflare_IP/refs/heads/main/result.csv",
	"https://raw.githubusercontent.com/cmliu/WorkerVless2sub/main/addressescsv.csv",
	"https://raw.githubusercontent.com/xiagefei/CFBestIP/refs/heads/main/Mobile.txt",
	"https://raw.githubusercontent.com/HandsomeMJZ/cfip/refs/heads/main/best_ips.txt",
}

// PoolEntry 池中单条记录
type PoolEntry struct {
	IP   string
	Port int
	TLS  bool
	DC   string // 三字码头
}

// ----------------------- 包级全局变量 -----------------------

var (
	dataDir         string
	randomMu        sync.Mutex
	randomGenerator = rand.New(rand.NewSource(time.Now().UnixNano()))
	progress        string
	progressMu      sync.Mutex
	cancelCtx       context.Context
	cancelCancel    context.CancelFunc
	cancelMu        sync.Mutex
)

func scanCtx() context.Context {
	cancelMu.Lock()
	defer cancelMu.Unlock()
	if cancelCtx != nil {
		return cancelCtx
	}
	return context.Background()
}

// ----------------------- 数据获取 -----------------------

var downloadClient = &http.Client{Timeout: 30 * time.Second}

// parseIPLine 解析各种格式的 IP 行
func parseIPLine(line string) *PoolEntry {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}

	// 跳过 CSV 表头
	lower := strings.ToLower(line)
	if strings.Contains(lower, "ip") && strings.Contains(lower, "端口") {
		return nil
	}
	if strings.Contains(lower, "address") && strings.Contains(lower, "port") {
		return nil
	}

	var ip string
	var port int = 443
	var dc string = ""
	var useTLS bool = true

	// 处理 CSV 格式: IP,端口,延迟,数据中心,...
	if strings.Contains(line, ",") {
		parts := strings.Split(line, ",")
		ip = strings.TrimSpace(parts[0])

		// 尝试提取端口
		if len(parts) > 1 {
			if p, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && p > 0 && p < 65536 {
				port = p
			}
		}

		// 尝试提取数据中心
		if len(parts) > 3 {
			dc = strings.TrimSpace(parts[3])
		}

		// 检查 IP 是否包含端口
		if strings.Contains(ip, ":") && !strings.HasPrefix(ip, "[") {
			ipParts := strings.Split(ip, ":")
			if len(ipParts) == 2 {
				if p, err := strconv.Atoi(ipParts[1]); err == nil {
					port = p
					ip = ipParts[0]
				}
			}
		}
	} else if strings.Contains(line, ":") {
		// IP:端口#备注 格式
		ipPortPart := line
		if idx := strings.Index(line, "#"); idx > 0 {
			ipPortPart = line[:idx]
			// 从备注提取数据中心
			remark := line[idx+1:]
			if idx2 := strings.Index(remark, "|"); idx2 > 0 {
				locPart := remark[idx2+1:]
				locPart = strings.TrimSpace(locPart)
				// 提取三字码头
				if len(locPart) >= 3 {
					dc = extractDCFromLocation(locPart)
				}
			}
		}

		if strings.Contains(ipPortPart, ":") {
			parts := strings.Split(ipPortPart, ":")
			ip = parts[0]
			if p, err := strconv.Atoi(parts[1]); err == nil && p > 0 && p < 65536 {
				port = p
			}
		} else {
			ip = ipPortPart
		}
	} else {
		// 纯 IP
		ip = line
	}

	// 验证 IP 格式
	if ip == "" || net.ParseIP(ip) == nil {
		return nil
	}

	// 根据端口判断 TLS
	if port == 443 || port == 8443 || port == 2053 || port == 2083 || port == 2087 || port == 2096 {
		useTLS = true
	} else {
		useTLS = false
	}

	return &PoolEntry{
		IP:   ip,
		Port: port,
		TLS:  useTLS,
		DC:   dc,
	}
}

// extractDCFromLocation 从位置描述中提取数据中心代码
func extractDCFromLocation(loc string) string {
	loc = strings.ToUpper(loc)

	// 常见数据中心映射
	dcMap := map[string]string{
		"香港": "HKG", "HK": "HKG", "HONG KONG": "HKG",
		"新加坡": "SIN", "SG": "SIN", "SINGAPORE": "SIN",
		"日本": "NRT", "JP": "NRT", "JAPAN": "NRT", "TOKYO": "NRT",
		"韩国": "ICN", "KR": "ICN", "KOREA": "ICN", "SEOUL": "ICN",
		"台湾": "TPE", "TW": "TPE", "TAIWAN": "TPE", "TAIPEI": "TPE",
		"美国": "LAX", "US": "LAX", "USA": "LAX", "LOS ANGELES": "LAX",
		"德国": "FRA", "DE": "FRA", "GERMANY": "FRA", "FRANKFURT": "FRA",
		"英国": "LHR", "UK": "LHR", "BRITAIN": "LHR", "LONDON": "LHR",
		"荷兰": "AMS", "NL": "AMS", "NETHERLANDS": "AMS", "AMSTERDAM": "AMS",
		"法国": "CDG", "FR": "CDG", "FRANCE": "CDG", "PARIS": "CDG",
		"澳大利亚": "SYD", "AU": "SYD", "AUSTRALIA": "SYD", "SYDNEY": "SYD",
		"加拿大": "YYZ", "CA": "YYZ", "CANADA": "YYZ", "TORONTO": "YYZ",
		"印度": "BOM", "IN": "BOM", "INDIA": "BOM", "MUMBAI": "BOM",
		"巴西": "GRU", "BR": "GRU", "BRAZIL": "GRU", "SAO PAULO": "GRU",
	}

	for key, dc := range dcMap {
		if strings.Contains(loc, key) {
			return dc
		}
	}

	return ""
}

// fetchFromFreeAPI 从单个免费 API 获取 IP
func fetchFromFreeAPI(apiURL string) ([]PoolEntry, error) {
	req, _ := http.NewRequestWithContext(scanCtx(), "GET", apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var entries []PoolEntry
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if isCancelled() {
			break
		}
		line := scanner.Text()
		if entry := parseIPLine(line); entry != nil {
			entries = append(entries, *entry)
		}
	}

	return entries, scanner.Err()
}

// fetchPoolFromServer 从免费 API 获取 IP 池（多源聚合）
func fetchPoolFromServer(v4, useTLS bool, dc string) ([]PoolEntry, error) {
	setProgress("正在从免费 API 获取 IP 池...")

	var allEntries []PoolEntry
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 并发从多个 API 获取
	for _, apiURL := range freeAPIs {
		if isCancelled() {
			break
		}
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			entries, err := fetchFromFreeAPI(url)
			if err != nil {
				return
			}

			mu.Lock()
			allEntries = append(allEntries, entries...)
			mu.Unlock()
		}(apiURL)
	}

	wg.Wait()

	if len(allEntries) == 0 {
		return nil, fmt.Errorf("所有免费 API 均无法获取数据")
	}

	setProgress(fmt.Sprintf("IP 池加载完成: 聚合 %d 个节点", len(allEntries)))

	// 去重
	seen := make(map[string]bool)
	var unique []PoolEntry
	for _, e := range allEntries {
		key := fmt.Sprintf("%s:%d", e.IP, e.Port)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, e)
		}
	}

	setProgress(fmt.Sprintf("去重后: %d 个节点", len(unique)))
	return unique, nil
}

// fetchDCs 从 API 获取数据中心列表（使用内置列表）
func fetchDCs() ([]DCEntry, error) {
	// 使用内置的常用数据中心列表
	dcs := []DCEntry{
		{"HKG", "HKG · 香港"},
		{"SIN", "SIN · 新加坡"},
		{"NRT", "NRT · 日本"},
		{"ICN", "ICN · 韩国"},
		{"TPE", "TPE · 台湾"},
		{"LAX", "LAX · 美国"},
		{"FRA", "FRA · 德国"},
		{"LHR", "LHR · 英国"},
		{"AMS", "AMS · 荷兰"},
		{"CDG", "CDG · 法国"},
		{"SYD", "SYD · 澳大利亚"},
		{"YYZ", "YYZ · 加拿大"},
		{"BOM", "BOM · 印度"},
		{"GRU", "GRU · 巴西"},
		{"AKL", "AKL · 新西兰"},
		{"BKK", "BKK · 泰国"},
		{"CGK", "CGK · 印尼"},
		{"DEL", "DEL · 印度"},
		{"DXB", "DXB · 阿联酋"},
		{"EVN", "EVN · 亚美尼亚"},
		{"IST", "IST · 土耳其"},
		{"JNB", "JNB · 南非"},
		{"KUL", "KUL · 马来西亚"},
		{"MNL", "MNL · 菲律宾"},
		{"SEA", "SEA · 美国西雅图"},
		{"SJC", "SJC · 美国圣何塞"},
		{"TLV", "TLV · 以色列"},
		{"VIE", "VIE · 奥地利"},
		{"WAW", "WAW · 波兰"},
		{"ZRH", "ZRH · 瑞士"},
	}
	return dcs, nil
}

// ----------------------- 工具函数 -----------------------

func timeNow() time.Time {
	return time.Now()
}

func timeSince(t time.Time) time.Duration {
	return time.Since(t)
}

func nextRandomIntn(n int) int {
	randomMu.Lock()
	defer randomMu.Unlock()
	return randomGenerator.Intn(n)
}

// randomSample 从列表中随机抽取 n 个元素
func randomSample[T any](list []T, n int) []T {
	shuffled := make([]T, len(list))
	copy(shuffled, list)
	randomMu.Lock()
	randomGenerator.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	randomMu.Unlock()
	if n > len(shuffled) {
		n = len(shuffled)
	}
	return shuffled[:n]
}

// ----------------------- RTT 测试 -----------------------

type RTTResult struct {
	IP        string
	Port      int
	TLS       bool
	DC        string
	LatencyMs int
}

// testRTT 测试单个 IP:port 的 RTT
func testRTT(entry PoolEntry) int {
	var totalMs int
	for i := 0; i < 3; i++ {
		start := time.Now()
		d := net.Dialer{Timeout: 1 * time.Second}
		conn, err := d.DialContext(scanCtx(), "tcp", net.JoinHostPort(entry.IP, strconv.Itoa(entry.Port)))
		if err != nil {
			return 0
		}
		tcpDuration := time.Since(start)

		conn.SetDeadline(start.Add(1 * time.Second))

		var rwc net.Conn = conn
		if entry.TLS {
			tlsConn := tls.Client(conn, &tls.Config{ServerName: "cloudflare.com", InsecureSkipVerify: true})
			if err := tlsConn.Handshake(); err != nil {
				conn.Close()
				return 0
			}
			rwc = tlsConn
		}

		reqStr := "GET / HTTP/1.1\r\nHost: cloudflare.com\r\nUser-Agent: Mozilla/5.0\r\nConnection: close\r\n\r\n"
		_, err = rwc.Write([]byte(reqStr))
		if err != nil {
			rwc.Close()
			return 0
		}

		reader := bufio.NewReader(rwc)
		resp, err := http.ReadResponse(reader, nil)
		rwc.Close()
		if err != nil {
			return 0
		}
		resp.Body.Close()

		if resp.Header.Get("CF-RAY") == "" {
			return 0
		}

		totalMs += int(tcpDuration.Milliseconds())
	}

	return totalMs / 3
}

// runRTTTest 运行 RTT 测试（并发）
func runRTTTest(entries []PoolEntry, taskNum int) []RTTResult {
	if len(entries) < taskNum {
		taskNum = len(entries)
	}

	var wg sync.WaitGroup
	resultChan := make(chan RTTResult, len(entries))
	thread := make(chan struct{}, taskNum)
	var count int
	var mu sync.Mutex
	total := len(entries)

	for _, e := range entries {
		if isCancelled() {
			break
		}
		wg.Add(1)
		thread <- struct{}{}
		go func(entry PoolEntry) {
			defer func() {
				<-thread
				wg.Done()
				mu.Lock()
				count++
				current := count
				mu.Unlock()
				if current%10 == 0 || current == total {
					setProgress(fmt.Sprintf("RTT 测试进度: %d/%d", current, total))
				}
			}()

			if isCancelled() {
				return
			}
			avgMs := testRTT(entry)
			if avgMs > 0 {
				resultChan <- RTTResult{
					IP:        entry.IP,
					Port:      entry.Port,
					TLS:       entry.TLS,
					DC:        entry.DC,
					LatencyMs: avgMs,
				}
			}
		}(e)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var results []RTTResult
	for r := range resultChan {
		results = append(results, r)
	}

	if isCancelled() {
		return nil
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].LatencyMs < results[j].LatencyMs
	})

	if len(results) > 10 {
		setProgress(fmt.Sprintf("RTT 测试完成，%d/%d 个 IP 有效，保留延迟最低的 10 个", len(results), total))
		results = results[:10]
	} else {
		setProgress(fmt.Sprintf("RTT 测试完成，%d/%d 个 IP 有效", len(results), total))
	}
	return results
}

// ----------------------- 速度测试 -----------------------

// runSpeedTestSimple 简单速度测试
func runSpeedTestSimple(ip string, port int, useTLS bool) (int, int, string) {
	var tcpMs int
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			start := time.Now()
			conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp",
				net.JoinHostPort(ip, strconv.Itoa(port)))
			if err == nil {
				tcpMs = int(time.Since(start).Milliseconds())
			}
			return conn, err
		},
	}
	if useTLS {
		transport.TLSClientConfig = &tls.Config{
			ServerName:         "speed.cloudflare.com",
			InsecureSkipVerify: true,
		}
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	testURL := fmt.Sprintf("%s://speed.cloudflare.com/__down?bytes=52428800", scheme)

	req, _ := http.NewRequestWithContext(scanCtx(), "GET", testURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, ""
	}
	defer resp.Body.Close()

	cfRay := resp.Header.Get("CF-RAY")
	dataCenter := extractDataCenter(cfRay)

	buf := make([]byte, 32*1024)
	var totalBytes int64
	var windowBytes int64
	windowStart := time.Now()
	maxSpeed := 0
	for {
		n, err := resp.Body.Read(buf)
		totalBytes += int64(n)
		windowBytes += int64(n)
		if err != nil {
			break
		}

		elapsed := time.Since(windowStart).Seconds()
		if elapsed >= 1.0 {
			speedKB := int(float64(windowBytes) / 1024 / elapsed)
			if speedKB > maxSpeed {
				maxSpeed = speedKB
			}
			windowBytes = 0
			windowStart = time.Now()
		}
	}

	return maxSpeed, tcpMs, dataCenter
}

// extractDataCenter 从 CF-RAY 头提取三字码头
func extractDataCenter(cfRay string) string {
	if cfRay == "" {
		return ""
	}
	parts := strings.Split(cfRay, "-")
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

// verifyTLS 快速 TLS 握手验证
func verifyTLS(ip string, port int) bool {
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 2 * time.Second},
		"tcp",
		net.JoinHostPort(ip, strconv.Itoa(port)),
		&tls.Config{
			ServerName:         "speed.cloudflare.com",
			InsecureSkipVerify: true,
		})
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ----------------------- 核心测试逻辑 -----------------------

// cloudflareTest 使用免费 IP 池做优选，返回 topN 个结果
func cloudflareTest(ipType int, useTLS bool, taskNum int, speed int, topN int) []ScanResult {
	entries, err := fetchPoolFromServer(ipType == 4, useTLS, dcFilter)
	if err != nil {
		setProgress("获取 IP 池失败: " + err.Error())
		return []ScanResult{{Error: "获取 IP 池失败: " + err.Error()}}
	}
	if isCancelled() {
		return []ScanResult{{Error: "扫描已取消"}}
	}

	// 按 IP 版本和 TLS 过滤
	var filtered []PoolEntry
	for _, e := range entries {
		isV4 := strings.Count(e.IP, ":") == 0
		if (ipType == 4 && !isV4) || (ipType == 6 && isV4) {
			continue
		}
		if useTLS && !e.TLS {
			continue
		}
		filtered = append(filtered, e)
	}

	if len(filtered) == 0 {
		return []ScanResult{{Error: "没有符合条件的节点"}}
	}

	setProgress(fmt.Sprintf("从 %d 个节点中筛选出 %d 个", len(entries), len(filtered)))

	// 按数据中心过滤
	if dcFilter != "" {
		var dcFiltered []PoolEntry
		for _, e := range filtered {
			if e.DC == dcFilter {
				dcFiltered = append(dcFiltered, e)
			}
		}
		if len(dcFiltered) == 0 {
			return []ScanResult{{Error: fmt.Sprintf("数据中心 %s 没有符合条件的节点", dcFilter)}}
		}
		filtered = dcFiltered
		setProgress(fmt.Sprintf("数据中心 %s: %d 个节点", dcFilter, len(filtered)))
	}

	sampleSize := 100
	if len(filtered) < sampleSize {
		sampleSize = len(filtered)
	}

	if isCancelled() {
		return []ScanResult{{Error: "扫描已取消"}}
	}

	// RTT 测试
	sampled := randomSample(filtered, sampleSize)
	setProgress(fmt.Sprintf("开始 RTT 测试 %d 个 IP...", len(sampled)))
	rttResults := runRTTTest(sampled, taskNum)
	if isCancelled() {
		return []ScanResult{{Error: "扫描已取消"}}
	}
	if len(rttResults) == 0 {
		return []ScanResult{{Error: "所有 IP RTT 丢包，无可用节点"}}
	}

	// TLS 握手验证
	if useTLS {
		var tlsOK []RTTResult
		for _, r := range rttResults {
			if isCancelled() {
				return []ScanResult{{Error: "扫描已取消"}}
			}
			setProgress(fmt.Sprintf("验证 TLS %s:%d...", r.IP, r.Port))
			if verifyTLS(r.IP, r.Port) {
				tlsOK = append(tlsOK, r)
			}
		}
		if len(tlsOK) == 0 {
			return []ScanResult{{Error: "暂无可用节点，请再次扫描"}}
		}
		setProgress(fmt.Sprintf("TLS 验证通过 %d/%d 个", len(tlsOK), len(rttResults)))
		rttResults = tlsOK
	}

	// 速度测试
	type testResult struct {
		ipport   string
		maxSpeed int
		latency  int
		dc       string
	}
	var results []testResult
	for _, r := range rttResults {
		if isCancelled() {
			return []ScanResult{{Error: "扫描已取消"}}
		}
		setProgress(fmt.Sprintf("正在测速 %s:%d (延迟 %dms)", r.IP, r.Port, r.LatencyMs))
		maxSpeed, tcpMs, dc := runSpeedTestSimple(r.IP, r.Port, r.TLS)
		dcName := dc
		if dc == "" {
			dcName = r.DC
		}
		setProgress(fmt.Sprintf("%s:%d 峰值 %d kB/s, 数据中心 %s", r.IP, r.Port, maxSpeed, dcName))
		results = append(results, testResult{
			ipport:   fmt.Sprintf("%s:%d", r.IP, r.Port),
			maxSpeed: maxSpeed,
			latency:  tcpMs,
			dc:       dcName,
		})
	}

	if len(results) > 0 {
		var ok []testResult
		for _, r := range results {
			if r.maxSpeed > 0 {
				ok = append(ok, r)
			}
		}
		results = ok
	}

	if len(results) == 0 {
		return []ScanResult{{Error: "暂无可用节点，请再次扫描"}}
	}

	// 按速度降序排列
	sort.Slice(results, func(i, j int) bool {
		return results[i].maxSpeed > results[j].maxSpeed
	})

	if topN > len(results) {
		topN = len(results)
	}

	var out []ScanResult
	for i := 0; i < topN; i++ {
		r := results[i]
		out = append(out, ScanResult{
			IP:            r.ipport,
			MaxSpeed:      r.maxSpeed,
			RealBandwidth: r.maxSpeed / 128,
			LatencyMs:     r.latency,
			DataCenter:    r.dc,
		})
	}
	setProgress(fmt.Sprintf("测速完成，返回 %d 个优选 IP", len(out)))
	return out
}
