# Multi-Target Tracking Implementation

## Phase 1: Architecture & Data Structures ⏱️ 3-4 hours

### Core Data Structures
- [ ] Create [Track](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/app/tracker.go#36-50) struct (ID, angle, peak, SNR, confidence, lock state, history, last update)
- [ ] Create `TrackManager` to maintain multiple tracks
- [ ] Add track lifecycle states (Tentative, Confirmed, Lost)
- [ ] Implement track ID generation (unique per track)

### Configuration
- [ ] Add `--tracking-mode` flag (single/multi)
- [ ] Add `--max-tracks` setting (default: 5)
- [ ] Add `--track-timeout` setting (seconds before track is dropped)
- [ ] Add `--min-snr-threshold` for track initiation
- [ ] Add configuration to `config.json`

### Telemetry Updates
- [ ] Extend [Sample](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go#354-363) struct to support multiple tracks
- [ ] Create `MultiTrackSample` with array of track data
- [ ] Update [Reporter](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/stdout.go#6-9) interface to handle multi-track data
- [ ] Maintain backward compatibility for single-track mode

---

## Phase 2: Peak Detection & Association ⏱️ 4-5 hours

the goal of the multi-target tracking is to detect multiple targets in the same scan and track them. This is to be optional, and selectable in the settings screen.


### Multi-Peak Detection
- [ ] Implement new function `FindMultiplePeaks()` in [dsp/monopulse.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/dsp/monopulse.go)
- [ ] Add peak prominence calculation (avoid noise peaks)
- [ ] Add minimum separation constraint (avoid duplicate detections)
- [ ] Return sorted list of peaks (by SNR or peak level)
- [ ] Add unit tests for peak detection

### Track Association
- [ ] Implement nearest-neighbor association (angle-based)
- [ ] Add gating threshold (max angle change per update)
- [ ] Handle new track initialization
- [ ] Handle track updates
- [ ] Handle missed detections (track coasting)

### Track Management
- [ ] Implement track confirmation logic (N out of M detections)
- [ ] Implement track deletion (timeout after missed detections)
- [ ] Add track history management (limited buffer per track)
- [ ] Implement track quality scoring

---

## Phase 3: DSP Modifications ⏱️ 3-4 hours

### Coarse Scan Updates
- [ ] Modify [CoarseScanParallel()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/dsp/monopulse.go#376-476) to return multiple peaks
- [ ] Add `PeakInfo` struct (angle, peak, SNR, bin, phase)
- [ ] Implement peak sorting and filtering
- [ ] Update return signature with peak array

### Monopulse Tracking Updates  
- [ ] Modify [MonopulseTrackParallel()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/dsp/monopulse.go#479-553) to accept target delay
- [ ] Support parallel tracking of multiple delays
- [ ] Optimize for multiple simultaneous monopulse calculations
- [ ] Add per-track phase delay state

### Performance Optimization
- [ ] Profile multi-target performance
- [ ] Optimize FFT reuse across tracks
- [ ] Consider worker pool for parallel track updates
- [ ] Benchmark vs single-target mode

---

## Phase 4: Tracker Logic Updates ⏱️ 4-5 hours

### Single-Target Mode (Option 1)
- [ ] Keep existing behavior as default
- [ ] Select highest SNR/peak from multi-peak detection
- [ ] Maintain single `lastDelay` tracking
- [ ] Ensure no performance regression

### Multi-Target Mode (Option 2)
- [ ] Implement `TrackManager.Update()` with new detections
- [ ] Run monopulse tracking for each confirmed track
- [ ] Update track states based on measurements
- [ ] Prune lost/timed-out tracks
- [ ] Limit to `max-tracks` simultaneous tracks

### Mode Switching
- [ ] Runtime mode selection via config
- [ ] Clean transition between modes
- [ ] Proper state reset when switching modes

---

## Phase 5: Web UI Enhancements ⏱️ 5-6 hours

### Radar Display Updates
- [ ] Support multiple target markers on radar
- [ ] Color-code tracks by state (Tentative/Confirmed/Lost)
- [ ] Add track IDs to display
- [ ] Show track history trails (optional)
- [ ] Add track selection/highlighting

### Track List Panel
- [ ] Create new "Tracks" panel in Telemetry tab
- [ ] Display table of active tracks (ID, Angle, SNR, Confidence, State, Age)
- [ ] Add sort/filter capabilities
- [ ] Show track count summary
- [ ] Add track selection for detail view

### Charts & Statistics
- [ ] Support multi-track angle chart (multiple lines)
- [ ] Add track-specific statistics
- [ ] Color-code chart lines by track ID
- [ ] Add legend for track identification
- [ ] Update statistics panel for selected track

### Settings UI
- [ ] Add tracking mode selector (Single/Multi)
- [ ] Add max tracks slider
- [ ] Add track timeout setting
- [ ] Add SNR threshold setting
- [ ] Show current tracking mode in summary

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
