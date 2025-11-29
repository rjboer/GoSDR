# Task: Persistent JSON Configuration System

## Phase 1: Configuration File Support
- [ ] Add `persistentConfig` struct to main.go
- [ ] Implement `loadOrCreateConfig()` function
- [ ] Implement `saveConfig()` function
- [ ] Implement `defaultPersistentConfig()` with web defaults
- [ ] Update [main()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/main.go#16-21) to load config.json
- [ ] Change hardcoded defaults (webAddr=":8080", sdrBackend="mock")
- [ ] Test: First run creates config.json

## Phase 2: Web Interface Integration
- [ ] Add SDR fields to `telemetry.Config` struct
- [ ] Update [settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html) with SDR backend dropdown
- [ ] Update [settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html) with SDR URI field
- [ ] Update [settings.html](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.html) with Mock phase delta field
- [ ] Update [settings.js](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/static/settings.js) to include SDR fields
- [ ] Add show/hide logic for SDR-specific fields
- [ ] Modify [handleSetConfig](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/telemetry/hub.go#260-289) to save to config.json
- [ ] Test: Change settings via web, verify persistence

## Phase 3: Validation & Polish
- [ ] Add validation for SDR backend values
- [ ] Add UI feedback for save success/failure
- [ ] Add "Restart Required" message for SDR changes
- [ ] Update program_overview.md documentation
- [ ] Test all scenarios (first run, load, override, web change)
- [ ] Test SDR backend switching

## Expected Results
- ✅ No command-line flags needed to run
- ✅ Web interface enabled by default
- ✅ MockSDR selected by default
- ✅ Settings persist across restarts
- ✅ SDR backend switchable via web UI
- ✅ config.json auto-created if missing
