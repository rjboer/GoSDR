# GoSDR Enhancement Tasks

## Priority 1: Persistent Configuration System

### Configuration File Support
- [ ] Add [persistentConfig](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/monopulse/main.go#98-118) struct to [main.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/monopulse/main.go)
- [ ] Implement [loadOrCreateConfig()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/monopulse/main.go#170-190) - loads or creates config.json
- [ ] Implement [saveConfig()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/monopulse/main.go#191-198) - writes config to JSON
- [ ] Implement [defaultPersistentConfig()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go#100-122) - defaults with web enabled
- [ ] Update [main()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/cmd/monopulse/main.go#18-76) to load config.json on startup
- [ ] Change defaults: `webAddr=":8080"`, `sdrBackend="mock"`
- [ ] Test: First run creates config.json with defaults

### Web Interface Integration
- [ ] Add SDR fields to `telemetry.Config` struct in [hub.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go)
- [ ] Update [settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html) - add SDR backend dropdown
- [ ] Update [settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html) - add SDR URI field
- [ ] Update [settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html) - add Mock phase delta field
- [ ] Update [settings.js](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.js) - handle SDR fields
- [ ] Add show/hide logic for SDR-specific fields (Mock vs Pluto)
- [ ] Modify [handleSetConfig()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go#389-424) to persist to config.json
- [ ] Add "Restart Required" message for SDR backend changes

### Testing & Validation
- [ ] Test: First run without config.json
- [ ] Test: Load existing config.json
- [ ] Test: CLI flags override config.json
- [ ] Test: Web UI changes persist to file
- [ ] Test: SDR backend switching via web
- [ ] Update [program_overview.md](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/program_overview.md) with new defaults

---

## Priority 2: Basic Debug Capabilities

### Structured Logging
- [ ] Create `internal/logging/logger.go` package
- [ ] Implement log levels (DEBUG, INFO, WARN, ERROR)
- [ ] Add `--log-level` CLI flag
- [ ] Replace `log.Printf` with structured logging in key areas
- [ ] Add JSON output option for production

### Diagnostic Endpoints
- [ ] Add `GET /api/diagnostics` - system metrics
- [ ] Add `GET /api/diagnostics/health` - health check
- [ ] Add `GET /api/diagnostics/spectrum` - latest FFT snapshot
- [ ] Add uptime, memory, goroutine count to diagnostics
- [ ] Test diagnostic endpoints

### Quick Debug Wins
- [ ] Add `--verbose` flag for detailed console output
- [ ] Add iteration timing logs (when verbose)
- [ ] Add error context wrapping
- [ ] Add startup banner with config summary

---

## Priority 3: Enhanced Telemetry (Optional)

### Debug Mode
- [ ] Add `DebugInfo` struct to telemetry Sample
- [ ] Capture phase delay, monopulse phase in debug mode
- [ ] Add `--debug-mode` flag
- [ ] Add debug panel to web UI

### Signal Quality Metrics
- [ ] Implement SNR estimation
- [ ] Add tracking confidence calculation
- [ ] Add lock status (searching/tracking/locked)
- [ ] Display quality metrics in web UI

---

## Priority 4: Performance & Profiling (Optional)

### Built-in Profiling
- [ ] Add `--enable-pprof` flag
- [ ] Start pprof server on :6060
- [ ] Document profiling usage
- [ ] Add performance benchmarks

### Optimization
- [ ] Profile hot paths with pprof
- [ ] Optimize identified bottlenecks
- [ ] Add performance regression tests

---

## Future Enhancements (Backlog)

### Advanced Debug Features
- [ ] Trace recording to JSONL
- [ ] Web-based debug dashboard
- [ ] Spectrum waterfall display
- [ ] Event log viewer

### Configuration Enhancements
- [ ] Multiple config profiles (save/load named configs)
- [ ] Hot reload for non-SDR settings
- [ ] Config import/export
- [ ] Config validation UI with warnings

### RDF Features
- [ ] Multi-frequency scanning
- [ ] GPS integration for triangulation
- [ ] Doppler compensation
- [ ] Multi-path rejection
- [ ] Bearing display (compass integration)

---

## Completed ✅

### Core Implementation
- [x] Go port of Python monopulse tracker
- [x] SDR abstraction (Mock + Pluto)
- [x] DSP pipeline (FFT, windowing, monopulse)
- [x] Tracking loop (coarse scan + fine tracking)

### Web Interface
- [x] Real-time telemetry dashboard
- [x] Radar visualization
- [x] Settings page with all parameters
- [x] Server-Sent Events for live updates
- [x] Navigation between pages

### Performance Optimizations
- [x] DSP caching (Hamming windows, FFT instances)
- [x] Parallel processing (worker pool coarse scan)
- [x] SIMD-friendly operations
- [x] Memory pooling and optimization

---

## Current Focus

**Active:** Persistent Configuration System (Priority 1)
**Next:** Basic Debug Capabilities (Priority 2)

**Goal:** Make GoSDR easy to run (no flags needed) with good debugging support.
