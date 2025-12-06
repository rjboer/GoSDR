// Tab management
const tabButtons = document.querySelectorAll('.tab-button');
const tabPanels = document.querySelectorAll('.tab-panel');

let telemetryActive = true;
let telemetryStream = null;
let historyLoaded = false;
let diagnosticsTimer = null;
let metricsStream = null;
let healthTimer = null;

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

let configSnapshot = {};

const numberFormatter = new Intl.NumberFormat('en-US');

// Radar dimensions
const radarCenterX = radarCanvas.width / 2;
const radarCenterY = radarCanvas.height - 20;
const radarMaxRadius = radarCenterY - 30;

// Range rings (in cm, for display only)
const MAX_RANGE_CM = 100; // total radar range in cm (must be divisible by 5)
const NUM_RANGE_RINGS = 5; // number of rings
const rangeRings = Array.from(
  { length: NUM_RANGE_RINGS },
  (_, i) => (MAX_RANGE_CM / NUM_RANGE_RINGS) * (i + 1)
);

const trackPalette = ['#2f80ed', '#9b59b6', '#27ae60', '#f59e0b', '#ef4444', '#10b981', '#a855f7', '#22c55e'];
const lockStateColors = {
  locked: '#22c55e',
  tracking: '#f59e0b',
  searching: '#94a3b8',
};

let paletteIndex = 0;
const trackStore = new Map();
const TRACK_HISTORY_LIMIT = 30;

function colorForState(lockState) {
  return lockStateColors[lockState] || '#94a3b8';
}

function colorForTrack(trackId) {
  if (trackStore.has(trackId) && trackStore.get(trackId).color) {
    return trackStore.get(trackId).color;
  }
  const color = trackPalette[paletteIndex % trackPalette.length];
  paletteIndex += 1;
  return color;
}

function angleToCoordinates(angleDeg, rangeCm) {
  const clampedAngle = Math.max(-90, Math.min(90, angleDeg));
  const clampedRange = Math.max(0, Math.min(MAX_RANGE_CM, rangeCm ?? MAX_RANGE_CM / 2));
  const rad = (clampedAngle - 90) * Math.PI / 180;
  const radius = radarMaxRadius * (clampedRange / MAX_RANGE_CM);
  const x = radarCenterX + radius * Math.cos(rad);
  const y = radarCenterY + radius * Math.sin(rad);
  return { x, y };
}

function drawRadar(tracks = []) {
  radarCtx.fillStyle = '#000000';
  radarCtx.fillRect(0, 0, radarCanvas.width, radarCanvas.height);

  radarCtx.strokeStyle = '#00ff00';
  radarCtx.lineWidth = 1;

  rangeRings.forEach((range) => {
    const radius = radarMaxRadius * (range / MAX_RANGE_CM);
    radarCtx.beginPath();
    radarCtx.arc(radarCenterX, radarCenterY, radius, Math.PI, 0, false);
    radarCtx.stroke();
    radarCtx.fillStyle = '#00ff00';
    radarCtx.font = '10px monospace';
    radarCtx.fillText(`${range}`, radarCenterX + radius + 5, radarCenterY - 5);
  });

  radarCtx.strokeStyle = '#00ff00';
  radarCtx.lineWidth = 0.5;

  for (let angleDeg = -90; angleDeg <= 90; angleDeg += 10) {
    const rad = (angleDeg - 90) * Math.PI / 180;
    const x = radarCenterX + radarMaxRadius * Math.cos(rad);
    const y = radarCenterY + radarMaxRadius * Math.sin(rad);

    radarCtx.beginPath();
    radarCtx.moveTo(radarCenterX, radarCenterY);
    radarCtx.lineTo(x, y);
    radarCtx.stroke();

    if (angleDeg % 45 === 0) {
      radarCtx.fillStyle = '#00ff00';
      radarCtx.font = '12px monospace';
      const labelX = radarCenterX + (radarMaxRadius + 15) * Math.cos(rad);
      const labelY = radarCenterY + (radarMaxRadius + 15) * Math.sin(rad);
      radarCtx.fillText(`${angleDeg}°`, labelX - 12, labelY + 4);
    }
  }

  tracks.forEach((track) => {
    const { x, y } = angleToCoordinates(track.angleDeg, track.range);
    const stateColor = colorForState(track.lockState);
    const trackColor = track.color || colorForTrack(track.id);

    if (Array.isArray(track.history) && track.history.length > 1) {
      radarCtx.beginPath();
      radarCtx.strokeStyle = trackColor;
      radarCtx.lineWidth = 1;
      track.history.forEach((point, idx) => {
        const coords = angleToCoordinates(point.angleDeg, point.range);
        if (idx === 0) {
          radarCtx.moveTo(coords.x, coords.y);
        } else {
          radarCtx.lineTo(coords.x, coords.y);
        }
      });
      radarCtx.stroke();
    }

    radarCtx.shadowBlur = 15;
    radarCtx.shadowColor = stateColor;
    radarCtx.fillStyle = stateColor;
    radarCtx.strokeStyle = trackColor;
    radarCtx.lineWidth = 2;
    radarCtx.beginPath();
    radarCtx.arc(x, y, 6, 0, 2 * Math.PI);
    radarCtx.fill();
    radarCtx.stroke();
    radarCtx.shadowBlur = 0;

    radarCtx.fillStyle = trackColor;
    radarCtx.font = '11px monospace';
    radarCtx.fillText(track.id || 'T', x + 8, y + 4);
  });
}

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

const angleChart = new Chart(document.getElementById('angleChart'), {
  type: 'line',
  data: { labels: [], datasets: [] },
  options: {
    scales: {
      x: { display: false },
      y: { title: { display: true, text: 'Degrees' }, ticks: { color: '#cbd5e1' } }
    },
    color: '#cbd5e1',
    animation: false,
    responsive: true,
    plugins: { legend: { labels: { color: '#cbd5e1' } } }
  }
});
const peakChart = createChart('peakChart', 'Peak (dBFS)', '#9b59b6', 'dBFS');
const snrChart = createChart('snrChart', 'SNR (dB)', '#27ae60', 'dB');
const confidenceChart = createChart('confidenceChart', 'Confidence (%)', '#f59e0b', 'Percent');

const MAX_POINTS = 100;
const TRACE_MAX_ROWS = 500;
const TRACE_ROW_HEIGHT = 36;

const METRIC_THRESHOLDS = {
  cpu: { warn: 75, critical: 90 },
  memAlloc: { warn: 512 * 1024 * 1024, critical: 800 * 1024 * 1024 },
  memRss: { warn: 600 * 1024 * 1024, critical: 900 * 1024 * 1024 },
  goroutines: { warn: 500, critical: 1000 },
  threads: { warn: 150, critical: 250 },
};

// Debug panel elements
const debugStatus = document.getElementById('debugStatus');
const healthStatusBadge = document.getElementById('healthStatusBadge');
const diagVersion = document.getElementById('diagVersion');
const versionApp = document.getElementById('versionApp');
const versionIiod = document.getElementById('versionIiod');
const versionIiodDescription = document.getElementById('versionIiodDescription');
const versionProtocol = document.getElementById('versionProtocol');
const versionFirmware = document.getElementById('versionFirmware');
const healthReason = document.getElementById('healthReason');
const healthChecks = document.getElementById('healthChecks');
const diagLastUpdated = document.getElementById('debugLastUpdated');
const diagUptime = document.getElementById('diagUptime');
const diagStartTime = document.getElementById('diagStartTime');
const diagSamples = document.getElementById('diagSamples');
const diagLastSample = document.getElementById('diagLastSample');
const diagUpdateRate = document.getElementById('diagUpdateRate');
const diagCpuLoad = document.getElementById('diagCpuLoad');
const diagMemAlloc = document.getElementById('diagMemAlloc');
const diagMemSys = document.getElementById('diagMemSys');
const diagMemRss = document.getElementById('diagMemRss');
const diagGoroutines = document.getElementById('diagGoroutines');
const diagThreads = document.getElementById('diagThreads');
const diagIterationTime = document.getElementById('diagIterationTime');
const cpuStatusBadge = document.getElementById('cpuStatusBadge');
const memoryStatusBadge = document.getElementById('memoryStatusBadge');
const goroutineStatusBadge = document.getElementById('goroutineStatusBadge');
const threadStatusBadge = document.getElementById('threadStatusBadge');
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
const trackSortState = { key: 'id', dir: 'asc' };
const perTrackMetrics = new Map();

const tracksTableBody = document.getElementById('tracksTableBody');
const trackFilterInput = document.getElementById('trackFilter');
const trackStateFilter = document.getElementById('trackStateFilter');
const trackStatsContainer = document.getElementById('trackStatsContainer');

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

function normalizeTracks(sample) {
  const tracks = Array.isArray(sample.tracks) ? sample.tracks : [];
  if (!tracks.length) {
    const fallbackLock = (sample.lockState || 'searching').toLowerCase();
    return [{
      id: 'T1',
      angleDeg: sample.angleDeg,
      peak: sample.peak,
      snr: sample.snr,
      trackingConfidence: sample.trackingConfidence,
      lockState: fallbackLock,
      range: sample.range ?? MAX_RANGE_CM / 2,
      ageSeconds: sample.ageSeconds,
    }];
  }

  return tracks.map((track, idx) => {
    const lockState = (track.lockState || sample.lockState || 'searching').toLowerCase();
    return {
      id: track.id || track.trackId || `T${idx + 1}`,
      angleDeg: Number.isFinite(track.angleDeg) ? track.angleDeg : sample.angleDeg,
      peak: Number.isFinite(track.peak) ? track.peak : sample.peak,
      snr: Number.isFinite(track.snr) ? track.snr : sample.snr,
      trackingConfidence: Number.isFinite(track.trackingConfidence) ? track.trackingConfidence : sample.trackingConfidence,
      lockState,
      range: Number.isFinite(track.range) ? track.range : MAX_RANGE_CM / 2,
      ageSeconds: Number.isFinite(track.ageSeconds) ? track.ageSeconds : null,
    };
  });
}

function updateTrackStore(tracks, timestamp) {
  const nowMs = timestamp?.getTime?.() ?? Date.now();
  const seen = new Set();
  const timeoutMs = configSnapshot.trackTimeoutMs || 5000;
  tracks.forEach((track) => {
    const id = track.id || 'T1';
    const existing = trackStore.get(id) || { history: [], firstSeen: nowMs, color: colorForTrack(id) };
    const history = existing.history.slice();
    history.push({ angleDeg: track.angleDeg, range: track.range });
    if (history.length > TRACK_HISTORY_LIMIT) {
      history.shift();
    }
    trackStore.set(id, {
      ...existing,
      id,
      color: existing.color || colorForTrack(id),
      lockState: track.lockState,
      lastSeen: nowMs,
      firstSeen: existing.firstSeen || nowMs,
      last: track,
      history,
    });
    seen.add(id);
  });

  Array.from(trackStore.entries()).forEach(([id, entry]) => {
    if (!seen.has(id) && nowMs - (entry.lastSeen || nowMs) > timeoutMs) {
      trackStore.delete(id);
    }
  });
}

function renderTracksTable() {
  if (!tracksTableBody) return;
  const filterText = (trackFilterInput?.value || '').toLowerCase();
  const filterState = (trackStateFilter?.value || '').toLowerCase();
  const rows = Array.from(trackStore.values()).map((entry) => {
    const ageSeconds = Number.isFinite(entry.last?.ageSeconds)
      ? entry.last.ageSeconds
      : (entry.lastSeen - (entry.firstSeen || entry.lastSeen)) / 1000;
    return {
      id: entry.id,
      angleDeg: entry.last?.angleDeg,
      snr: entry.last?.snr,
      confidence: entry.last?.trackingConfidence,
      lockState: entry.last?.lockState || 'searching',
      ageSeconds,
      color: entry.color,
    };
  });

  const filtered = rows.filter((row) => {
    const matchesFilter = !filterText || row.id.toLowerCase().includes(filterText);
    const matchesState = !filterState || row.lockState === filterState;
    return matchesFilter && matchesState;
  });

  filtered.sort((a, b) => {
    const dir = trackSortState.dir === 'desc' ? -1 : 1;
    const key = trackSortState.key;
    if (key === 'angleDeg' || key === 'snr' || key === 'confidence' || key === 'ageSeconds') {
      return ((a[key] ?? -Infinity) - (b[key] ?? -Infinity)) * dir;
    }
    if (a[key] < b[key]) return -1 * dir;
    if (a[key] > b[key]) return 1 * dir;
    return 0;
  });

  tracksTableBody.innerHTML = '';
  filtered.forEach((row) => {
    const div = document.createElement('div');
    div.className = 'tracks-row';
    div.innerHTML = `
      <span class="track-pill" style="border-color:${row.color}">${row.id}</span>
      <span>${Number.isFinite(row.angleDeg) ? row.angleDeg.toFixed(1) : '--'}</span>
      <span>${Number.isFinite(row.snr) ? row.snr.toFixed(1) : '--'}</span>
      <span>${Number.isFinite(row.confidence) ? `${(row.confidence * 100).toFixed(0)}%` : '--'}</span>
      <span><span class="lock-badge ${row.lockState}">${row.lockState}</span></span>
      <span>${Number.isFinite(row.ageSeconds) ? `${row.ageSeconds.toFixed(1)}s` : '--'}</span>
    `;
    tracksTableBody.appendChild(div);
  });
}

function renderTrackStats() {
  if (!trackStatsContainer) return;
  trackStatsContainer.innerHTML = '';
  trackStore.forEach((entry) => {
    const stats = entry.stats;
    if (!stats) return;
    const card = document.createElement('div');
    card.className = 'track-stat-card';
    card.innerHTML = `
      <div class="track-stat-header">
        <span class="track-pill" style="border-color:${entry.color}">${entry.id}</span>
        <span class="muted">${entry.last?.lockState || 'searching'}</span>
      </div>
      <div class="track-stat-grid">
        <div><p class="muted">Angle avg</p><div class="metric-value">${stats.angle ? stats.angle.avg.toFixed(2) : '--'}</div></div>
        <div><p class="muted">Angle std</p><div class="metric-value">${stats.angle ? stats.angle.std.toFixed(2) : '--'}</div></div>
        <div><p class="muted">SNR avg</p><div class="metric-value">${stats.snr ? stats.snr.avg.toFixed(2) : '--'}</div></div>
        <div><p class="muted">Confidence</p><div class="metric-value">${stats.confidence ? `${(stats.confidence.avg * 100).toFixed(0)}%` : '--'}</div></div>
      </div>
    `;
    trackStatsContainer.appendChild(card);
  });
}

function toggleTrackSort(key) {
  if (trackSortState.key === key) {
    trackSortState.dir = trackSortState.dir === 'asc' ? 'desc' : 'asc';
  } else {
    trackSortState.key = key;
    trackSortState.dir = 'asc';
  }
  renderTracksTable();
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
  const timestampObj = new Date(sample.timestamp);
  const timestamp = timestampObj.toLocaleTimeString();
  const tracks = normalizeTracks(sample);
  const primary = tracks[0];

  pushAngleSeries(timestamp, tracks);
  pushPoint(peakChart, timestamp, primary?.peak);
  pushPoint(snrChart, timestamp, primary?.snr ?? 0);
  const confidencePercent = Math.max(0, Math.min(1, primary?.trackingConfidence ?? 0)) * 100;
  pushPoint(confidenceChart, timestamp, confidencePercent);

  updateTrackStore(tracks, timestampObj);
  const radarTracks = Array.from(trackStore.values()).map((entry) => ({
    id: entry.id,
    angleDeg: entry.last?.angleDeg,
    lockState: entry.last?.lockState,
    range: entry.last?.range,
    color: entry.color,
    history: entry.history,
  }));
  drawRadar(radarTracks);
  if (radarAngleDisplay && primary) {
    radarAngleDisplay.textContent = `${primary.id}: ${primary.angleDeg.toFixed(1)}° (${tracks.length} tracks)`;
  }
  renderTracksTable();

  updateMetrics(primary?.snr, confidencePercent, primary?.lockState || sample.lockState);

  updateDebugPanel(sample);
  updateStats(sample, tracks, fromHistory);
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

function severityFor(value, limits) {
  if (!limits) return 'ok';
  const { warn, critical } = limits;
  if (Number.isFinite(critical) && value >= critical) return 'critical';
  if (Number.isFinite(warn) && value >= warn) return 'warn';
  return 'ok';
}

function setStatusBadge(el, severity, label) {
  if (!el) return;
  const normalized = severity || 'ok';
  el.classList.remove('ok', 'warn', 'critical', 'degraded');
  if (normalized) {
    el.classList.add(normalized);
  }
  el.textContent = label || normalized || '--';
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

function renderProcessMetrics(process) {
  if (!process) return;

  if (diagLastUpdated) diagLastUpdated.textContent = formatTimestamp(process.lastUpdated);
  if (diagUptime) diagUptime.textContent = formatDuration(process.uptime);
  if (diagStartTime) diagStartTime.textContent = formatTimestamp(process.startTime);
  if (diagSamples) diagSamples.textContent = numberFormatter.format(process.samples || 0);
  if (diagLastSample) diagLastSample.textContent = formatTimestamp(process.lastSample);
  if (diagUpdateRate) {
    diagUpdateRate.textContent = Number.isFinite(process.updateRateHz)
      ? `${process.updateRateHz.toFixed(2)} Hz`
      : '--';
  }

  const cpuLabel = Number.isFinite(process.cpuPercent) ? `${process.cpuPercent.toFixed(1)}%` : '--';
  if (diagCpuLoad) diagCpuLoad.textContent = cpuLabel;
  setStatusBadge(cpuStatusBadge, severityFor(process.cpuPercent, METRIC_THRESHOLDS.cpu), cpuLabel);

  if (diagMemAlloc) diagMemAlloc.textContent = formatBytes(process.memoryAllocBytes);
  if (diagMemSys) diagMemSys.textContent = formatBytes(process.memorySysBytes);
  if (diagMemRss) diagMemRss.textContent = formatBytes(process.memoryRssBytes);
  const memSeverity = severityFor(process.memoryAllocBytes, METRIC_THRESHOLDS.memAlloc);
  const rssSeverity = severityFor(process.memoryRssBytes, METRIC_THRESHOLDS.memRss);
  const memoryBadgeSeverity = rssSeverity !== 'ok' ? rssSeverity : memSeverity;
  setStatusBadge(memoryStatusBadge, memoryBadgeSeverity, memoryBadgeSeverity);

  if (diagGoroutines) diagGoroutines.textContent = process.numGoroutine ?? '--';
  if (diagThreads) diagThreads.textContent = process.numThreads ?? '--';
  setStatusBadge(
    goroutineStatusBadge,
    severityFor(process.numGoroutine, METRIC_THRESHOLDS.goroutines),
    Number.isFinite(process.numGoroutine) ? process.numGoroutine.toString() : '--'
  );
  setStatusBadge(
    threadStatusBadge,
    severityFor(process.numThreads, METRIC_THRESHOLDS.threads),
    Number.isFinite(process.numThreads) ? process.numThreads.toString() : '--'
  );

  if (diagIterationTime) {
    const last = formatDuration(process.iterationLast);
    const avg = formatDuration(process.iterationAvg);
    diagIterationTime.textContent = `${last} / ${avg}`;
  }
}

function renderSignalQuality(signal = {}) {
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
}

function renderHealthStatus(health) {
  if (!health) return;
  const status = health.status || 'ok';
  setStatusBadge(healthStatusBadge, status, status);
  if (diagVersion && health.version) {
    diagVersion.textContent = health.version;
  }
  if (healthReason) {
    healthReason.textContent = health.reason || 'All systems nominal';
  }

  if (!healthChecks) return;
  healthChecks.innerHTML = '';
  const checks = Array.isArray(health.checks) ? health.checks : [];
  if (!checks.length) {
    const empty = document.createElement('div');
    empty.className = 'health-check muted';
    empty.textContent = 'No recent checks';
    healthChecks.appendChild(empty);
    return;
  }

  checks.forEach((check) => {
    const row = document.createElement('div');
    row.className = 'health-check';
    const detail = document.createElement('div');
    detail.className = 'health-check-detail';
    const name = document.createElement('div');
    name.textContent = check.name || 'check';
    const desc = document.createElement('small');
    desc.textContent = check.detail || '';
    detail.append(name, desc);

    const badge = document.createElement('span');
    badge.className = 'status-badge';
    setStatusBadge(badge, check.status || 'ok', check.status || 'ok');

    row.append(detail, badge);
    healthChecks.appendChild(row);
  });
}

function renderVersions(versions) {
  const components = (versions && versions.components) || {};

  if (versionApp) {
    versionApp.textContent = versions?.app || diagVersion?.textContent || '--';
  }
  if (versionIiod) {
    versionIiod.textContent = components['IIOD'] || '--';
  }
  if (versionIiodDescription) {
    versionIiodDescription.textContent = components['IIOD description'] || '--';
  }
  if (versionProtocol) {
    versionProtocol.textContent = components['IIOD protocol'] || components['Protocol'] || '--';
  }
  if (versionFirmware) {
    versionFirmware.textContent = components.Firmware || components['Firmware'] || '--';
  }
}

function renderDiagnostics(diag) {
  if (!diag || !diag.process) {
    if (debugStatus) debugStatus.textContent = 'Diagnostics unavailable';
    return;
  }

  if (debugStatus) debugStatus.textContent = 'Diagnostics live';
  if (diagVersion && diag.version) diagVersion.textContent = diag.version;
  renderVersions(diag.versions);
  if (diag.health) renderHealthStatus(diag.health);

  const process = { ...diag.process, lastSample: diag.process.lastSample || diag.signal?.updatedAt };
  renderProcessMetrics(process);
  renderSignalQuality(diag.signal || {});
  updateDebugPanel(diag.debug);
  renderEventLog(diag.events);
}

function startMetricsStream() {
  if (metricsStream) return;
  metricsStream = new EventSource('/api/diagnostics/metrics');
  metricsStream.onmessage = (event) => {
    try {
      const payload = JSON.parse(event.data);
      if (payload.process) {
        renderProcessMetrics(payload.process);
      }
      if (payload.health) {
        renderHealthStatus(payload.health);
      }
    } catch (err) {
      console.error('metrics stream parse', err);
    }
  };
  metricsStream.onerror = () => {
    stopMetricsStream();
  };
}

function stopMetricsStream() {
  if (metricsStream) {
    metricsStream.close();
    metricsStream = null;
  }
}

function toggleDiagnosticsRefresh(enable) {
  if (enable) {
    if (!diagnosticsTimer) {
      fetchDiagnostics();
      diagnosticsTimer = setInterval(fetchDiagnostics, 5000);
    }
    if (!healthTimer) {
      fetchHealth();
      healthTimer = setInterval(fetchHealth, 10000);
    }
    startMetricsStream();
    return;
  }
  if (diagnosticsTimer) {
    clearInterval(diagnosticsTimer);
    diagnosticsTimer = null;
  }
  if (healthTimer) {
    clearInterval(healthTimer);
    healthTimer = null;
  }
  stopMetricsStream();
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

async function fetchHealth() {
  try {
    const res = await fetch('/api/diagnostics/health');
    if (!res.ok) throw new Error(`status ${res.status}`);
    const payload = await res.json();
    renderHealthStatus(payload);
  } catch (err) {
    console.error('health fetch failed', err);
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

function updateStats(sample, tracks, fromHistory) {
  const primary = (tracks && tracks[0]) || sample;
  recordStatValue(statsState.angle, primary?.angleDeg);
  recordStatValue(statsState.peak, primary?.peak);

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

  if (Array.isArray(tracks)) {
    tracks.forEach((track) => {
      const bucket = perTrackMetrics.get(track.id) || { angle: [], snr: [], confidence: [] };
      recordStatValue(bucket.angle, track.angleDeg);
      recordStatValue(bucket.snr, track.snr);
      recordStatValue(bucket.confidence, track.trackingConfidence);
      perTrackMetrics.set(track.id, bucket);

      const stats = {
        angle: computeStats(bucket.angle),
        snr: computeStats(bucket.snr),
        confidence: computeStats(bucket.confidence),
      };
      if (trackStore.has(track.id)) {
        const entry = trackStore.get(track.id);
        entry.stats = stats;
        trackStore.set(track.id, entry);
      }
    });
    renderTrackStats();
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
  configSnapshot = cfg;
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
  const summaryTrackingMode = document.getElementById('summaryTrackingMode');
  const summaryMaxTracks = document.getElementById('summaryMaxTracks');
  const summaryTimeout = document.getElementById('summaryTimeout');
  const summarySnrThreshold = document.getElementById('summarySnrThreshold');
  if (summaryTrackingMode) {
    summaryTrackingMode.textContent = cfg.trackingMode || 'multi';
  }
  if (summaryMaxTracks) {
    summaryMaxTracks.textContent = formatSamples(cfg.maxTracks);
  }
  if (summaryTimeout) {
    summaryTimeout.textContent = `${cfg.trackTimeoutMs ?? 0} ms`;
  }
  if (summarySnrThreshold) {
    summarySnrThreshold.textContent = Number.isFinite(cfg.snrThreshold)
      ? `${cfg.snrThreshold.toFixed(1)} dB`
      : '--';
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

function ensureAngleDataset(trackId, color) {
  const existing = angleChart.data.datasets.find((ds) => ds.label === trackId);
  if (existing) {
    existing.borderColor = color;
    return existing;
  }
  const ds = { label: trackId, data: [], borderColor: color, tension: 0.2, pointRadius: 0 };
  angleChart.data.datasets.push(ds);
  return ds;
}

function pushAngleSeries(label, tracks) {
  angleChart.data.labels.push(label);
  const trackValues = new Map();
  tracks.forEach((track) => {
    const color = trackStore.get(track.id)?.color || colorForTrack(track.id);
    ensureAngleDataset(track.id, color);
    trackValues.set(track.id, track.angleDeg);
  });

  angleChart.data.datasets.forEach((ds) => {
    const value = trackValues.has(ds.label) ? trackValues.get(ds.label) : null;
    ds.data.push(value);
    if (ds.data.length > MAX_POINTS) {
      ds.data.shift();
    }
  });

  if (angleChart.data.labels.length > MAX_POINTS) {
    angleChart.data.labels.shift();
  }

  angleChart.update('none');
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
document.querySelectorAll('[data-track-sort]').forEach((btn) => {
  btn.addEventListener('click', () => toggleTrackSort(btn.dataset.trackSort));
});
if (trackFilterInput) {
  trackFilterInput.addEventListener('input', renderTracksTable);
}
if (trackStateFilter) {
  trackStateFilter.addEventListener('change', renderTracksTable);
}

updateTraceSummary();
renderTraceRows();

setActiveTab('telemetry');

// Manual QA checklist:
// 1. Load page and confirm Telemetry tab is active with charts and radar visible.
// 2. Switch to Trace/Debug/Settings tabs and verify Telemetry visuals hide while placeholders render.
// 3. Return to Telemetry and confirm data resumes updating without console errors.
// 4. Let more than 100 samples stream in and confirm chart/stat windows cap at 100, scroll smoothly, and Chrome shows no performance warnings.
