# Multi-Target Tracking Implementation

## Phase 1: Architecture & Data Structures ⏱️ 3-4 hours ✅

### Core Data Structures
- [x] Create [Track](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/app/tracker.go#36-50) struct (ID, angle, peak, SNR, confidence, lock state, history, last update)
- [x] Create `TrackManager` to maintain multiple tracks
- [x] Add track lifecycle states (Tentative, Confirmed, Lost)
- [x] Implement track ID generation (unique per track)

### Configuration
- [x] Add `--tracking-mode` flag (single/multi)
- [x] Add `--max-tracks` setting (default: 5)
- [x] Add `--track-timeout` setting (seconds before track is dropped)
- [x] Add `--min-snr-threshold` for track initiation
- [x] Add configuration to `config.json`

### Telemetry Updates
- [ ] Extend [Sample](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go#354-363) struct to support multiple tracks
- [ ] Create `MultiTrackSample` with array of track data
- [x] Update [Reporter](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/stdout.go#6-9) interface to handle multi-track data (added ReportMultiTrack)
- [x] Maintain backward compatibility for single-track mode

---

## Phase 2: Peak Detection & Association ⏱️ 4-5 hours ✅

the goal of the multi-target tracking is to detect multiple targets in the same scan and track them. This is to be optional, and selectable in the settings screen.


### Multi-Peak Detection
- [x] Implement new function `FindMultiplePeaks()` in [dsp/monopulse.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/dsp/monopulse.go)
- [x] Add peak prominence calculation (avoid noise peaks)
- [x] Add minimum separation constraint (avoid duplicate detections)
- [x] Return sorted list of peaks (by SNR or peak level)
- [x] Add unit tests for peak detection

### Track Association
- [x] Implement nearest-neighbor association (angle-based)
- [x] Add gating threshold (max angle change per update)
- [x] Handle new track initialization
- [x] Handle track updates
- [x] Handle missed detections (track coasting)

### Track Management
- [x] Implement track confirmation logic (N out of M detections)
- [x] Implement track deletion (timeout after missed detections)
- [x] Add track history management (limited buffer per track)
- [x] Implement track quality scoring

---

## Phase 3: DSP Modifications ⏱️ 3-4 hours ✅

### Coarse Scan Updates
- [x] Modify [CoarseScanParallel()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/dsp/monopulse.go#376-476) to return multiple peaks
- [x] Add `PeakInfo` struct (angle, peak, SNR, bin, phase)
- [x] Implement peak sorting and filtering
- [x] Update return signature with peak array

### Monopulse Tracking Updates  
- [x] Modify [MonopulseTrackParallel()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/dsp/monopulse.go#479-553) to accept target delay
- [x] Support parallel tracking of multiple delays (via TrackTarget array)
- [x] Optimize for multiple simultaneous monopulse calculations (shared FFT)
- [x] Add per-track phase delay state

### Performance Optimization
- [ ] Profile multi-target performance
- [x] Optimize FFT reuse across tracks (ShiftedFFT method)
- [ ] Consider worker pool for parallel track updates
- [ ] Benchmark vs single-target mode

---

## Phase 4: Tracker Logic Updates ⏱️ 4-5 hours ✅

### Single-Target Mode (Option 1)
- [x] Keep existing behavior as default
- [x] Select highest SNR/peak from multi-peak detection
- [x] Maintain single `lastDelay` tracking
- [x] Ensure no performance regression

### Multi-Target Mode (Option 2)
- [x] Implement `TrackManager.Update()` with new detections
- [x] Run monopulse tracking for each confirmed track
- [x] Update track states based on measurements
- [x] Prune lost/timed-out tracks
- [x] Limit to `max-tracks` simultaneous tracks

### Mode Switching
- [x] Runtime mode selection via config
- [x] Clean transition between modes (applyTrackingMode)
- [x] Proper state reset when switching modes

---

## Phase 5: Web UI Enhancements ⏱️ 5-6 hours ✅

### Radar Display Updates
- [x] Support multiple target markers on radar
- [x] Color-code tracks by state (Tentative/Confirmed/Lost)
- [x] Add track IDs to display
- [x] Show track history trails (optional)
- [x] Add track selection/highlighting

### Track List Panel
- [x] Create new "Tracks" panel in Telemetry tab
- [x] Display table of active tracks (ID, Angle, SNR, Confidence, State, Age)
- [x] Add sort/filter capabilities
- [x] Show track count summary
- [ ] Add track selection for detail view

### Charts & Statistics
- [x] Support multi-track angle chart (multiple lines)
- [x] Add track-specific statistics
- [x] Color-code chart lines by track ID
- [x] Add legend for track identification
- [x] Update statistics panel for selected track

### Settings UI
- [x] Add tracking mode selector (Single/Multi) - in summary
- [x] Add max tracks slider - in summary
- [x] Add track timeout setting - in summary
- [x] Add SNR threshold setting - in summary
- [ ] Show current tracking mode in settings page (not just summary)

---

## Phase 6: API & Backend ⏱️ 2-3 hours

### Telemetry API Updates
- [ ] Update `/api/live` SSE to send multi-track data
- [ ] Update `/api/history` to support multi-track
- [ ] Add `/api/tracks` endpoint for current track list
- [ ] Add `/api/tracks/{id}` for individual track details
- [ ] Update JSON schema documentation

### Hub Updates
- [ ] Modify [Hub](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go#448-471) to store multi-track samples
- [ ] Update history buffer for multi-track data
- [ ] Implement track-specific history queries
- [ ] Add track filtering in API responses

---

## Phase 7: Testing & Validation ⏱️ 3-4 hours

### Unit Tests
- [ ] Test multi-peak detection with synthetic data
- [ ] Test track association logic
- [ ] Test track lifecycle (init, update, timeout)
- [ ] Test mode switching
- [ ] Test edge cases (max tracks, rapid changes)

### Integration Tests
- [ ] Test with mock SDR generating multiple tones
- [ ] Verify single-target mode unchanged
- [ ] Verify multi-target mode with 2-5 targets
- [ ] Test track persistence across updates
- [ ] Test UI updates with multiple tracks

### Performance Tests
- [ ] Benchmark single vs multi-target overhead
- [ ] Verify real-time performance with max tracks
- [ ] Check memory usage with long-running tracks
- [ ] Profile CPU usage in multi-target mode

### Documentation
- [ ] Update [program_overview.md](file:///c:/Users/Roelof%20Jan/.gemini/antigravity/brain/b0d07863-2ded-42f4-ac71-738e69539f93/program_overview.md) with multi-target architecture
- [ ] Document tracking modes and configuration
- [ ] Add multi-target usage examples
- [ ] Create troubleshooting guide

---

## Phase 8: Advanced Features (Optional) ⏱️ 4-6 hours

### Track Filtering & Smoothing
- [ ] Implement Kalman filter for track smoothing
- [ ] Add track velocity estimation (angle rate)
- [ ] Implement track prediction during missed detections
- [ ] Add configurable smoothing parameters

### Track Visualization Enhancements
- [ ] Add track history trails on radar
- [ ] Implement track age visualization
- [ ] Add track confidence coloring
- [ ] Show predicted track positions

### Advanced Association
- [ ] Implement Global Nearest Neighbor (GNN) association
- [ ] Add multi-hypothesis tracking (MHT) option
- [ ] Handle track merging/splitting
- [ ] Add track quality metrics

---

## Implementation Priority

**Phase 1-2**: Foundation (7-9 hours) - Data structures and peak detection
**Phase 3-4**: Core Logic (7-9 hours) - DSP and tracker updates  
**Phase 5**: UI (5-6 hours) - Visualization and controls
**Phase 6**: API (2-3 hours) - Backend integration
**Phase 7**: Testing (3-4 hours) - Validation and performance
**Phase 8**: Optional (4-6 hours) - Advanced features

**Total Estimated Time**: 24-31 hours (core), +4-6 hours (optional)

---

## Current Status

**Mode**: Planning
**Next**: Phase 1 - Architecture & Data Structures
**Target**: Multi-target tracking with single/multi mode selection
