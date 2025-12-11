package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

//go:embed static/*
var staticFiles embed.FS

// Config holds all configuration options
type Config struct {
	Port           string
	DiskPath       string
	UpdateInterval time.Duration
	Hostname       string
}

var config Config
var cachedStats *Stats
var statsMutex sync.RWMutex

// CPU tracking
var prevCPUTimes []cpu.TimesStat
var cpuPercents []float64
var cpuMutex sync.RWMutex

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev
	},
}

func loadConfig() {
	config.Port = getEnv("PORT", "8080")
	config.DiskPath = getEnv("DISK_PATH", "/")
	config.Hostname = getEnv("HOSTNAME", "")

	intervalMs, _ := strconv.Atoi(getEnv("UPDATE_INTERVAL_MS", "500"))
	if intervalMs < 100 {
		intervalMs = 100
	}
	config.UpdateInterval = time.Duration(intervalMs) * time.Millisecond
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// updateCPUPercents calculates CPU percentages from time deltas
func updateCPUPercents() {
	times, err := cpu.Times(true)
	if err != nil {
		return
	}

	cpuMutex.Lock()
	defer cpuMutex.Unlock()

	if prevCPUTimes == nil {
		prevCPUTimes = times
		cpuPercents = make([]float64, len(times))
		return
	}

	for i, t := range times {
		if i >= len(prevCPUTimes) {
			break
		}
		prev := prevCPUTimes[i]
		totalDelta := (t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal) -
			(prev.User + prev.System + prev.Idle + prev.Nice + prev.Iowait + prev.Irq + prev.Softirq + prev.Steal)

		if totalDelta > 0 {
			idleDelta := t.Idle - prev.Idle
			cpuPercents[i] = 100 * (1 - idleDelta/totalDelta)
		}
	}
	prevCPUTimes = times
}

func getCPUPercents() []float64 {
	cpuMutex.RLock()
	defer cpuMutex.RUnlock()
	result := make([]float64, len(cpuPercents))
	copy(result, cpuPercents)
	return result
}

func collectStats() {
	// Initial CPU sample
	updateCPUPercents()

	for {
		updateCPUPercents()

		stats, err := getStatsNonBlocking()
		if err != nil {
			log.Printf("Error collecting stats: %v", err)
		} else {
			statsMutex.Lock()
			cachedStats = stats
			statsMutex.Unlock()
		}
		time.Sleep(config.UpdateInterval)
	}
}

func getCachedStats() *Stats {
	statsMutex.RLock()
	defer statsMutex.RUnlock()
	return cachedStats
}

type Stats struct {
	Hostname string     `json:"hostname"`
	Uptime   uint64     `json:"uptime"`
	OS       string     `json:"os"`
	Arch     string     `json:"arch"`
	CPU      CPUStats   `json:"cpu"`
	Memory   MemStats   `json:"memory"`
	Disk     DiskStats  `json:"disk"`
	Network  []NetStats `json:"network"`
	GPU      *GPUStats  `json:"gpu,omitempty"`
}

type CPUStats struct {
	Model   string    `json:"model"`
	Cores   int       `json:"cores"`
	Threads int       `json:"threads"`
	Percent []float64 `json:"percent"`
}

type MemStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"usedPercent"`
}

type DiskStats struct {
	Path        string  `json:"path"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"usedPercent"`
}

type NetStats struct {
	Name      string `json:"name"`
	BytesSent uint64 `json:"bytesSent"`
	BytesRecv uint64 `json:"bytesRecv"`
}

type GPUStats struct {
	Name        string  `json:"name"`
	MemoryTotal uint64  `json:"memoryTotal"`
	MemoryUsed  uint64  `json:"memoryUsed"`
	UsedPercent float64 `json:"usedPercent"`
	Temperature float64 `json:"temperature"`
}

// getGPUStats tries AMD (rocm-smi) then NVIDIA (nvidia-smi)
func getGPUStats() *GPUStats {
	// Try AMD first (Linux)
	if runtime.GOOS == "linux" {
		if gpu := getAMDGPU(); gpu != nil {
			return gpu
		}
	}
	// Try NVIDIA (cross-platform)
	if gpu := getNVIDIAGPU(); gpu != nil {
		return gpu
	}
	// Try Windows WMI for any GPU
	if runtime.GOOS == "windows" {
		if gpu := getWindowsGPU(); gpu != nil {
			return gpu
		}
	}
	return nil
}

func getAMDGPU() *GPUStats {
	// Try rocm-smi first
	cmd := exec.Command("rocm-smi", "--showmeminfo", "vram", "--showtemp", "--showuse", "--showproductname")
	output, err := cmd.Output()
	if err == nil {
		return parseRocmSMI(string(output))
	}

	// Fallback: try reading from sysfs (works without rocm-smi)
	return getAMDFromSysfs()
}

func parseRocmSMI(output string) *GPUStats {
	gpu := &GPUStats{}
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.Contains(line, "Card series:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				gpu.Name = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "GPU use (%)") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(strings.TrimSuffix(parts[1], "%"))
				gpu.UsedPercent, _ = strconv.ParseFloat(val, 64)
			}
		}
		if strings.Contains(line, "Temperature") && strings.Contains(line, "edge") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(strings.TrimSuffix(parts[1], "c"))
				gpu.Temperature, _ = strconv.ParseFloat(val, 64)
			}
		}
		if strings.Contains(line, "VRAM Total Memory") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				gpu.MemoryTotal = parseMemoryValue(val)
			}
		}
		if strings.Contains(line, "VRAM Total Used Memory") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				gpu.MemoryUsed = parseMemoryValue(val)
			}
		}
	}

	if gpu.Name == "" {
		return nil
	}
	return gpu
}

func parseMemoryValue(s string) uint64 {
	s = strings.ToLower(strings.TrimSpace(s))
	multiplier := uint64(1)

	if strings.HasSuffix(s, "gb") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "gb")
	} else if strings.HasSuffix(s, "mb") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "mb")
	} else if strings.HasSuffix(s, "kb") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "kb")
	}

	val, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return uint64(val * float64(multiplier))
}

func getAMDFromSysfs() *GPUStats {
	// Find AMD GPU in /sys/class/drm
	dirs, err := os.ReadDir("/sys/class/drm")
	if err != nil {
		return nil
	}

	for _, d := range dirs {
		if !strings.HasPrefix(d.Name(), "card") || strings.Contains(d.Name(), "-") {
			continue
		}

		basePath := "/sys/class/drm/" + d.Name() + "/device"

		// Check if it's an AMD GPU
		vendor, err := os.ReadFile(basePath + "/vendor")
		if err != nil || !strings.Contains(string(vendor), "0x1002") {
			continue
		}

		gpu := &GPUStats{}

		// Get GPU name from product info
		if name, err := os.ReadFile(basePath + "/product_name"); err == nil {
			gpu.Name = strings.TrimSpace(string(name))
		} else {
			gpu.Name = "AMD GPU"
		}

		// Get VRAM info
		if total, err := os.ReadFile(basePath + "/mem_info_vram_total"); err == nil {
			gpu.MemoryTotal, _ = strconv.ParseUint(strings.TrimSpace(string(total)), 10, 64)
		}
		if used, err := os.ReadFile(basePath + "/mem_info_vram_used"); err == nil {
			gpu.MemoryUsed, _ = strconv.ParseUint(strings.TrimSpace(string(used)), 10, 64)
		}

		// Get temperature from hwmon
		hwmonDirs, _ := os.ReadDir(basePath + "/hwmon")
		for _, hw := range hwmonDirs {
			tempFile := basePath + "/hwmon/" + hw.Name() + "/temp1_input"
			if temp, err := os.ReadFile(tempFile); err == nil {
				tempVal, _ := strconv.ParseFloat(strings.TrimSpace(string(temp)), 64)
				gpu.Temperature = tempVal / 1000 // Convert from millidegrees
				break
			}
		}

		// Get GPU usage from gpu_busy_percent
		if busy, err := os.ReadFile(basePath + "/gpu_busy_percent"); err == nil {
			gpu.UsedPercent, _ = strconv.ParseFloat(strings.TrimSpace(string(busy)), 64)
		}

		if gpu.Name != "" {
			return gpu
		}
	}
	return nil
}

func getNVIDIAGPU() *GPUStats {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total,memory.used,utilization.gpu,temperature.gpu", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	line := strings.TrimSpace(string(output))
	parts := strings.Split(line, ", ")
	if len(parts) < 5 {
		return nil
	}

	memTotal, _ := strconv.ParseUint(parts[1], 10, 64)
	memUsed, _ := strconv.ParseUint(parts[2], 10, 64)
	usage, _ := strconv.ParseFloat(parts[3], 64)
	temp, _ := strconv.ParseFloat(parts[4], 64)

	return &GPUStats{
		Name:        parts[0],
		MemoryTotal: memTotal * 1024 * 1024, // Convert MiB to bytes
		MemoryUsed:  memUsed * 1024 * 1024,
		UsedPercent: usage,
		Temperature: temp,
	}
}

func getWindowsGPU() *GPUStats {
	// Use PowerShell to query GPU info via WMI - get all GPUs, pick the one with most VRAM (discrete)
	cmd := exec.Command("powershell", "-Command",
		"Get-CimInstance Win32_VideoController | Sort-Object -Property AdapterRAM -Descending | Select-Object -First 1 -Property Name,AdapterRAM | ForEach-Object { $_.Name + '|' + $_.AdapterRAM }")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	line := strings.TrimSpace(string(output))
	parts := strings.Split(line, "|")
	if len(parts) < 2 || parts[0] == "" {
		return nil
	}

	memTotal, _ := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)

	return &GPUStats{
		Name:        strings.TrimSpace(parts[0]),
		MemoryTotal: memTotal,
		MemoryUsed:  0, // WMI doesn't provide current usage
		UsedPercent: 0, // WMI doesn't provide utilization
		Temperature: 0, // WMI doesn't provide temperature
	}
}

// Static info cached at startup
var staticHostname string
var staticCPUModel string
var staticCPUCores int
var staticUptime uint64

func initStaticInfo() {
	staticHostname = config.Hostname
	if staticHostname == "" {
		staticHostname, _ = os.Hostname()
	}

	cpuInfo, _ := cpu.Info()
	if len(cpuInfo) > 0 {
		staticCPUModel = cpuInfo[0].ModelName
	}
	// Get physical core count
	physicalCores, err := cpu.Counts(false)
	if err == nil && physicalCores > 0 {
		staticCPUCores = physicalCores
	} else if len(cpuInfo) > 0 {
		staticCPUCores = int(cpuInfo[0].Cores)
	}

	hostInfo, _ := host.Info()
	staticUptime = hostInfo.Uptime
}

func getStatsNonBlocking() (*Stats, error) {
	var wg sync.WaitGroup
	var memInfo *mem.VirtualMemoryStat
	var diskInfo *disk.UsageStat
	var netStats []NetStats
	var gpuStats *GPUStats
	var uptime uint64

	wg.Add(5)

	go func() {
		defer wg.Done()
		memInfo, _ = mem.VirtualMemory()
	}()

	go func() {
		defer wg.Done()
		diskInfo, _ = disk.Usage(config.DiskPath)
	}()

	go func() {
		defer wg.Done()
		netInfo, _ := net.IOCounters(true)
		for _, n := range netInfo {
			if n.BytesSent > 0 || n.BytesRecv > 0 {
				netStats = append(netStats, NetStats{
					Name:      n.Name,
					BytesSent: n.BytesSent,
					BytesRecv: n.BytesRecv,
				})
			}
		}
	}()

	go func() {
		defer wg.Done()
		gpuStats = getGPUStats()
	}()

	go func() {
		defer wg.Done()
		hostInfo, _ := host.Info()
		if hostInfo != nil {
			uptime = hostInfo.Uptime
		}
	}()

	wg.Wait()

	if memInfo == nil || diskInfo == nil {
		return nil, fmt.Errorf("failed to get stats")
	}

	return &Stats{
		Hostname: staticHostname,
		Uptime:   uptime,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPU: CPUStats{
			Model:   staticCPUModel,
			Cores:   staticCPUCores,
			Threads: runtime.NumCPU(),
			Percent: getCPUPercents(),
		},
		Memory: MemStats{
			Total:       memInfo.Total,
			Used:        memInfo.Used,
			Available:   memInfo.Available,
			UsedPercent: memInfo.UsedPercent,
		},
		Disk: DiskStats{
			Path:        config.DiskPath,
			Total:       diskInfo.Total,
			Used:        diskInfo.Used,
			Free:        diskInfo.Free,
			UsedPercent: diskInfo.UsedPercent,
		},
		Network: netStats,
		GPU:     gpuStats,
	}, nil
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats := getCachedStats()
	if stats == nil {
		http.Error(w, "Stats not yet available", http.StatusServiceUnavailable)
		return
	}

	json.NewEncoder(w).Encode(stats)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("Client connected")

	for {
		stats := getCachedStats()
		if stats == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if err := conn.WriteJSON(stats); err != nil {
			log.Printf("WebSocket write failed: %v", err)
			break
		}

		time.Sleep(config.UpdateInterval)
	}

	log.Printf("Client disconnected")
}

func main() {
	loadConfig()
	initStaticInfo()

	log.Printf("Config: port=%s disk=%s interval=%s", config.Port, config.DiskPath, config.UpdateInterval)

	// Start background stats collector
	go collectStats()

	// API routes
	http.HandleFunc("/api/stats", handleStats)
	http.HandleFunc("/api/ws", handleWebSocket)

	// Serve static files from embedded filesystem
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(staticFS)))

	log.Printf("Starting server on :%s", config.Port)
	log.Fatal(http.ListenAndServe(":"+config.Port, nil))
}
