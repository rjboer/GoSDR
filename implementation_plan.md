# Settings Page Implementation Plan

## Overview

Add a dedicated settings page to the web telemetry interface for configuring all tracker parameters including frequency, sample rate, timing, gains, and tracking parameters.

## Configurable Parameters

### RF Configuration
- **RX LO Frequency** (Hz) - Receiver local oscillator frequency
- **TX LO Frequency** (Hz) - Transmitter local oscillator frequency  
- **Tone Offset** (Hz) - Offset frequency for the transmitted tone
- **Sample Rate** (Hz) - ADC sample rate
- **RX Gain 0** (dB) - Receiver channel 0 gain
- **RX Gain 1** (dB) - Receiver channel 1 gain
- **TX Gain** (dB) - Transmitter gain

### Timing Parameters
- **Number of Samples** - Samples per RX buffer
- **Tracking Length** - Number of tracking iterations
- **Update Interval** (ms) - Time between tracking updates
- **Warm-up Iterations** - Number of buffers to discard on startup

### Tracking Parameters
- **Antenna Spacing** (wavelengths) - Distance between antennas
- **Phase Step** (degrees) - Monopulse update step size
- **Scan Step** (degrees) - Coarse scan step size
- **Phase Calibration** (degrees) - Phase offset calibration

### SDR Backend
- **Backend Type** - Mock or Pluto
- **SDR URI** - Connection string for hardware SDR
- **Mock Phase Delta** (degrees) - Phase offset for MockSDR

---

## Proposed Changes

### Backend API

#### [MODIFY] [hub.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go)

Add configuration management:

```go
// Config holds all tracker configuration parameters
type Config struct {
    // RF Configuration
    RxLO       float64 `json:"rxLO"`
    TxLO       float64 `json:"txLO"`
    ToneOffset float64 `json:"toneOffset"`
    SampleRate float64 `json:"sampleRate"`
    RxGain0    int     `json:"rxGain0"`
    RxGain1    int     `json:"rxGain1"`
    TxGain     int     `json:"txGain"`
    
    // Timing
    NumSamples       int `json:"numSamples"`
    TrackingLength   int `json:"trackingLength"`
    UpdateInterval   int `json:"updateInterval"` // milliseconds
    WarmupIterations int `json:"warmupIterations"`
    
    // Tracking
    SpacingWavelength float64 `json:"spacingWavelength"`
    PhaseStep         float64 `json:"phaseStep"`
    ScanStep          float64 `json:"scanStep"`
    PhaseCal          float64 `json:"phaseCal"`
    
    // SDR
    SDRBackend   string  `json:"sdrBackend"`
    SDRURI       string  `json:"sdrURI"`
    MockPhaseDelta float64 `json:"mockPhaseDelta"`
}

// Add to Hub
type Hub struct {
    // ... existing fields
    config Config
    configMu sync.RWMutex
}

func (h *Hub) GetConfig() Config {
    h.configMu.RLock()
    defer h.configMu.RUnlock()
    return h.config
}

func (h *Hub) SetConfig(cfg Config) error {
    h.configMu.Lock()
    defer h.configMu.Unlock()
    
    // Validate configuration
    if err := validateConfig(cfg); err != nil {
        return err
    }
    
    h.config = cfg
    return nil
}

func validateConfig(cfg Config) error {
    if cfg.SampleRate <= 0 || cfg.SampleRate > 30.72e6 {
        return fmt.Errorf("sample rate must be between 0 and 30.72 MHz")
    }
    if cfg.NumSamples <= 0 || cfg.NumSamples > 65536 {
        return fmt.Errorf("num samples must be between 1 and 65536")
    }
    // ... more validation
    return nil
}

// Add HTTP handlers
func (h *Hub) handleGetConfig(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(h.GetConfig())
}

func (h *Hub) handleSetConfig(w http.ResponseWriter, r *http.Request) {
    var cfg Config
    if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    if err := h.SetConfig(cfg); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    w.WriteHeader(http.StatusOK)
}
```

#### [MODIFY] [webserver.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/webserver.go)

Add settings endpoints:

```go
func NewWebServer(addr string, hub *Hub) *WebServer {
    mux := http.NewServeMux()
    mux.Handle("/static/", http.FileServer(http.FS(staticFiles)))
    mux.HandleFunc("/api/history", hub.handleHistory)
    mux.HandleFunc("/api/live", hub.handleLive)
    mux.HandleFunc("/api/config", hub.handleGetConfig)      // NEW
    mux.HandleFunc("/api/config/update", hub.handleSetConfig) // NEW
    mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFileFS(w, r, staticFiles, "static/settings.html")
    })
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFileFS(w, r, staticFiles, "static/index.html")
    })
    
    return &WebServer{hub: hub, srv: &http.Server{Addr: addr, Handler: mux}}
}
```

---

### Frontend

#### [NEW] [settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html)

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Settings - GoSDR</title>
  <link rel="stylesheet" href="/static/style.css" />
  <link rel="stylesheet" href="/static/settings.css" />
</head>
<body>
  <header>
    <h1>⚙️ GoSDR Settings</h1>
    <nav>
      <a href="/" class="nav-link">← Back to Dashboard</a>
    </nav>
  </header>
  <main class="settings-main">
    <form id="settingsForm">
      
      <!-- RF Configuration -->
      <section class="settings-section">
        <h2>RF Configuration</h2>
        <div class="settings-grid">
          <div class="form-group">
            <label for="rxLO">RX LO Frequency (Hz)</label>
            <input type="number" id="rxLO" name="rxLO" step="1000000" required>
            <span class="help-text">Receiver local oscillator (e.g., 2.3 GHz)</span>
          </div>
          
          <div class="form-group">
            <label for="toneOffset">Tone Offset (Hz)</label>
            <input type="number" id="toneOffset" name="toneOffset" step="1000" required>
            <span class="help-text">Transmitted tone offset (e.g., 200 kHz)</span>
          </div>
          
          <div class="form-group">
            <label for="sampleRate">Sample Rate (Hz)</label>
            <input type="number" id="sampleRate" name="sampleRate" step="100000" required>
            <span class="help-text">ADC sample rate (max 30.72 MHz)</span>
          </div>
          
          <div class="form-group">
            <label for="rxGain0">RX Gain 0 (dB)</label>
            <input type="number" id="rxGain0" name="rxGain0" min="-10" max="73" required>
          </div>
          
          <div class="form-group">
            <label for="rxGain1">RX Gain 1 (dB)</label>
            <input type="number" id="rxGain1" name="rxGain1" min="-10" max="73" required>
          </div>
          
          <div class="form-group">
            <label for="txGain">TX Gain (dB)</label>
            <input type="number" id="txGain" name="txGain" min="-89" max="0" required>
          </div>
        </div>
      </section>
      
      <!-- Timing Parameters -->
      <section class="settings-section">
        <h2>Timing Parameters</h2>
        <div class="settings-grid">
          <div class="form-group">
            <label for="numSamples">Number of Samples</label>
            <input type="number" id="numSamples" name="numSamples" step="256" required>
            <span class="help-text">Samples per RX buffer (power of 2)</span>
          </div>
          
          <div class="form-group">
            <label for="trackingLength">Tracking Length</label>
            <input type="number" id="trackingLength" name="trackingLength" required>
            <span class="help-text">Number of tracking iterations</span>
          </div>
          
          <div class="form-group">
            <label for="updateInterval">Update Interval (ms)</label>
            <input type="number" id="updateInterval" name="updateInterval" min="1" max="1000" required>
            <span class="help-text">Time between tracking updates</span>
          </div>
          
          <div class="form-group">
            <label for="warmupIterations">Warm-up Iterations</label>
            <input type="number" id="warmupIterations" name="warmupIterations" min="0" max="100" required>
            <span class="help-text">Buffers to discard on startup</span>
          </div>
        </div>
      </section>
      
      <!-- Tracking Parameters -->
      <section class="settings-section">
        <h2>Tracking Parameters</h2>
        <div class="settings-grid">
          <div class="form-group">
            <label for="spacingWavelength">Antenna Spacing (λ)</label>
            <input type="number" id="spacingWavelength" name="spacingWavelength" step="0.01" required>
            <span class="help-text">Fraction of wavelength (typically 0.5)</span>
          </div>
          
          <div class="form-group">
            <label for="phaseStep">Phase Step (degrees)</label>
            <input type="number" id="phaseStep" name="phaseStep" step="0.1" required>
            <span class="help-text">Monopulse update step size</span>
          </div>
          
          <div class="form-group">
            <label for="scanStep">Scan Step (degrees)</label>
            <input type="number" id="scanStep" name="scanStep" step="0.5" required>
            <span class="help-text">Coarse scan step size</span>
          </div>
          
          <div class="form-group">
            <label for="phaseCal">Phase Calibration (degrees)</label>
            <input type="number" id="phaseCal" name="phaseCal" step="0.1" required>
            <span class="help-text">Phase offset calibration</span>
          </div>
        </div>
      </section>
      
      <!-- SDR Backend -->
      <section class="settings-section">
        <h2>SDR Backend</h2>
        <div class="settings-grid">
          <div class="form-group">
            <label for="sdrBackend">Backend Type</label>
            <select id="sdrBackend" name="sdrBackend" required>
              <option value="mock">Mock (Simulation)</option>
              <option value="pluto">Pluto (Hardware)</option>
            </select>
          </div>
          
          <div class="form-group">
            <label for="sdrURI">SDR URI</label>
            <input type="text" id="sdrURI" name="sdrURI" placeholder="ip:192.168.2.1">
            <span class="help-text">Connection string for hardware SDR</span>
          </div>
          
          <div class="form-group">
            <label for="mockPhaseDelta">Mock Phase Delta (degrees)</label>
            <input type="number" id="mockPhaseDelta" name="mockPhaseDelta" step="1">
            <span class="help-text">Phase offset for MockSDR testing</span>
          </div>
        </div>
      </section>
      
      <!-- Actions -->
      <div class="settings-actions">
        <button type="button" id="resetBtn" class="btn btn-secondary">Reset to Defaults</button>
        <button type="submit" class="btn btn-primary">Save Settings</button>
      </div>
      
      <div id="statusMessage" class="status-message"></div>
    </form>
  </main>
  <script src="/static/settings.js"></script>
</body>
</html>
```

#### [NEW] [settings.css](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.css)

```css
.settings-main {
  max-width: 1200px;
  margin: 0 auto;
}

nav {
  display: inline-block;
}

.nav-link {
  color: #2f80ed;
  text-decoration: none;
  font-size: 0.9rem;
}

.nav-link:hover {
  text-decoration: underline;
}

.settings-section {
  background: #121926;
  padding: 1.5rem;
  border-radius: 8px;
  margin-bottom: 1.5rem;
}

.settings-section h2 {
  margin: 0 0 1rem 0;
  color: #cbd5e1;
  font-size: 1.25rem;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 1.5rem;
}

.form-group {
  display: flex;
  flex-direction: column;
}

.form-group label {
  margin-bottom: 0.25rem;
  color: #9fb3c8;
  font-size: 0.875rem;
  font-weight: 600;
}

.form-group input,
.form-group select {
  padding: 0.5rem;
  background: #0c1118;
  border: 1px solid #1f2a3a;
  border-radius: 4px;
  color: #e4e9f0;
  font-size: 0.875rem;
}

.form-group input:focus,
.form-group select:focus {
  outline: none;
  border-color: #2f80ed;
}

.help-text {
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: #64748b;
  font-style: italic;
}

.settings-actions {
  display: flex;
  gap: 1rem;
  justify-content: flex-end;
  margin-top: 2rem;
}

.btn {
  padding: 0.75rem 1.5rem;
  border: none;
  border-radius: 6px;
  font-size: 0.875rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-primary {
  background: #2f80ed;
  color: white;
}

.btn-primary:hover {
  background: #1d6fd8;
}

.btn-secondary {
  background: #475569;
  color: white;
}

.btn-secondary:hover {
  background: #334155;
}

.status-message {
  margin-top: 1rem;
  padding: 0.75rem;
  border-radius: 4px;
  text-align: center;
  font-size: 0.875rem;
  display: none;
}

.status-message.success {
  display: block;
  background: #10b98133;
  color: #10b981;
  border: 1px solid #10b981;
}

.status-message.error {
  display: block;
  background: #ef444433;
  color: #ef4444;
  border: 1px solid #ef4444;
}
```

#### [NEW] [settings.js](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.js)

```javascript
const form = document.getElementById('settingsForm');
const statusMessage = document.getElementById('statusMessage');

// Default configuration
const defaultConfig = {
  rxLO: 2.3e9,
  txLO: 2.3e9,
  toneOffset: 200e3,
  sampleRate: 2e6,
  rxGain0: 40,
  rxGain1: 40,
  txGain: -3,
  numSamples: 4096,
  trackingLength: 1000,
  updateInterval: 10,
  warmupIterations: 20,
  spacingWavelength: 0.5,
  phaseStep: 1.0,
  scanStep: 2.0,
  phaseCal: 0.0,
  sdrBackend: 'mock',
  sdrURI: '',
  mockPhaseDelta: 30.0
};

// Load current configuration
async function loadConfig() {
  try {
    const response = await fetch('/api/config');
    const config = await response.json();
    populateForm(config);
  } catch (err) {
    console.error('Failed to load config:', err);
    populateForm(defaultConfig);
  }
}

// Populate form with configuration
function populateForm(config) {
  for (const [key, value] of Object.entries(config)) {
    const input = form.elements[key];
    if (input) {
      input.value = value;
    }
  }
}

// Save configuration
form.addEventListener('submit', async (e) => {
  e.preventDefault();
  
  const formData = new FormData(form);
  const config = {};
  
  for (const [key, value] of formData.entries()) {
    const input = form.elements[key];
    if (input.type === 'number') {
      config[key] = parseFloat(value) || parseInt(value);
    } else {
      config[key] = value;
    }
  }
  
  try {
    const response = await fetch('/api/config/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config)
    });
    
    if (response.ok) {
      showStatus('Settings saved successfully!', 'success');
    } else {
      const error = await response.text();
      showStatus(`Error: ${error}`, 'error');
    }
  } catch (err) {
    showStatus(`Failed to save: ${err.message}`, 'error');
  }
});

// Reset to defaults
document.getElementById('resetBtn').addEventListener('click', () => {
  if (confirm('Reset all settings to defaults?')) {
    populateForm(defaultConfig);
  }
});

// Show status message
function showStatus(message, type) {
  statusMessage.textContent = message;
  statusMessage.className = `status-message ${type}`;
  
  setTimeout(() => {
    statusMessage.className = 'status-message';
  }, 5000);
}

// Initialize
loadConfig();
```

#### [MODIFY] [index.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/index.html)

Add navigation link:

```html
<header>
  <h1>GoSDR Monopulse Tracker</h1>
  <nav>
    <a href="/settings" class="nav-link">⚙️ Settings</a>
  </nav>
  <p>Live steering angle and peak telemetry</p>
</header>
```

---

## Implementation Steps

1. **Backend** (30 min)
   - Add Config struct to hub.go
   - Add validation function
   - Add GET/POST handlers
   - Update webserver routes

2. **Frontend HTML** (20 min)
   - Create settings.html
   - Add all form fields
   - Add navigation

3. **Frontend CSS** (15 min)
   - Create settings.css
   - Style form layout
   - Add responsive design

4. **Frontend JS** (20 min)
   - Create settings.js
   - Load/save configuration
   - Form validation

5. **Integration** (15 min)
   - Add navigation to index.html
   - Test settings flow
   - Verify persistence

**Total Time**: ~100 minutes

---

## Success Criteria

- [ ] Settings page accessible from dashboard
- [ ] All parameters configurable via web UI
- [ ] Settings persist across page reloads
- [ ] Validation prevents invalid values
- [ ] Reset to defaults works
- [ ] Save confirmation displayed
- [ ] Responsive design on mobile

---

## Future Enhancements

1. **Live restart**: Apply settings without manual tracker restart
2. **Presets**: Save/load configuration presets
3. **Import/Export**: JSON configuration file support
4. **Advanced mode**: Show/hide advanced parameters
5. **Validation feedback**: Real-time field validation
