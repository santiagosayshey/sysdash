# sysdash

Lightweight system resource monitor. Single binary deployment, designed for embedding in dashboards like Homarr.

## Development

Run backend and frontend separately:

```bash
# Terminal 1 - Backend (port 8080)
make dev-backend

# Terminal 2 - Frontend (port 5173, proxies /api to backend)
make dev-frontend
```

Open http://localhost:5173

## Build

```bash
# Install dependencies
make deps

# Build single binary for current platform
make build

# Build for specific platforms
make build-linux-amd64
make build-linux-arm64
make build-all
```

## Deploy

Copy the binary to your target machine and run:

```bash
./sysdash
```

## Configuration

Configure via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `DISK_PATH` | `/` | Disk path to monitor |
| `UPDATE_INTERVAL` | `1` | Seconds between WebSocket updates |
| `HOSTNAME` | (auto) | Override displayed hostname |

Example:

```bash
PORT=3000 DISK_PATH=/mnt/media HOSTNAME="Plex Server" ./sysdash
```

## API

`GET /api/stats` returns:

```json
{
  "hostname": "server1",
  "uptime": 123456,
  "os": "linux",
  "arch": "amd64",
  "cpu": { "cores": 4, "percent": [12.5, 8.3, 15.2, 5.1] },
  "memory": { "total": 8589934592, "used": 4294967296, "available": 4294967296, "usedPercent": 50.0 },
  "disk": { "total": 107374182400, "used": 53687091200, "free": 53687091200, "usedPercent": 50.0 },
  "network": [{ "name": "eth0", "bytesSent": 123456789, "bytesRecv": 987654321 }]
}
```
