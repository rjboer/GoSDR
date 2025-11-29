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
        data.forEach(handleSample);
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

const MAX_POINTS = 100;

const historyBody = document.querySelector('#historyTable tbody');

// Debug panel elements
const debugStatus = document.getElementById('debugStatus');
const debugPhaseDelay = document.getElementById('debugPhaseDelay');
const debugMonopulsePhase = document.getElementById('debugMonopulsePhase');
const debugPeakValue = document.getElementById('debugPeakValue');
const debugPeakBin = document.getElementById('debugPeakBin');
const debugPeakBand = document.getElementById('debugPeakBand');
let debugStreamEnabled = false;

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

function handleSample(sample) {
  if (!telemetryActive) return;
  addSample(sample);
}

function addSample(sample) {
  const timestamp = new Date(sample.timestamp).toLocaleTimeString();
  pushPoint(angleChart, timestamp, sample.angleDeg);
  pushPoint(peakChart, timestamp, sample.peak);

  // Update radar display
  updateRadar(sample.angleDeg);

  updateDebugPanel(sample);

  const row = document.createElement('tr');
  row.innerHTML = `<td>${timestamp}</td><td>${sample.angleDeg.toFixed(2)}</td><td>${sample.peak.toFixed(2)}</td>`;
  historyBody.prepend(row);
  while (historyBody.children.length > MAX_POINTS) {
    historyBody.removeChild(historyBody.lastChild);
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

function pushPoint(chart, label, value) {
  chart.data.labels.push(label);
  chart.data.datasets[0].data.push(value);
  if (chart.data.labels.length > MAX_POINTS) {
    chart.data.labels.shift();
    chart.data.datasets[0].data.shift();
  }
  chart.update('none');
}

setActiveTab('telemetry');

// Manual QA checklist:
// 1. Load page and confirm Telemetry tab is active with charts and radar visible.
// 2. Switch to Trace/Debug/Settings tabs and verify Telemetry visuals hide while placeholders render.
// 3. Return to Telemetry and confirm data resumes updating without console errors.
// 4. Let more than 100 samples stream in and confirm charts/history cap at 100, scroll smoothly, and Chrome shows no performance warnings.
