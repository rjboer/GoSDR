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
  'mockPhaseDelta',
  'warmupBuffers',
  'rxGain0',
  'rxGain1',
  'txGain',
  'sdrBackend',
  'sdrUri',
  'logLevel',
  'logFormat',
  'debugMode',
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
  'mockPhaseDelta',
  'warmupBuffers',
  'rxGain0',
  'rxGain1',
  'txGain',
]);

const booleanFields = new Set(['debugMode']);

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
  mockPhaseDelta: 30,
  warmupBuffers: 3,
  rxGain0: 0,
  rxGain1: 0,
  txGain: -10,
  sdrBackend: 'mock',
  sdrUri: 'ip:192.168.2.1',
  logLevel: 'warn',
  logFormat: 'text',
  debugMode: false,
};

const statusEl = $('status');
const form = $('configForm');
const resetBtn = $('resetBtn');
const backendField = $('sdrBackend');
const restartBanner = $('restartBanner');

let lastSavedConfig = null;

function setStatus(message, type = 'info') {
  if (!statusEl) return;
  statusEl.textContent = message;
  const classNames = ['status'];
  if (type === 'error') classNames.push('error');
  if (type === 'success') classNames.push('success');
  if (!message) classNames.push('hidden');
  statusEl.className = classNames.join(' ');
}

function showRestartBanner(visible) {
  if (!restartBanner) return;
  restartBanner.classList.toggle('hidden', !visible);
}

function updateBackendState() {
  const backend = backendField?.value;
  const isMock = backend === 'mock';
  const sdrUriInput = $('sdrUri');
  const mockPhaseInput = $('mockPhaseDelta');

  if (sdrUriInput) {
    sdrUriInput.disabled = isMock;
    if (isMock) {
      sdrUriInput.value = '';
    }
  }
  if (mockPhaseInput) {
    mockPhaseInput.disabled = !isMock;
  }
}

function applyConfig(cfg) {
  const merged = { ...defaults, ...cfg };
  fieldIds.forEach((id) => {
    if (merged[id] === undefined || merged[id] === null) return;
    const el = $(id);
    if (!el) return;
    if (el.type === 'checkbox' || booleanFields.has(id)) {
      el.checked = Boolean(merged[id]);
    } else {
      el.value = merged[id];
    }
  });
  updateBackendState();
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
    lastSavedConfig = cfg;
    showRestartBanner(false);
    setStatus('');
  } catch (err) {
    console.error(err);
    setStatus(err.message, 'error');
    applyConfig(defaults);
  }
}

function collectPayload() {
  const payload = {};
  fieldIds.forEach((id) => {
    const el = $(id);
    if (!el) return;
    if (el.type === 'checkbox' || booleanFields.has(id)) {
      payload[id] = el.checked;
      return;
    }
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
      let message = errText || `Failed with status ${res.status}`;
      try {
        const parsed = JSON.parse(errText);
        if (parsed?.error) message = parsed.error;
      } catch (_) {}
      throw new Error(message);
    }
    const saved = await res.json();
    applyConfig(saved);
    const backendChanged =
      lastSavedConfig &&
      (lastSavedConfig.sdrBackend !== saved.sdrBackend || lastSavedConfig.sdrUri !== saved.sdrUri);
    lastSavedConfig = saved;
    setStatus('Configuration updated', 'success');
    showRestartBanner(Boolean(backendChanged));
  } catch (err) {
    console.error(err);
    setStatus(err.message, 'error');
  }
});

resetBtn.addEventListener('click', () => {
  applyConfig(defaults);
  setStatus('Restored defaults (not yet saved)');
});

backendField?.addEventListener('change', updateBackendState);

loadConfig();
