// Tab management
const tabButtons = document.querySelectorAll('.tab-button');
const tabPanels = document.querySelectorAll('.tab-panel');

let telemetryActive = true;
let telemetryStream = null;
let historyLoaded = false;
let diagnosticsTimer = null;

function setActiveTab(tabId) {
  tabButtons.forEach((btn) => {
    const isActive = btn.dataset.tab === tabId;
    btn.classList.toggle('active', isActive);
    btn.setAttribute('aria-selected', isActive);
  });

  tabPanels.forEach((panel) => {
    const isActive = panel.id === tabId;
    panel.classList.toggle('active', isActive);
  });

  setTelemetryActive(tabId === 'telemetry');
  toggleDiagnosticsRefresh(tabId === 'debug');
}

tabButtons.forEach((btn) => {
  btn.addEventListener('click', () => setActiveTab(btn.dataset.tab));
});

function setTelemetryActive(isActive) {
  telemetryActive = isActive;
  if (!telemetryStream) {
    startTelemetry();
  }
}

function startTelemetry() {
  if (!historyLoaded) {
    fetch('/api/history')
      .then((res) => res.json())
      .then((data) => {
        historyLoaded = true;
        data.forEach((sample) => handleSample(sample, true));
      })
      .catch((err) => console.error('history', err));
  }

  if (telemetryStream) return;

  telemetryStream = new EventSource('/api/live');
  telemetryStream.onmessage = (event) => {
    try {
      const sample = JSON.parse(event.data);
      pendingSample = sample;
      scheduleSampleRender();
    } catch (err) {
      console.error('parse sample', err);
    }
  };
  telemetryStream.onerror = (err) => console.error('sse error', err);
}

function stopTelemetry() {
  if (telemetryStream) {
    telemetryStream.close();
    telemetryStream = null;
  }
}

// Radar Configuration
const radarCanvas = document.getElementById('radarCanvas');
const radarCtx = radarCanvas.getContext('2d');
const radarAngleDisplay = document.getElementById('radarAngle');
const snrDisplay = document.getElementById('snrValue');
const confidenceDisplay = document.getElementById('confidenceValue');
const lockBadge = document.getElementById('lockBadge');
const summaryBackend = document.getElementById('summaryBackend');
const summaryRxLo = document.getElementById('summaryRxLo');
const summaryToneOffset = document.getElementById('summaryToneOffset');
const summarySampleRate = document.getElementById('summarySampleRate');
const summaryFftSize = document.getElementById('summaryFftSize');
const updateRateDisplay = document.getElementById('updateRate');
const angleStatsEls = {
  avg: document.getElementById('angleAvg'),
  std: document.getElementById('angleStd'),
  min: document.getElementById('angleMin'),
  max: document.getElementById('angleMax'),
};
const peakStatsEls = {
  avg: document.getElementById('peakAvg'),
  std: document.getElementById('peakStd'),
  min: document.getElementById('peakMin'),
  max: document.getElementById('peakMax'),
};

const numberFormatter = new Intl.NumberFormat('en-US');

// Radar dimensions
const radarCenterX = radarCanvas.width / 2;
const radarCenterY = radarCanvas.height - 20;
const radarMaxRadius = radarCenterY - 30;

// Range rings (in cm, for display only)
// Define a maximum range (in cm) and create 5 evenly spaced rings
const MAX_RANGE_CM = 100;          // total radar range in cm (must be divisible by 5)
const NUM_RANGE_RINGS = 5;         // number of rings
const rangeRings = Array.from(
  { length: NUM_RANGE_RINGS },
  (_, i) => (MAX_RANGE_CM / NUM_RANGE_RINGS) * (i + 1)
);

// Current detection
// Tracker coordinates: -90° (left) .. 0° (ahead/top) .. +90° (right)
let currentAngleDeg = 0;                 // start pointing straight ahead
let currentRange = MAX_RANGE_CM / 2;     // start at mid-range

function drawRadar() {
  // Clear canvas
  radarCtx.fillStyle = '#000000';
  radarCtx.fillRect(0, 0, radarCanvas.width, radarCanvas.height);

  // Draw range rings
  radarCtx.strokeStyle = '#00ff00';
  radarCtx.lineWidth = 1;

  rangeRings.forEach((range) => {
    // radius proportional to the actual range value
    const radius = radarMaxRadius * (range / MAX_RANGE_CM);

    radarCtx.beginPath();
    // Semicircle above the center
    radarCtx.arc(radarCenterX, radarCenterY, radius, Math.PI, 0, false);
    radarCtx.stroke();

    // Range labels
    radarCtx.fillStyle = '#00ff00';
    radarCtx.font = '10px monospace';
    radarCtx.fillText(`${range}`, radarCenterX + radius + 5, radarCenterY - 5);
  });

  // Draw static angle lines (every 10 degrees in tracker coords: -90..90)
  radarCtx.strokeStyle = '#00ff00';
  radarCtx.lineWidth = 0.5;

  for (let angleDeg = -90; angleDeg <= 90; angleDeg += 10) {
    // Map tracker angle to canvas angle:
    // 0° (ahead)   -> -90° (up)
    // +90° (right) ->   0° (right)
    // -90° (left)  -> -180° (left)
    const rad = (angleDeg - 90) * Math.PI / 180;

    const x = radarCenterX + radarMaxRadius * Math.cos(rad);
    const y = radarCenterY + radarMaxRadius * Math.sin(rad);

    radarCtx.beginPath();
    radarCtx.moveTo(radarCenterX, radarCenterY);
    radarCtx.lineTo(x, y);
    radarCtx.stroke();

    // Angle labels at -90°, -45°, 0°, 45°, 90°
    if (angleDeg % 45 === 0) {
      radarCtx.fillStyle = '#00ff00';
      radarCtx.font = '12px monospace';
      const labelX = radarCenterX + (radarMaxRadius + 15) * Math.cos(rad);
      const labelY = radarCenterY + (radarMaxRadius + 15) * Math.sin(rad);
      radarCtx.fillText(`${angleDeg}°`, labelX - 12, labelY + 4);
    }
  }

  // Draw detection marker (red dot) using the SAME mapping & scaling
  const detectionRad = (currentAngleDeg - 90) * Math.PI / 180;
  const detectionRadius = radarMaxRadius * (currentRange / MAX_RANGE_CM);
  const detectionX = radarCenterX + detectionRadius * Math.cos(detectionRad);
  const detectionY = radarCenterY + detectionRadius * Math.sin(detectionRad);

  // Draw glow effect
  radarCtx.shadowBlur = 15;
  radarCtx.shadowColor = '#ff0000';
  radarCtx.fillStyle = '#ff0000';
  radarCtx.beginPath();
  radarCtx.arc(detectionX, detectionY, 6, 0, 2 * Math.PI);
  radarCtx.fill();
  radarCtx.shadowBlur = 0;
}

function updateRadar(angleDeg) {
  // angleDeg comes in as tracker coords [-90, 90]
  currentAngleDeg = Math.max(-90, Math.min(90, angleDeg));

  drawRadar();
  if (radarAngleDisplay) {
    radarAngleDisplay.textContent = `Angle: ${currentAngleDeg.toFixed(1)}°`;
  }
}

// Initial draw
drawRadar();

function createChart(elementId, label, color, yTitle) {
  return new Chart(document.getElementById(elementId), {
    type: 'line',
    data: { labels: [], datasets: [{ label, data: [], borderColor: color, tension: 0.2, pointRadius: 0 }] },
    options: {
      scales: {
        x: { display: false },
        y: { title: { display: true, text: yTitle }, ticks: { color: '#cbd5e1' } }
      },
      color: '#cbd5e1',
      animation: false,
      responsive: true,
      plugins: { legend: { labels: { color: '#cbd5e1' } } }
    }
  });
}

const angleChart = createChart('angleChart', 'Angle (deg)', '#2f80ed', 'Degrees');
const peakChart = createChart('peakChart', 'Peak (dBFS)', '#9b59b6', 'dBFS');
const snrChart = createChart('snrChart', 'SNR (dB)', '#27ae60', 'dB');
const confidenceChart = createChart('confidenceChart', 'Confidence (%)', '#f59e0b', 'Percent');

const MAX_POINTS = 100;
const TRACE_MAX_ROWS = 500;
const TRACE_ROW_HEIGHT = 36;

// Debug panel elements
const debugStatus = document.getElementById('debugStatus');
const diagLastUpdated = document.getElementById('debugLastUpdated');
const diagUptime = document.getElementById('diagUptime');
const diagStartTime = document.getElementById('diagStartTime');
const diagSamples = document.getElementById('diagSamples');
const diagLastSample = document.getElementById('diagLastSample');
const diagUpdateRate = document.getElementById('diagUpdateRate');
const diagCpuLoad = document.getElementById('diagCpuLoad');
const diagMemAlloc = document.getElementById('diagMemAlloc');
const diagMemSys = document.getElementById('diagMemSys');
const diagGoroutines = document.getElementById('diagGoroutines');
const diagIterationTime = document.getElementById('diagIterationTime');
const diagSNR = document.getElementById('diagSNR');
const diagNoiseFloor = document.getElementById('diagNoiseFloor');
const diagConfidence = document.getElementById('diagConfidence');
const diagLockState = document.getElementById('diagLockState');
const eventLogBody = document.getElementById('eventLog');
const debugPhaseDelay = document.getElementById('debugPhaseDelay');
const debugMonopulsePhase = document.getElementById('debugMonopulsePhase');
const debugPeakValue = document.getElementById('debugPeakValue');
const debugPeakBin = document.getElementById('debugPeakBin');
const debugPeakBand = document.getElementById('debugPeakBand');
let debugStreamEnabled = false;
const statsState = {
  angle: [],
  peak: [],
  intervals: [],
  lastUpdate: null,
};

const traceViewport = document.getElementById('traceViewport');
const traceVirtualList = document.getElementById('traceVirtualList');
const tracePauseBtn = document.getElementById('tracePauseBtn');
const traceClearBtn = document.getElementById('traceClearBtn');
const traceCountEl = document.getElementById('traceCount');
const traceStatusEl = document.getElementById('traceStatus');
const traceExportFormat = document.getElementById('traceExportFormat');
const traceCopyBtn = document.getElementById('traceCopyBtn');
const traceDownloadBtn = document.getElementById('traceDownloadBtn');
const traceState = {
  buffer: [],
  paused: false,
};

function updateTraceSummary() {
  if (traceCountEl) {
    traceCountEl.textContent = `${traceState.buffer.length} / ${TRACE_MAX_ROWS}`;
  }
  if (traceStatusEl) {
    traceStatusEl.textContent = traceState.paused ? 'paused' : 'live';
    traceStatusEl.classList.toggle('paused', traceState.paused);
  }
  if (tracePauseBtn) {
    tracePauseBtn.textContent = traceState.paused ? 'Resume' : 'Pause';
  }
}

function setTracePaused(paused) {
  traceState.paused = paused;
  updateTraceSummary();
}

function clearTraceHistory() {
  traceState.buffer = [];
  renderTraceRows();
  updateTraceSummary();
}

function formatTraceTimestamp(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '--';
  }
  return date.toLocaleTimeString();
}

function renderTraceRows() {
  if (!traceVirtualList || !traceViewport) return;
  const total = traceState.buffer.length;
  const viewportHeight = traceViewport.clientHeight || 0;
  const scrollTop = traceViewport.scrollTop;
  const start = Math.floor(scrollTop / TRACE_ROW_HEIGHT);
  const visibleCount = Math.ceil(viewportHeight / TRACE_ROW_HEIGHT) + 5;
  const end = Math.min(total, start + visibleCount);

  traceVirtualList.innerHTML = '';
  traceVirtualList.style.height = `${total * TRACE_ROW_HEIGHT}px`;

  for (let i = start; i < end; i += 1) {
    const entry = traceState.buffer[i];
    const row = document.createElement('div');
    row.className = 'trace-row';
    if (i % 2 === 1) {
      row.classList.add('trace-row-alt');
    }
    row.style.top = `${i * TRACE_ROW_HEIGHT}px`;
    row.style.height = `${TRACE_ROW_HEIGHT}px`;
    row.setAttribute('role', 'row');
    row.innerHTML = `
      <div role="cell">${formatTraceTimestamp(entry.timestamp)}</div>
      <div role="cell">${Number.isFinite(entry.angleDeg) ? entry.angleDeg.toFixed(2) : '--'}</div>
      <div role="cell">${Number.isFinite(entry.peak) ? entry.peak.toFixed(2) : '--'}</div>
    `;
    traceVirtualList.appendChild(row);
  }
}

function isTraceNearBottom() {
  if (!traceViewport) return true;
  const { scrollTop, scrollHeight, clientHeight } = traceViewport;
  return scrollHeight - (scrollTop + clientHeight) < TRACE_ROW_HEIGHT * 1.5;
}

function scrollTraceToBottom() {
  if (!traceViewport) return;
  traceViewport.scrollTop = traceViewport.scrollHeight;
}

function addTraceSample(sample) {
  if (!traceVirtualList || traceState.paused) return;
  const entry = {
    timestamp: sample.timestamp,
    angleDeg: sample.angleDeg,
    peak: sample.peak,
  };
  traceState.buffer.push(entry);
  if (traceState.buffer.length > TRACE_MAX_ROWS) {
    traceState.buffer.splice(0, traceState.buffer.length - TRACE_MAX_ROWS);
  }

  const stickToBottom = isTraceNearBottom();
  renderTraceRows();
  updateTraceSummary();
  if (stickToBottom) {
    scrollTraceToBottom();
  }
}

function serializeTrace(format) {
  if (format === 'json') {
    return JSON.stringify(traceState.buffer, null, 2);
  }

  const header = 'timestamp,angleDeg,peak';
  const rows = traceState.buffer.map((entry) => {
    const angle = Number.isFinite(entry.angleDeg) ? entry.angleDeg.toFixed(4) : '';
    const peak = Number.isFinite(entry.peak) ? entry.peak.toFixed(4) : '';
    const ts = entry.timestamp ?? '';
    return `${ts},${angle},${peak}`;
  });
  return [header, ...rows].join('\n');
}

async function handleTraceCopy() {
  if (!navigator.clipboard) {
    console.warn('Clipboard API unavailable');
    return;
  }
  const format = traceExportFormat?.value || 'csv';
  const payload = serializeTrace(format);
  await navigator.clipboard.writeText(payload);
}

function handleTraceDownload() {
  const format = traceExportFormat?.value || 'csv';
  const payload = serializeTrace(format);
  const blob = new Blob([payload], { type: format === 'json' ? 'application/json' : 'text/csv' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = format === 'json' ? 'trace.json' : 'trace.csv';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

const CONFIG_REFRESH_MS = 5000;

// Rate limiting for SSE updates (10 Hz cap + animation frame batching)
const FRAME_INTERVAL_MS = 100;
let pendingSample = null;
let frameScheduled = false;
let lastFrameTime = 0;

function scheduleSampleRender() {
  if (frameScheduled) return;
  frameScheduled = true;
  requestAnimationFrame(processPendingSample);
}

function processPendingSample(timestamp) {
  if (!pendingSample) {
    frameScheduled = false;
    return;
  }

  const elapsed = timestamp - lastFrameTime;
  if (elapsed < FRAME_INTERVAL_MS) {
    requestAnimationFrame(processPendingSample);
    return;
  }

  frameScheduled = false;
  lastFrameTime = timestamp;

  const sample = pendingSample;
  pendingSample = null;
  handleSample(sample);

  if (pendingSample) {
    scheduleSampleRender();
  }
}

function handleSample(sample, fromHistory = false) {
  addTraceSample(sample);
  if (!telemetryActive) return;
  addSample(sample, fromHistory);
}

function addSample(sample, fromHistory = false) {
  const timestamp = new Date(sample.timestamp).toLocaleTimeString();
  pushPoint(angleChart, timestamp, sample.angleDeg);
  pushPoint(peakChart, timestamp, sample.peak);
  pushPoint(snrChart, timestamp, sample.snr ?? 0);
  const confidencePercent = Math.max(0, Math.min(1, sample.trackingConfidence ?? 0)) * 100;
  pushPoint(confidenceChart, timestamp, confidencePercent);

  // Update radar display
  updateRadar(sample.angleDeg);
  updateMetrics(sample.snr, confidencePercent, sample.lockState);

  updateDebugPanel(sample);
  updateStats(sample, fromHistory);
}

function updateMetrics(snr, confidencePercent, lockState) {
  if (snrDisplay) {
    snrDisplay.textContent = Number.isFinite(snr) ? `${snr.toFixed(1)} dB` : '-- dB';
  }
  if (confidenceDisplay) {
    const safe = Number.isFinite(confidencePercent) ? confidencePercent : 0;
    confidenceDisplay.textContent = `${safe.toFixed(0)}%`;
  }
  updateLockBadge(lockState);
}

function updateLockBadge(state, badgeEl = lockBadge) {
  if (!badgeEl) return;
  const normalized = (state || 'searching').toLowerCase();
  badgeEl.textContent = normalized;
  badgeEl.classList.remove('locked', 'tracking', 'searching');
  if (normalized === 'locked') {
    badgeEl.classList.add('locked');
  } else if (normalized === 'tracking') {
    badgeEl.classList.add('tracking');
  } else {
    badgeEl.classList.add('searching');
  }
}

function updateDebugPanel(sample) {
  if (!debugStatus) return;
  const info = sample?.debug ?? sample;
  if (!info) {
    if (!debugStreamEnabled) {
      debugStatus.textContent = 'Debug mode disabled';
    }
    return;
  }

  debugStreamEnabled = true;
  debugStatus.textContent = 'Live debug mode enabled';

  const safeBand = Array.isArray(info.peak?.band) ? info.peak.band : [];
  const bandLabel = safeBand.length === 2 ? `[${safeBand[0]}, ${safeBand[1]})` : '--';

  if (debugPhaseDelay) {
    debugPhaseDelay.textContent = Number.isFinite(info.phaseDelayDeg)
      ? `${info.phaseDelayDeg.toFixed(2)}°`
      : '--';
  }
  if (debugMonopulsePhase) {
    debugMonopulsePhase.textContent = Number.isFinite(info.monopulsePhaseRad)
      ? info.monopulsePhaseRad.toFixed(3)
      : '--';
  }
  if (debugPeakValue) {
    debugPeakValue.textContent = Number.isFinite(info.peak?.value)
      ? `${info.peak.value.toFixed(2)} dBFS`
      : '--';
  }
  if (debugPeakBin) {
    debugPeakBin.textContent = Number.isFinite(info.peak?.bin) ? info.peak.bin : '--';
  }
  if (debugPeakBand) {
    debugPeakBand.textContent = bandLabel;
  }
}

function formatDuration(ns) {
  if (!Number.isFinite(ns)) return '--';
  const totalSeconds = ns / 1e9;
  if (totalSeconds < 1) return `${(totalSeconds * 1000).toFixed(0)} ms`;
  if (totalSeconds < 60) return `${totalSeconds.toFixed(1)} s`;
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = Math.floor(totalSeconds % 60);
  const hours = Math.floor(minutes / 60);
  if (hours > 0) {
    return `${hours}h ${minutes % 60}m`;
  }
  return `${minutes}m ${seconds}s`;
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes)) return '--';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatTimestamp(value) {
  if (!value) return '--';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  return date.toLocaleString();
}

function renderEventLog(events = []) {
  if (!eventLogBody) return;
  eventLogBody.innerHTML = '';
  if (!events.length) {
    const row = document.createElement('div');
    row.className = 'event-row';
    row.innerHTML = '<span>--</span><span>--</span><span>No events yet</span>';
    eventLogBody.appendChild(row);
    return;
  }

  for (let idx = events.length - 1; idx >= 0; idx -= 1) {
    const evt = events[idx];
    const row = document.createElement('div');
    row.className = 'event-row';
    const timeEl = document.createElement('span');
    timeEl.textContent = formatTimestamp(evt.timestamp);
    const levelEl = document.createElement('span');
    levelEl.textContent = (evt.level || '').toUpperCase();
    const msgEl = document.createElement('span');
    msgEl.textContent = evt.message || '';
    row.append(timeEl, levelEl, msgEl);
    eventLogBody.appendChild(row);
  }
}

function renderDiagnostics(diag) {
  if (!diag || !diag.process) {
    if (debugStatus) debugStatus.textContent = 'Diagnostics unavailable';
    return;
  }

  if (debugStatus) debugStatus.textContent = 'Diagnostics live';
  if (diagLastUpdated) diagLastUpdated.textContent = formatTimestamp(diag.process.lastUpdated);
  if (diagUptime) diagUptime.textContent = formatDuration(diag.process.uptime);
  if (diagStartTime) diagStartTime.textContent = formatTimestamp(diag.process.startTime);
  if (diagSamples) diagSamples.textContent = numberFormatter.format(diag.process.samples || 0);
  if (diagLastSample) diagLastSample.textContent = formatTimestamp(diag.process.lastSample || diag.signal?.updatedAt);
  if (diagUpdateRate) {
    diagUpdateRate.textContent = Number.isFinite(diag.process.updateRateHz)
      ? `${diag.process.updateRateHz.toFixed(2)} Hz`
      : '--';
  }

  if (diagCpuLoad) {
    diagCpuLoad.textContent = Number.isFinite(diag.process.cpuPercent)
      ? `${diag.process.cpuPercent.toFixed(1)}%`
      : '--';
  }
  if (diagMemAlloc) diagMemAlloc.textContent = formatBytes(diag.process.memoryAllocBytes);
  if (diagMemSys) diagMemSys.textContent = formatBytes(diag.process.memorySysBytes);
  if (diagGoroutines) diagGoroutines.textContent = diag.process.numGoroutine ?? '--';
  if (diagIterationTime) {
    const last = formatDuration(diag.process.iterationLast);
    const avg = formatDuration(diag.process.iterationAvg);
    diagIterationTime.textContent = `${last} / ${avg}`;
  }

  const signal = diag.signal || {};
  if (diagSNR) {
    diagSNR.textContent = Number.isFinite(signal.snr) ? `${signal.snr.toFixed(1)} dB` : '--';
  }
  if (diagNoiseFloor) {
    diagNoiseFloor.textContent = Number.isFinite(signal.noiseFloor)
      ? `${signal.noiseFloor.toFixed(1)} dBFS`
      : '--';
  }
  if (diagConfidence) {
    const confPct = Number.isFinite(signal.confidence) ? signal.confidence * 100 : NaN;
    diagConfidence.textContent = Number.isFinite(confPct) ? `${confPct.toFixed(0)}%` : '--';
  }
  updateLockBadge(signal.lockState, diagLockState);

  updateDebugPanel(diag.debug);
  renderEventLog(diag.events);
}

function toggleDiagnosticsRefresh(enable) {
  if (enable) {
    if (!diagnosticsTimer) {
      fetchDiagnostics();
      diagnosticsTimer = setInterval(fetchDiagnostics, 5000);
    }
    return;
  }
  if (diagnosticsTimer) {
    clearInterval(diagnosticsTimer);
    diagnosticsTimer = null;
  }
}

async function fetchDiagnostics() {
  try {
    const res = await fetch('/api/diagnostics');
    if (!res.ok) throw new Error(`status ${res.status}`);
    const payload = await res.json();
    renderDiagnostics(payload);
  } catch (err) {
    console.error('diagnostics fetch failed', err);
    if (debugStatus) {
      debugStatus.textContent = 'Diagnostics unavailable';
    }
  }
}

function renderStatValue(el, value, precision = 2) {
  if (!el) return;
  if (!Number.isFinite(value)) {
    el.textContent = '--';
    return;
  }
  el.textContent = value.toFixed(precision);
}

function computeStats(values) {
  if (!values.length) {
    return null;
  }
  const count = values.length;
  const sum = values.reduce((acc, v) => acc + v, 0);
  const avg = sum / count;
  const variance = values.reduce((acc, v) => acc + (v - avg) ** 2, 0) / count;
  return {
    avg,
    std: Math.sqrt(variance),
    min: Math.min(...values),
    max: Math.max(...values),
  };
}

function recordStatValue(bucket, value) {
  if (!Number.isFinite(value)) return;
  bucket.push(value);
  if (bucket.length > MAX_POINTS) {
    bucket.shift();
  }
}

function updateStats(sample, fromHistory) {
  recordStatValue(statsState.angle, sample.angleDeg);
  recordStatValue(statsState.peak, sample.peak);

  const angleStats = computeStats(statsState.angle);
  if (angleStats) {
    renderStatValue(angleStatsEls.avg, angleStats.avg, 2);
    renderStatValue(angleStatsEls.std, angleStats.std, 2);
    renderStatValue(angleStatsEls.min, angleStats.min, 2);
    renderStatValue(angleStatsEls.max, angleStats.max, 2);
  }

  const peakStats = computeStats(statsState.peak);
  if (peakStats) {
    renderStatValue(peakStatsEls.avg, peakStats.avg, 2);
    renderStatValue(peakStatsEls.std, peakStats.std, 2);
    renderStatValue(peakStatsEls.min, peakStats.min, 2);
    renderStatValue(peakStatsEls.max, peakStats.max, 2);
  }

  if (!fromHistory) {
    updateUpdateRate();
  }
}

function updateUpdateRate() {
  const now = performance.now();
  if (statsState.lastUpdate !== null) {
    const interval = now - statsState.lastUpdate;
    statsState.intervals.push(interval);
    if (statsState.intervals.length > MAX_POINTS) {
      statsState.intervals.shift();
    }
    const sum = statsState.intervals.reduce((acc, v) => acc + v, 0);
    const avgInterval = sum / statsState.intervals.length;
    const rateHz = avgInterval > 0 ? 1000 / avgInterval : 0;
    if (updateRateDisplay) {
      updateRateDisplay.textContent = `${rateHz.toFixed(1)} Hz`;
    }
  }
  statsState.lastUpdate = now;
}

function formatHz(value) {
  if (!Number.isFinite(value)) return '--';
  const abs = Math.abs(value);
  if (abs >= 1e9) return `${(value / 1e9).toFixed(2)} GHz`;
  if (abs >= 1e6) return `${(value / 1e6).toFixed(2)} MHz`;
  if (abs >= 1e3) return `${(value / 1e3).toFixed(2)} kHz`;
  return `${numberFormatter.format(value)} Hz`;
}

function formatSamples(value) {
  if (!Number.isFinite(value)) return '--';
  return numberFormatter.format(value);
}

function updateSummaryPanel(cfg) {
  if (!cfg) return;
  if (summaryBackend) {
    summaryBackend.textContent = cfg.sdrBackend || '--';
  }
  if (summaryRxLo) {
    summaryRxLo.textContent = formatHz(cfg.rxLoHz);
  }
  if (summaryToneOffset) {
    summaryToneOffset.textContent = formatHz(cfg.toneOffsetHz);
  }
  if (summarySampleRate) {
    summarySampleRate.textContent = formatHz(cfg.sampleRateHz);
  }
  if (summaryFftSize) {
    summaryFftSize.textContent = formatSamples(cfg.numSamples);
  }
}

async function refreshConfigSummary() {
  try {
    const res = await fetch('/api/config');
    if (!res.ok) return;
    const cfg = await res.json();
    updateSummaryPanel(cfg);
  } catch (err) {
    console.error('config summary', err);
  }
}

function pushPoint(chart, label, value) {
  chart.data.labels.push(label);
  chart.data.datasets[0].data.push(value);
  if (chart.data.labels.length > MAX_POINTS) {
    chart.data.labels.shift();
    chart.data.datasets[0].data.shift();
  }
  chart.update('none');
}

refreshConfigSummary();
setInterval(refreshConfigSummary, CONFIG_REFRESH_MS);

if (traceViewport) {
  traceViewport.addEventListener('scroll', renderTraceRows);
}
if (tracePauseBtn) {
  tracePauseBtn.addEventListener('click', () => setTracePaused(!traceState.paused));
}
if (traceClearBtn) {
  traceClearBtn.addEventListener('click', clearTraceHistory);
}
if (traceCopyBtn) {
  traceCopyBtn.addEventListener('click', handleTraceCopy);
}
if (traceDownloadBtn) {
  traceDownloadBtn.addEventListener('click', handleTraceDownload);
}

updateTraceSummary();
renderTraceRows();

setActiveTab('telemetry');

// Manual QA checklist:
// 1. Load page and confirm Telemetry tab is active with charts and radar visible.
// 2. Switch to Trace/Debug/Settings tabs and verify Telemetry visuals hide while placeholders render.
// 3. Return to Telemetry and confirm data resumes updating without console errors.
// 4. Let more than 100 samples stream in and confirm chart/stat windows cap at 100, scroll smoothly, and Chrome shows no performance warnings.
