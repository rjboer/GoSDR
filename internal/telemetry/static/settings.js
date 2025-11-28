function $(id) {
  return document.getElementById(id);
}

const fieldIds = [
  'sampleRateHz',
  'rxLoHz',
  'toneOffsetHz',
  'spacingWavelength',
  'numSamples',
  'bufferSize',
  'historyLimit',
  'trackingLength',
  'phaseStepDeg',
  'scanStepDeg',
  'phaseCalDeg',
  'phaseDeltaDeg',
  'warmupBuffers',
  'rxGain0',
  'rxGain1',
  'txGain',
  'sdrBackend',
  'sdrUri',
];

const numericFields = new Set([
  'sampleRateHz',
  'rxLoHz',
  'toneOffsetHz',
  'spacingWavelength',
  'numSamples',
  'bufferSize',
  'historyLimit',
  'trackingLength',
  'phaseStepDeg',
  'scanStepDeg',
  'phaseCalDeg',
  'phaseDeltaDeg',
  'warmupBuffers',
  'rxGain0',
  'rxGain1',
  'txGain',
]);

const defaults = {
  sampleRateHz: 2000000,
  rxLoHz: 2300000000,
  toneOffsetHz: 200000,
  spacingWavelength: 0.5,
  numSamples: 512,
  bufferSize: 4096,
  historyLimit: 500,
  trackingLength: 50,
  phaseStepDeg: 1,
  scanStepDeg: 2,
  phaseCalDeg: 0,
  phaseDeltaDeg: 35,
  warmupBuffers: 3,
  rxGain0: 0,
  rxGain1: 0,
  txGain: -10,
  sdrBackend: 'mock',
  sdrUri: 'ip:192.168.2.1',
};

const statusEl = $('status');
const form = $('configForm');
const resetBtn = $('resetBtn');

function setStatus(message, isError = false) {
  statusEl.textContent = message;
  statusEl.className = isError ? 'status error' : 'status';
}

function applyConfig(cfg) {
  const merged = { ...defaults, ...cfg };
  fieldIds.forEach((id) => {
    if (merged[id] === undefined || merged[id] === null) return;
    const el = $(id);
    if (!el) return;
    el.value = merged[id];
  });
}

async function loadConfig() {
  try {
    setStatus('Loading configuration…');
    const res = await fetch('/api/config');
    if (!res.ok) {
      throw new Error(`Failed to load config: ${res.status}`);
    }
    const cfg = await res.json();
    applyConfig(cfg);
    setStatus('');
  } catch (err) {
    console.error(err);
    setStatus(err.message, true);
    applyConfig(defaults);
  }
}

function collectPayload() {
  const payload = {};
  fieldIds.forEach((id) => {
    const el = $(id);
    if (!el) return;
    const value = el.value;
    payload[id] = numericFields.has(id) ? Number(value) : value;
  });
  return payload;
}

form.addEventListener('submit', async (evt) => {
  evt.preventDefault();
  const payload = collectPayload();
  try {
    setStatus('Saving…');
    const res = await fetch('/api/config/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const errText = await res.text();
      throw new Error(errText || `Failed with status ${res.status}`);
    }
    const saved = await res.json();
    applyConfig(saved);
    setStatus('Configuration updated');
  } catch (err) {
    console.error(err);
    setStatus(err.message, true);
  }
});

resetBtn.addEventListener('click', () => {
  applyConfig(defaults);
  setStatus('Restored defaults (not yet saved)');
});

loadConfig();
