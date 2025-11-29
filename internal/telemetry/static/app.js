// Tab management
const tabButtons = document.querySelectorAll('.tab-button');
const tabPanels = document.querySelectorAll('.tab-panel');

let telemetryActive = true;
let telemetryStream = null;
let historyLoaded = false;

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
}

tabButtons.forEach((btn) => {
  btn.addEventListener('click', () => setActiveTab(btn.dataset.tab));
});

function setTelemetryActive(isActive) {
  telemetryActive = isActive;
  if (telemetryActive) {
    startTelemetry();
  } else {
    stopTelemetry();
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
const rangeRings = [10, 20, 30];

// Current detection
let currentAngle = 90; // Start at center (0° in tracker coordinates)
let currentRange = 20; // Fixed at middle ring

function drawRadar() {
  // Clear canvas
  radarCtx.fillStyle = '#000000';
  radarCtx.fillRect(0, 0, radarCanvas.width, radarCanvas.height);

  // Draw range rings
  radarCtx.strokeStyle = '#00ff00';
  radarCtx.lineWidth = 1;

  rangeRings.forEach((range, index) => {
    const radius = radarMaxRadius * (index + 1) / rangeRings.length;
    radarCtx.beginPath();
    radarCtx.arc(radarCenterX, radarCenterY, radius, Math.PI, 0, false);
    radarCtx.stroke();

    // Range labels
    radarCtx.fillStyle = '#00ff00';
    radarCtx.font = '10px monospace';
    radarCtx.fillText(`${range}cm`, radarCenterX + radius + 5, radarCenterY - 5);
  });

  // Draw static angle lines (every 10 degrees)
  radarCtx.strokeStyle = '#00ff00';
  radarCtx.lineWidth = 0.5;

  for (let angle = 0; angle <= 180; angle += 10) {
    const rad = (angle - 90) * Math.PI / 180;
    const x = radarCenterX + radarMaxRadius * Math.cos(rad);
    const y = radarCenterY + radarMaxRadius * Math.sin(rad);

    radarCtx.beginPath();
    radarCtx.moveTo(radarCenterX, radarCenterY);
    radarCtx.lineTo(x, y);
    radarCtx.stroke();

    // Angle labels at 0°, 45°, 90°, 135°, 180°
    if (angle % 45 === 0) {
      radarCtx.fillStyle = '#00ff00';
      radarCtx.font = '12px monospace';
      const labelX = radarCenterX + (radarMaxRadius + 15) * Math.cos(rad);
      const labelY = radarCenterY + (radarMaxRadius + 15) * Math.sin(rad);
      radarCtx.fillText(`${angle}°`, labelX - 10, labelY + 5);
    }
  }

  // Draw detection marker (red dot)
  const detectionRad = (currentAngle - 90) * Math.PI / 180;
  const detectionRadius = radarMaxRadius * (currentRange / rangeRings[rangeRings.length - 1]);
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
  // Convert angle from [-90, 90] to [0, 180] for radar display
  currentAngle = angleDeg + 90;

  // Clamp to valid range
  if (currentAngle < 0) currentAngle = 0;
  if (currentAngle > 180) currentAngle = 180;

  drawRadar();
  radarAngleDisplay.textContent = `Angle: ${angleDeg.toFixed(1)}°`;
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

// Debug panel elements
const debugStatus = document.getElementById('debugStatus');
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

function updateLockBadge(state) {
  if (!lockBadge) return;
  const normalized = (state || 'searching').toLowerCase();
  lockBadge.textContent = normalized;
  lockBadge.classList.remove('locked', 'tracking', 'searching');
  if (normalized === 'locked') {
    lockBadge.classList.add('locked');
  } else if (normalized === 'tracking') {
    lockBadge.classList.add('tracking');
  } else {
    lockBadge.classList.add('searching');
  }
}

function updateDebugPanel(sample) {
  if (!debugStatus) return;
  const info = sample.debug;
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

setActiveTab('telemetry');

// Manual QA checklist:
// 1. Load page and confirm Telemetry tab is active with charts and radar visible.
// 2. Switch to Trace/Debug/Settings tabs and verify Telemetry visuals hide while placeholders render.
// 3. Return to Telemetry and confirm data resumes updating without console errors.
// 4. Let more than 100 samples stream in and confirm chart/stat windows cap at 100, scroll smoothly, and Chrome shows no performance warnings.
