# Persistent Configuration System - Implementation Plan

## Goal

Implement a configuration system that:
1. **Defaults to web interface + MockSDR** (no hardware required)
2. **Persists settings to JSON file** (`config.json`)
3. **Allows SDR backend switching** via web settings page
4. **Auto-creates config file** if it doesn't exist

---

## User Requirements

> **"I would like that as the default option with no hardware connected, And i would like to switch in the options. the options should be stored in a json the application needs to make."**

**Translation:**
- Default: `--sdr-backend=mock --web-addr=:8080`
- Settings changeable via web interface
- Settings saved to `config.json`
- Application creates `config.json` if missing

---

## Proposed Changes

### 1. Configuration File Structure

#### [NEW] `config.json` (auto-created)

```json
{
  "sdr": {
    "backend": "mock",
    "uri": "",
    "mockPhaseDelta": 30.0
  },
  "rf": {
    "sampleRate": 2000000,
    "rxLO": 2300000000,
    "rxGain0": 60,
    "rxGain1": 60,
    "txGain": -10,
    "toneOffset": 200000,
    "spacingWavelength": 0.5
  },
  "dsp": {
    "numSamples": 4096,
    "trackingLength": 1000,
    "phaseStep": 1.0,
    "scanStep": 2.0,
    "phaseCal": 0.0,
    "warmupBuffers": 3
  },
  "telemetry": {
    "historyLimit": 500,
    "webAddr": ":8080"
  }
}
```

**Default Location:** `./config.json` (current directory)

---

### 2. Configuration Loading Priority

```
1. config.json (if exists)
   ↓
2. Environment variables (override)
   ↓
3. Command-line flags (override)
   ↓
4. Hardcoded defaults (fallback)
```

**Example:**
```bash
# Uses config.json
go run main.go

# Override specific values
go run main.go --rx-lo=915000000

# Ignore config.json, use env vars
MONO_SDR_BACKEND=pluto go run main.go
```

---

### 3. Code Changes

#### [MODIFY] [cmd/monopulse/main.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/monopulse/main.go)

**Add configuration file support:**

```go
const defaultConfigPath = "config.json"

type persistentConfig struct {
    SDR struct {
        Backend        string  `json:"backend"`
        URI            string  `json:"uri"`
        MockPhaseDelta float64 `json:"mockPhaseDelta"`
    } `json:"sdr"`
    RF struct {
        SampleRate        float64 `json:"sampleRate"`
        RxLO              float64 `json:"rxLO"`
        RxGain0           int     `json:"rxGain0"`
        RxGain1           int     `json:"rxGain1"`
        TxGain            int     `json:"txGain"`
        ToneOffset        float64 `json:"toneOffset"`
        SpacingWavelength float64 `json:"spacingWavelength"`
    } `json:"rf"`
    DSP struct {
        NumSamples     int     `json:"numSamples"`
        TrackingLength int     `json:"trackingLength"`
        PhaseStep      float64 `json:"phaseStep"`
        ScanStep       float64 `json:"scanStep"`
        PhaseCal       float64 `json:"phaseCal"`
        WarmupBuffers  int     `json:"warmupBuffers"`
    } `json:"dsp"`
    Telemetry struct {
        HistoryLimit int    `json:"historyLimit"`
        WebAddr      string `json:"webAddr"`
    } `json:"telemetry"`
}

// loadOrCreateConfig loads config.json or creates it with defaults
func loadOrCreateConfig(path string) (persistentConfig, error) {
    cfg := defaultPersistentConfig()
    
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            // Create default config file
            log.Printf("Config file not found, creating %s with defaults", path)
            if err := saveConfig(path, cfg); err != nil {
                return cfg, fmt.Errorf("create config: %w", err)
            }
            return cfg, nil
        }
        return cfg, fmt.Errorf("read config: %w", err)
    }
    
    if err := json.Unmarshal(data, &cfg); err != nil {
        return cfg, fmt.Errorf("parse config: %w", err)
    }
    
    log.Printf("Loaded configuration from %s", path)
    return cfg, nil
}

// saveConfig writes configuration to JSON file
func saveConfig(path string, cfg persistentConfig) error {
    data, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(path, data, 0644)
}

// defaultPersistentConfig returns defaults with web interface enabled
func defaultPersistentConfig() persistentConfig {
    var cfg persistentConfig
    
    // SDR defaults (MockSDR)
    cfg.SDR.Backend = "mock"
    cfg.SDR.URI = ""
    cfg.SDR.MockPhaseDelta = 30.0
    
    // RF defaults
    cfg.RF.SampleRate = 2e6
    cfg.RF.RxLO = 2.3e9
    cfg.RF.RxGain0 = 60
    cfg.RF.RxGain1 = 60
    cfg.RF.TxGain = -10
    cfg.RF.ToneOffset = 200e3
    cfg.RF.SpacingWavelength = 0.5
    
    // DSP defaults
    cfg.DSP.NumSamples = 4096
    cfg.DSP.TrackingLength = 1000
    cfg.DSP.PhaseStep = 1.0
    cfg.DSP.ScanStep = 2.0
    cfg.DSP.PhaseCal = 0.0
    cfg.DSP.WarmupBuffers = 3
    
    // Telemetry defaults (WEB ENABLED BY DEFAULT)
    cfg.Telemetry.HistoryLimit = 500
    cfg.Telemetry.WebAddr = ":8080"
    
    return cfg
}
```

**Update main() to use persistent config:**

```go
func main() {
    // Load or create config.json
    persistCfg, err := loadOrCreateConfig(defaultConfigPath)
    if err != nil {
        log.Printf("Warning: %v, using defaults", err)
        persistCfg = defaultPersistentConfig()
    }
    
    // Parse CLI flags (override config.json)
    cfg, err := parseConfig(os.Args[1:], os.LookupEnv, persistCfg)
    if err != nil {
        log.Fatalf("parse config: %v", err)
    }
    
    // ... rest of main()
}
```

---

#### [MODIFY] [internal/telemetry/hub.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go)

**Add SDR backend to Config struct:**

```go
type Config struct {
    // SDR Configuration
    SDRBackend     string  `json:"sdrBackend"`     // NEW
    SDRURI         string  `json:"sdrUri"`         // NEW
    MockPhaseDelta float64 `json:"mockPhaseDelta"` // NEW
    
    // RF Configuration
    SampleRate        float64 `json:"sampleRate"`
    RxLO              float64 `json:"rxLO"`
    // ... existing fields
}
```

**Update handleSetConfig to save to file:**

```go
func (h *Hub) handleSetConfig(w http.ResponseWriter, r *http.Request) {
    var incoming Config
    if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    cfg, err := validateConfig(incoming, h.config)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // Apply to hub
    h.applyConfig(cfg)
    
    // Save to config.json
    if err := saveConfigToFile("config.json", cfg); err != nil {
        log.Printf("Warning: failed to save config: %v", err)
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(cfg)
}
```

---

#### [MODIFY] [internal/telemetry/static/settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html)

**Add SDR backend selection:**

```html
<section class="settings-section">
  <h2>SDR Backend</h2>
  
  <div class="form-field">
    <label for="sdrBackend">Backend Type</label>
    <select id="sdrBackend">
      <option value="mock">Mock (Simulation)</option>
      <option value="pluto">Pluto (Hardware)</option>
    </select>
    <small>Select SDR backend. Mock requires no hardware.</small>
  </div>
  
  <div class="form-field">
    <label for="sdrUri">SDR URI</label>
    <input type="text" id="sdrUri" placeholder="ip:192.168.2.1">
    <small>Connection URI for hardware SDR (e.g., ip:192.168.2.1)</small>
  </div>
  
  <div class="form-field">
    <label for="mockPhaseDelta">Mock Phase Delta (°)</label>
    <input type="number" id="mockPhaseDelta" step="0.1">
    <small>Simulated angle for MockSDR (degrees)</small>
  </div>
</section>
```

---

#### [MODIFY] [internal/telemetry/static/settings.js](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.js)

**Add SDR backend fields:**

```javascript
const fieldIds = [
  'sdrBackend',      // NEW
  'sdrUri',          // NEW
  'mockPhaseDelta',  // NEW
  'sampleRate',
  // ... existing fields
];

const defaults = {
  sdrBackend: 'mock',
  sdrUri: '',
  mockPhaseDelta: 30.0,
  sampleRate: 2000000,
  // ... existing defaults
};

// Show/hide SDR-specific fields based on backend
document.getElementById('sdrBackend').addEventListener('change', (e) => {
  const isMock = e.target.value === 'mock';
  document.getElementById('sdrUri').disabled = isMock;
  document.getElementById('mockPhaseDelta').disabled = !isMock;
});
```

---

### 4. Runtime SDR Switching

> **Note:** Switching SDR backend at runtime requires tracker restart

**Options:**

**Option A: Restart Required (Simple)**
```
User changes SDR backend → Save to config.json → Show message:
"Configuration saved. Please restart the application for changes to take effect."
```

**Option B: Hot Reload (Complex)**
```
User changes SDR backend → Save to config.json → Stop tracker → 
Reinitialize with new backend → Start tracker
```

**Recommendation:** Start with Option A (restart required), implement Option B later if needed.

---

## Implementation Steps

### Phase 1: Configuration File Support
1. Add `persistentConfig` struct to [main.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/main.go)
2. Implement `loadOrCreateConfig()` and `saveConfig()`
3. Update [main()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/monopulse/main.go#17-52) to load `config.json`
4. Change defaults: `webAddr=":8080"`, `sdrBackend="mock"`
5. Test: Run without flags, verify `config.json` created

### Phase 2: Web Interface Integration
6. Add SDR fields to `telemetry.Config`
7. Update [settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html) with SDR backend dropdown
8. Update [settings.js](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.js) to handle SDR fields
9. Modify [handleSetConfig](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go#260-289) to save to `config.json`
10. Test: Change settings via web, verify saved to file

### Phase 3: Validation & Polish
11. Add validation for SDR backend values
12. Add UI feedback for save success/failure
13. Add "Restart Required" message for SDR changes
14. Update documentation
15. Test all scenarios

---

## Testing Plan

### Test Cases

**1. First Run (No config.json)**
```bash
go run main.go
# Expected: Creates config.json with defaults
# Expected: Web interface at http://localhost:8080
# Expected: MockSDR running
```

**2. Load Existing Config**
```bash
# Edit config.json manually
go run main.go
# Expected: Uses values from config.json
```

**3. CLI Override**
```bash
go run main.go --rx-lo=915000000
# Expected: Uses config.json + overrides RX LO
```

**4. Web Settings Change**
```
1. Open http://localhost:8080/settings
2. Change "Sample Rate" to 1000000
3. Click "Save changes"
# Expected: config.json updated
# Expected: Success message shown
```

**5. SDR Backend Switch**
```
1. Open settings
2. Change backend from "Mock" to "Pluto"
3. Enter URI: "ip:192.168.2.1"
4. Save
# Expected: config.json updated
# Expected: Message: "Restart required"
```

---

## Success Criteria

- [ ] `config.json` auto-created on first run
- [ ] Default: web interface enabled, MockSDR selected
- [ ] Settings persist across restarts
- [ ] Web interface can change all settings
- [ ] SDR backend switchable via web UI
- [ ] CLI flags override config file
- [ ] Clear user feedback on save/errors

---

## Future Enhancements

1. **Multiple Config Profiles**
   - Save/load named configurations
   - Quick switch between setups

2. **Hot Reload**
   - Apply some settings without restart
   - Restart tracker automatically for SDR changes

3. **Config Validation UI**
   - Real-time validation feedback
   - Prevent invalid configurations

4. **Import/Export**
   - Export config as JSON
   - Import from file or URL
