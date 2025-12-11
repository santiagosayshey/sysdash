package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"runtime"
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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev
	},
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
}

type CPUStats struct {
	Cores   int       `json:"cores"`
	Percent []float64 `json:"percent"`
}

type MemStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"usedPercent"`
}

type DiskStats struct {
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

func getStats() (*Stats, error) {
	hostname, _ := os.Hostname()

	hostInfo, err := host.Info()
	if err != nil {
		return nil, err
	}

	cpuPercent, err := cpu.Percent(time.Second, true)
	if err != nil {
		return nil, err
	}

	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	diskInfo, err := disk.Usage("/")
	if err != nil {
		return nil, err
	}

	netInfo, err := net.IOCounters(true)
	if err != nil {
		return nil, err
	}

	var netStats []NetStats
	for _, n := range netInfo {
		if n.BytesSent > 0 || n.BytesRecv > 0 {
			netStats = append(netStats, NetStats{
				Name:      n.Name,
				BytesSent: n.BytesSent,
				BytesRecv: n.BytesRecv,
			})
		}
	}

	return &Stats{
		Hostname: hostname,
		Uptime:   hostInfo.Uptime,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPU: CPUStats{
			Cores:   runtime.NumCPU(),
			Percent: cpuPercent,
		},
		Memory: MemStats{
			Total:       memInfo.Total,
			Used:        memInfo.Used,
			Available:   memInfo.Available,
			UsedPercent: memInfo.UsedPercent,
		},
		Disk: DiskStats{
			Total:       diskInfo.Total,
			Used:        diskInfo.Used,
			Free:        diskInfo.Free,
			UsedPercent: diskInfo.UsedPercent,
		},
		Network: netStats,
	}, nil
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats, err := getStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		stats, err := getStats()
		if err != nil {
			log.Printf("Error getting stats: %v", err)
			break
		}

		if err := conn.WriteJSON(stats); err != nil {
			log.Printf("WebSocket write failed: %v", err)
			break
		}

		time.Sleep(1 * time.Second)
	}

	log.Printf("Client disconnected")
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// API routes
	http.HandleFunc("/api/stats", handleStats)
	http.HandleFunc("/api/ws", handleWebSocket)

	// Serve static files from embedded filesystem
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(staticFS)))

	log.Printf("Starting server on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
