# Web Interface Redesign - Task List

## Phase 1: Tab Infrastructure ⏱️ 2-3 hours


## Priority 1: Basic Debug Capabilities

### Structured Logging
- [x] Create `internal/logging/logger.go` package
- [x] Implement log levels (DEBUG, INFO, WARN, ERROR)
- [x] Add `--log-level` CLI flag
- [ ] Replace `log.Printf` with structured logging in key areas
- [x] Add JSON output option for production

### Diagnostic Endpoints
- [x] Add `GET /api/diagnostics` - system metrics
- [x] Add `GET /api/diagnostics/health` - health check
- [x] Add `GET /api/diagnostics/spectrum` - latest FFT snapshot
- [x] Add uptime, memory, goroutine count to diagnostics
- [x] Test diagnostic endpoints

### Quick Debug Wins
- [ ] Add `--verbose` flag for detailed console output
- [ ] Add iteration timing logs (when verbose)
- [ ] Add error context wrapping
- [ ] Add startup banner with config summary

---

## Priority 3: Enhanced Telemetry 

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





### Tab Navigation System
- [x] Create tab navigation HTML structure in [index.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/index.html)
- [x] Add tab container divs (telemetry, trace, debug, settings)
- [x] Implement tab switching JavaScript in [app.js](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/app.js)
- [x] Add CSS styling for tabs in [app.css](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/app.css)
- [x] Make tabs responsive (mobile-friendly)
- [x] Test tab switching functionality

---

## Phase 2: Telemetry Tab Improvements ⏱️ 2-3 hours

### Remove Table
- [ ] Remove latest samples table HTML
- [ ] Remove table update JavaScript logic

### Add Settings Display
- [ ] Create current settings summary panel
- [ ] Display SDR backend, frequency, sample rate, FFT size
- [ ] Update display when settings change

### Add Statistics Panel
- [ ] Implement statistics calculation (avg, std dev, min/max)
- [ ] Display angle statistics
- [ ] Display peak level statistics  
- [ ] Show current update rate

### Fix Chart Data Accumulation
- [x] Implement `MAX_CHART_POINTS = 100` limit
- [x] Auto-remove old data with `shift()` when limit exceeded
- [x] Apply to angle chart
- [x] Apply to peak chart
- [x] Test chart scrolling behavior

### Performance Optimizations
- [x] Add SSE update throttling (10 Hz max)
- [x] Use `chart.update('none')` to disable animations
- [x] Implement requestAnimationFrame for rendering
- [x] Add update rate limiter

---

## Phase 3: Raw Trace Tab ⏱️ 2-3 hours

### Build Table View
- [ ] Create trace tab HTML section
- [ ] Implement scrollable table with fixed header
- [ ] Add columns: Timestamp, Angle, Peak
- [ ] Implement virtual scrolling (render only visible rows)
- [ ] Limit to 500 samples max

### Controls
- [ ] Add Pause/Resume button
- [ ] Add Clear History button
- [ ] Add sample count display
- [ ] Implement pause functionality

### Export Features
- [ ] Implement CSV export
- [ ] Implement JSON export
- [ ] Add Copy to Clipboard button
- [ ] Add Download as File button

---

## Phase 4: Debug Tab ⏱️ 3-4 hours

### Backend API Endpoints
- [x] Create `/api/diagnostics` endpoint in [hub.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go)
- [x] Create `/api/diagnostics/health` endpoint
- [x] Implement system metrics collection (uptime, CPU, memory)
- [x] Add goroutine count tracking
- [ ] Implement signal quality estimation (SNR, confidence)

### Frontend Debug UI
- [ ] Create debug tab HTML
- [ ] Add System Status section (status, uptime, samples, update rate)
- [ ] Add Performance section (CPU, memory, goroutines, iteration time)
- [ ] Add Signal Quality section (SNR, confidence, lock status, noise floor)
- [ ] Add Debug Info section (phase delay, monopulse phase, peaks)
- [ ] Add Event Log viewer (last 100 events)
- [ ] Implement auto-refresh (5 second interval)

---

## Phase 5: Settings Tab Enhancements ⏱️ 1-2 hours

### Add SDR Backend Fields
- [x] Add backend dropdown (Mock/Pluto) to [settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html)
- [x] Add SDR URI input field
- [x] Add Mock phase delta field
- [x] Implement show/hide logic for backend-specific fields

### Configuration Persistence
- [x] Update [handleSetConfig](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go#389-424) to save to config.json
- [x] Add "Restart Required" warning for SDR changes
- [ ] Add Export Config button
- [x] Test settings persistence

---

## Phase 6: Testing & Polish ⏱️ 1-2 hours

### Functionality Testing
- [ ] Test all tabs individually
- [ ] Test tab switching
- [ ] Test telemetry updates
- [ ] Test raw trace table
- [ ] Test debug diagnostics
- [ ] Test settings save/load

### Performance Testing
- [ ] Verify Chrome performance (no lag warnings)
- [ ] Check memory usage over time
- [ ] Verify chart data limiting works
- [ ] Test with high update rates

### Cross-Browser Testing
- [ ] Test in Chrome
- [ ] Test in Firefox
- [ ] Test in Edge
- [ ] Test mobile responsiveness

### Documentation
- [ ] Update [program_overview.md](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/program_overview.md)
- [ ] Add screenshots to walkthrough
- [ ] Document new features

---
### RDF Features future (future/optional)
- [ ] Multi-frequency scanning
- [ ] GPS integration for triangulation
- [ ] Doppler compensation
- [ ] Multi-path rejection
- [ ] Bearing display (compass integration)
- [ ] Spectrum waterfall display in a separate tab

## Expected Results

### Performance
- ✅ Reduce SSE message handling from 651ms to <50ms
- ✅ Eliminate Chrome lag warnings
- ✅ Smooth 60 FPS rendering
- ✅ Stable memory usage (no accumulation)

### Features
- ✅ Clean tabbed interface
- ✅ Current settings display
- ✅ Real-time statistics
- ✅ Raw data export
- ✅ Advanced diagnostics
- ✅ Better configuration management

---

## Current Status

**Active:** Phase 2 - Telemetry Tab Improvements
**Next:** Phase 3 - Raw Trace Tab

**Total Estimated Time:** 10-16 hours
