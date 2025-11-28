# Web Interface Implementation Plan

A web-based real-time visualization and control interface for the GoSDR monopulse DOA tracker using **pure Go stdlib** (no external frameworks like Gorilla).

## Overview

Create a new command `cmd/webmonopulse/` that runs an HTTP server with:
- **Real-time visualization**: Live angle-vs-time chart using WebSockets
- **Settings interface**: Web form to configure tracker parameters
- **Status display**: Current tracking state, peak levels, etc.
- **Pure Go**: Using only `net/http` and `golang.org/x/net/websocket` (minimal dependency)

---

## Architecture

```
┌─────────────────────────────────────────────┐
│           Browser (Client)                  │
│  ┌──────────────┐  ┌──────────────────────┐ │
│  │  Settings UI │  │  Real-time Chart     │ │
│  │  (HTML Form) │  │  (Chart.js/Canvas)   │ │
│  └──────┬───────┘  └──────────┬───────────┘ │
│         │ HTTP POST           │ WebSocket   │
└─────────┼─────────────────────┼─────────────┘
          │                     │
          ▼                     ▼
┌─────────────────────────────────────────────┐
│         Go HTTP Server (stdlib)             │
│  ┌──────────────┐  ┌──────────────────────┐ │
│  │ Settings API │  │  WebSocket Handler   │ │
│  │  /api/config │  │     /ws/telemetry    │ │
│  └──────┬───────┘  └──────────┬───────────┘ │
│         │                     │             │
│         ▼                     ▼             │
│  ┌─────────────────────────────────────┐   │
│  │      Tracker (app.Tracker)          │   │
│  │  ┌──────────┐  ┌─────────────────┐  │   │
│  │  │   SDR    │  │  DSP Functions  │  │   │
│  │  └──────────┘  └─────────────────┘  │   │
│  └─────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

---

## Project Structure

```
cmd/
├── monopulse/              # Existing CLI tool
│   └── main.go
└── webmonopulse/           # NEW: Web interface
    ├── main.go             # HTTP server entry point
    ├── handlers.go         # HTTP/WebSocket handlers
    ├── telemetry.go        # Telemetry broadcaster
    └── static/             # Embedded static files
        ├── index.html      # Main dashboard
        ├── style.css       # Styling
        └── app.js          # Frontend JavaScript

internal/
├── telemetry/
│   ├── stdout.go           # Existing
│   └── websocket.go        # NEW: WebSocket broadcaster
└── app/
    └── tracker.go          # Modified to support dynamic config
```

---

## Proposed Changes

### Backend Components

#### [NEW] [cmd/webmonopulse/main.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/webmonopulse/main.go)

Main HTTP server entry point:

```go
package main

import (
    "context"
    "embed"
    "flag"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
    addr := flag.String("addr", ":8080", "HTTP server address")
    flag.Parse()
    
    // Create server
    srv := NewServer()
    
    // Setup routes
    http.HandleFunc("/", srv.handleIndex)
    http.HandleFunc("/api/config", srv.handleConfig)
    http.HandleFunc("/api/start", srv.handleStart)
    http.HandleFunc("/api/stop", srv.handleStop)
    http.HandleFunc("/api/status", srv.handleStatus)
    http.Handle("/ws/telemetry", srv.handleWebSocket())
    http.Handle("/static/", http.FileServer(http.FS(staticFiles)))
    
    // Start server with graceful shutdown
    go func() {
        log.Printf("Starting web server on %s", *addr)
        if err := http.ListenAndServe(*addr, nil); err != nil {
            log.Fatal(err)
        }
    }()
    
    // Wait for interrupt
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan
    
    log.Println("Shutting down...")
}
```

---

#### [NEW] [cmd/webmonopulse/handlers.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/webmonopulse/handlers.go)

HTTP and WebSocket handlers:

```go
package main

import (
    "encoding/json"
    "net/http"
    "sync"
    "golang.org/x/net/websocket"
    
    "github.com/rjboer/GoSDR/internal/app"
    "github.com/rjboer/GoSDR/internal/sdr"
)

type Server struct {
    mu          sync.RWMutex
    config      app.Config
    tracker     *app.Tracker
    running     bool
    broadcaster *TelemetryBroadcaster
}

func NewServer() *Server {
    return &Server{
        config: app.Config{
            SampleRate:        2e6,
            RxLO:              2.3e9,
            ToneOffset:        200e3,
            NumSamples:        4096,
            SpacingWavelength: 0.5,
            TrackingLength:    1000,
            PhaseStep:         1.0,
            ScanStep:          2.0,
        },
        broadcaster: NewTelemetryBroadcaster(),
    }
}

// GET /
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "static/index.html")
}

// GET/POST /api/config
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
    if r.Method == "GET" {
        s.mu.RLock()
        defer s.mu.RUnlock()
        json.NewEncoder(w).Encode(s.config)
        return
    }
    
    if r.Method == "POST" {
        s.mu.Lock()
        defer s.mu.Unlock()
        
        if s.running {
            http.Error(w, "Cannot change config while running", http.StatusBadRequest)
            return
        }
        
        var newConfig app.Config
        if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        
        s.config = newConfig
        w.WriteHeader(http.StatusOK)
        return
    }
    
    http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// POST /api/start
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.running {
        http.Error(w, "Already running", http.StatusBadRequest)
        return
    }
    
    // Create tracker with web telemetry
    backend := sdr.NewMock()
    s.tracker = app.NewTracker(backend, s.broadcaster, s.config)
    
    // Start tracking in background
    go func() {
        ctx := context.Background()
        if err := s.tracker.Init(ctx); err != nil {
            log.Printf("Init error: %v", err)
            return
        }
        if err := s.tracker.Run(ctx); err != nil {
            log.Printf("Run error: %v", err)
        }
        s.mu.Lock()
        s.running = false
        s.mu.Unlock()
    }()
    
    s.running = true
    w.WriteHeader(http.StatusOK)
}

// POST /api/stop
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if !s.running {
        http.Error(w, "Not running", http.StatusBadRequest)
        return
    }
    
    // TODO: Implement graceful stop via context cancellation
    s.running = false
    w.WriteHeader(http.StatusOK)
}

// GET /api/status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    status := map[string]interface{}{
        "running": s.running,
    }
    
    if s.tracker != nil {
        status["lastDelay"] = s.tracker.LastDelay()
    }
    
    json.NewEncoder(w).Encode(status)
}

// WebSocket /ws/telemetry
func (s *Server) handleWebSocket() http.Handler {
    return websocket.Handler(func(ws *websocket.Conn) {
        s.broadcaster.Register(ws)
        defer s.broadcaster.Unregister(ws)
        
        // Keep connection alive
        for {
            var msg string
            if err := websocket.Message.Receive(ws, &msg); err != nil {
                break
            }
        }
    })
}
```

---

#### [NEW] [cmd/webmonopulse/telemetry.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/webmonopulse/telemetry.go)

WebSocket broadcaster for real-time telemetry:

```go
package main

import (
    "encoding/json"
    "sync"
    "time"
    
    "golang.org/x/net/websocket"
)

type TelemetryMessage struct {
    Timestamp time.Time `json:"timestamp"`
    Angle     float64   `json:"angle"`
    Peak      float64   `json:"peak,omitempty"`
    Type      string    `json:"type"` // "scan" or "track"
}

type TelemetryBroadcaster struct {
    mu      sync.RWMutex
    clients map[*websocket.Conn]bool
}

func NewTelemetryBroadcaster() *TelemetryBroadcaster {
    return &TelemetryBroadcaster{
        clients: make(map[*websocket.Conn]bool),
    }
}

func (b *TelemetryBroadcaster) Register(ws *websocket.Conn) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.clients[ws] = true
}

func (b *TelemetryBroadcaster) Unregister(ws *websocket.Conn) {
    b.mu.Lock()
    defer b.mu.Unlock()
    delete(b.clients, ws)
    ws.Close()
}

func (b *TelemetryBroadcaster) Report(theta float64, peak float64) {
    msg := TelemetryMessage{
        Timestamp: time.Now(),
        Angle:     theta,
        Peak:      peak,
        Type:      "track",
    }
    
    if peak != 0 {
        msg.Type = "scan"
    }
    
    data, _ := json.Marshal(msg)
    
    b.mu.RLock()
    defer b.mu.RUnlock()
    
    for client := range b.clients {
        websocket.Message.Send(client, string(data))
    }
}
```

---

### Frontend Components

#### [NEW] [cmd/webmonopulse/static/index.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/webmonopulse/static/index.html)

Main dashboard HTML:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GoSDR Monopulse Tracker</title>
    <link rel="stylesheet" href="/static/style.css">
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
</head>
<body>
    <div class="container">
        <header>
            <h1>🎯 GoSDR Monopulse DOA Tracker</h1>
            <div class="status" id="status">
                <span class="status-indicator" id="statusIndicator"></span>
                <span id="statusText">Stopped</span>
            </div>
        </header>

        <div class="main-grid">
            <!-- Real-time Chart -->
            <section class="chart-section">
                <h2>Steering Angle vs Time</h2>
                <canvas id="angleChart"></canvas>
            </section>

            <!-- Settings Panel -->
            <aside class="settings-panel">
                <h2>Configuration</h2>
                <form id="configForm">
                    <div class="form-group">
                        <label>Sample Rate (Hz)</label>
                        <input type="number" name="SampleRate" value="2000000" step="1000">
                    </div>
                    
                    <div class="form-group">
                        <label>RX LO (Hz)</label>
                        <input type="number" name="RxLO" value="2300000000" step="1000000">
                    </div>
                    
                    <div class="form-group">
                        <label>Tone Offset (Hz)</label>
                        <input type="number" name="ToneOffset" value="200000" step="1000">
                    </div>
                    
                    <div class="form-group">
                        <label>Number of Samples</label>
                        <input type="number" name="NumSamples" value="4096" step="256">
                    </div>
                    
                    <div class="form-group">
                        <label>Tracking Length</label>
                        <input type="number" name="TrackingLength" value="1000" step="10">
                    </div>
                    
                    <div class="form-group">
                        <label>Phase Step (deg)</label>
                        <input type="number" name="PhaseStep" value="1" step="0.1">
                    </div>
                    
                    <div class="form-group">
                        <label>Scan Step (deg)</label>
                        <input type="number" name="ScanStep" value="2" step="0.5">
                    </div>
                    
                    <div class="form-group">
                        <label>Spacing (λ)</label>
                        <input type="number" name="SpacingWavelength" value="0.5" step="0.01">
                    </div>
                    
                    <button type="submit" class="btn btn-primary">Update Config</button>
                </form>
                
                <div class="controls">
                    <button id="startBtn" class="btn btn-success">Start Tracking</button>
                    <button id="stopBtn" class="btn btn-danger" disabled>Stop Tracking</button>
                </div>
                
                <div class="stats">
                    <h3>Current Stats</h3>
                    <div class="stat-item">
                        <span>Current Angle:</span>
                        <strong id="currentAngle">--</strong>
                    </div>
                    <div class="stat-item">
                        <span>Peak Level:</span>
                        <strong id="peakLevel">--</strong>
                    </div>
                    <div class="stat-item">
                        <span>Last Delay:</span>
                        <strong id="lastDelay">--</strong>
                    </div>
                </div>
            </aside>
        </div>
    </div>

    <script src="/static/app.js"></script>
</body>
</html>
```

---

#### [NEW] [cmd/webmonopulse/static/style.css](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/webmonopulse/static/style.css)

Modern, clean styling:

```css
:root {
    --primary: #2563eb;
    --success: #10b981;
    --danger: #ef4444;
    --bg: #0f172a;
    --surface: #1e293b;
    --text: #f1f5f9;
    --text-muted: #94a3b8;
    --border: #334155;
}

* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

body {
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    background: var(--bg);
    color: var(--text);
    line-height: 1.6;
}

.container {
    max-width: 1400px;
    margin: 0 auto;
    padding: 2rem;
}

header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 2rem;
    padding-bottom: 1rem;
    border-bottom: 2px solid var(--border);
}

h1 {
    font-size: 2rem;
    font-weight: 700;
}

.status {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 1.1rem;
}

.status-indicator {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--text-muted);
    animation: pulse 2s infinite;
}

.status-indicator.running {
    background: var(--success);
}

@keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
}

.main-grid {
    display: grid;
    grid-template-columns: 1fr 400px;
    gap: 2rem;
}

.chart-section {
    background: var(--surface);
    padding: 1.5rem;
    border-radius: 12px;
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3);
}

.chart-section h2 {
    margin-bottom: 1rem;
    font-size: 1.25rem;
}

.settings-panel {
    background: var(--surface);
    padding: 1.5rem;
    border-radius: 12px;
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3);
}

.settings-panel h2 {
    margin-bottom: 1rem;
    font-size: 1.25rem;
}

.form-group {
    margin-bottom: 1rem;
}

.form-group label {
    display: block;
    margin-bottom: 0.25rem;
    font-size: 0.875rem;
    color: var(--text-muted);
}

.form-group input {
    width: 100%;
    padding: 0.5rem;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font-size: 0.875rem;
}

.form-group input:focus {
    outline: none;
    border-color: var(--primary);
}

.btn {
    width: 100%;
    padding: 0.75rem;
    border: none;
    border-radius: 6px;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
}

.btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
}

.btn-primary {
    background: var(--primary);
    color: white;
}

.btn-primary:hover:not(:disabled) {
    background: #1d4ed8;
}

.btn-success {
    background: var(--success);
    color: white;
}

.btn-success:hover:not(:disabled) {
    background: #059669;
}

.btn-danger {
    background: var(--danger);
    color: white;
}

.btn-danger:hover:not(:disabled) {
    background: #dc2626;
}

.controls {
    margin-top: 1.5rem;
    display: flex;
    gap: 0.5rem;
}

.controls .btn {
    width: auto;
    flex: 1;
}

.stats {
    margin-top: 2rem;
    padding-top: 1.5rem;
    border-top: 1px solid var(--border);
}

.stats h3 {
    font-size: 1rem;
    margin-bottom: 1rem;
}

.stat-item {
    display: flex;
    justify-content: space-between;
    padding: 0.5rem 0;
    font-size: 0.875rem;
}

.stat-item span {
    color: var(--text-muted);
}

.stat-item strong {
    color: var(--text);
    font-family: 'Courier New', monospace;
}

@media (max-width: 1024px) {
    .main-grid {
        grid-template-columns: 1fr;
    }
}
```

---

#### [NEW] [cmd/webmonopulse/static/app.js](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/webmonopulse/static/app.js)

Frontend JavaScript logic:

```javascript
// Chart setup
const ctx = document.getElementById('angleChart').getContext('2d');
const maxDataPoints = 1000;
const angleData = [];
const timeData = [];

const chart = new Chart(ctx, {
    type: 'line',
    data: {
        labels: timeData,
        datasets: [{
            label: 'Steering Angle (°)',
            data: angleData,
            borderColor: '#10b981',
            backgroundColor: 'rgba(16, 185, 129, 0.1)',
            borderWidth: 2,
            tension: 0.4,
            pointRadius: 0,
        }]
    },
    options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        scales: {
            x: {
                display: false,
            },
            y: {
                title: {
                    display: true,
                    text: 'Angle (degrees)',
                    color: '#f1f5f9'
                },
                min: -90,
                max: 90,
                grid: {
                    color: '#334155'
                },
                ticks: {
                    color: '#94a3b8'
                }
            }
        },
        plugins: {
            legend: {
                labels: {
                    color: '#f1f5f9'
                }
            }
        }
    }
});

// WebSocket connection
let ws = null;
let reconnectInterval = null;

function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/telemetry`;
    
    ws = new WebSocket(wsUrl);
    
    ws.onopen = () => {
        console.log('WebSocket connected');
        if (reconnectInterval) {
            clearInterval(reconnectInterval);
            reconnectInterval = null;
        }
    };
    
    ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        updateChart(msg.angle);
        updateStats(msg);
    };
    
    ws.onerror = (error) => {
        console.error('WebSocket error:', error);
    };
    
    ws.onclose = () => {
        console.log('WebSocket disconnected');
        if (!reconnectInterval) {
            reconnectInterval = setInterval(connectWebSocket, 3000);
        }
    };
}

function updateChart(angle) {
    angleData.push(angle);
    timeData.push('');
    
    if (angleData.length > maxDataPoints) {
        angleData.shift();
        timeData.shift();
    }
    
    chart.update();
}

function updateStats(msg) {
    document.getElementById('currentAngle').textContent = msg.angle.toFixed(2) + '°';
    
    if (msg.peak) {
        document.getElementById('peakLevel').textContent = msg.peak.toFixed(2) + ' dBFS';
    }
}

// API calls
async function loadConfig() {
    const response = await fetch('/api/config');
    const config = await response.json();
    
    const form = document.getElementById('configForm');
    for (const [key, value] of Object.entries(config)) {
        const input = form.elements[key];
        if (input) {
            input.value = value;
        }
    }
}

async function saveConfig(config) {
    const response = await fetch('/api/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config)
    });
    
    if (!response.ok) {
        const error = await response.text();
        alert('Error: ' + error);
    }
}

async function startTracking() {
    const response = await fetch('/api/start', { method: 'POST' });
    
    if (response.ok) {
        updateUIState(true);
    } else {
        const error = await response.text();
        alert('Error: ' + error);
    }
}

async function stopTracking() {
    const response = await fetch('/api/stop', { method: 'POST' });
    
    if (response.ok) {
        updateUIState(false);
    }
}

async function updateStatus() {
    const response = await fetch('/api/status');
    const status = await response.json();
    
    updateUIState(status.running);
    
    if (status.lastDelay !== undefined) {
        document.getElementById('lastDelay').textContent = status.lastDelay.toFixed(2) + '°';
    }
}

function updateUIState(running) {
    const indicator = document.getElementById('statusIndicator');
    const statusText = document.getElementById('statusText');
    const startBtn = document.getElementById('startBtn');
    const stopBtn = document.getElementById('stopBtn');
    const configForm = document.getElementById('configForm');
    
    if (running) {
        indicator.classList.add('running');
        statusText.textContent = 'Running';
        startBtn.disabled = true;
        stopBtn.disabled = false;
        configForm.querySelectorAll('input, button[type="submit"]').forEach(el => el.disabled = true);
    } else {
        indicator.classList.remove('running');
        statusText.textContent = 'Stopped';
        startBtn.disabled = false;
        stopBtn.disabled = true;
        configForm.querySelectorAll('input, button[type="submit"]').forEach(el => el.disabled = false);
    }
}

// Event listeners
document.getElementById('configForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    
    const formData = new FormData(e.target);
    const config = {};
    
    for (const [key, value] of formData.entries()) {
        config[key] = parseFloat(value) || value;
    }
    
    await saveConfig(config);
    alert('Configuration updated!');
});

document.getElementById('startBtn').addEventListener('click', startTracking);
document.getElementById('stopBtn').addEventListener('click', stopTracking);

// Initialize
connectWebSocket();
loadConfig();
setInterval(updateStatus, 1000);
```

---

## Implementation Workflow

### Phase 1: Backend Setup (30 min)

1. Create directory structure:
   ```bash
   mkdir -p cmd/webmonopulse/static
   ```

2. Implement Go files:
   - [main.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/main.go) - HTTP server
   - `handlers.go` - API endpoints
   - `telemetry.go` - WebSocket broadcaster

3. Add minimal dependency:
   ```bash
   go get golang.org/x/net/websocket
   ```

### Phase 2: Frontend (45 min)

4. Create HTML dashboard (`index.html`)
5. Add CSS styling (`style.css`)
6. Implement JavaScript logic (`app.js`)

### Phase 3: Integration (15 min)

7. Wire tracker with web telemetry
8. Test with MockSDR
9. Verify real-time updates

---

## API Specification

### REST Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serve dashboard HTML |
| GET | `/api/config` | Get current configuration |
| POST | `/api/config` | Update configuration (only when stopped) |
| POST | `/api/start` | Start tracking |
| POST | `/api/stop` | Stop tracking |
| GET | `/api/status` | Get current status |

### WebSocket Protocol

**Endpoint**: `/ws/telemetry`

**Message Format**:
```json
{
  "timestamp": "2025-11-28T22:00:00Z",
  "angle": 12.34,
  "peak": -23.45,
  "type": "track"
}
```

---

## Success Criteria

- [ ] Web server starts and serves dashboard
- [ ] Settings form loads current configuration
- [ ] Settings can be updated (when stopped)
- [ ] Start/Stop buttons control tracker
- [ ] Real-time chart updates via WebSocket
- [ ] Chart displays last 1000 data points
- [ ] Responsive design works on mobile
- [ ] No external dependencies except `golang.org/x/net/websocket`

---

## Future Enhancements

1. **Authentication**: Add basic auth for production use
2. **Multiple backends**: Select Mock vs Pluto from UI
3. **Scan visualization**: Show full coarse scan results
4. **Spectrum plot**: Add FFT spectrum display
5. **Recording**: Save/replay IQ data
6. **Export**: Download tracking data as CSV

---

## Estimated Time

- **Backend**: 30 minutes
- **Frontend**: 45 minutes
- **Integration & Testing**: 15 minutes
- **Total**: ~90 minutes

This provides a production-ready web interface with real-time visualization using only Go stdlib + minimal WebSocket dependency!
