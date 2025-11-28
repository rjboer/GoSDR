function $(id) {
  return document.getElementById(id);
}

const form = $('configForm');
const statusEl = $('status');

function setStatus(message, isError = false) {
  statusEl.textContent = message;
  statusEl.className = isError ? 'status error' : 'status';
}

async function loadConfig() {
  try {
    const res = await fetch('/api/config');
    if (!res.ok) {
      throw new Error(`Failed to load config: ${res.status}`);
    }
    const cfg = await res.json();
    $('sampleRateHz').value = cfg.sampleRateHz;
    $('bufferSize').value = cfg.bufferSize;
    $('historyLimit').value = cfg.historyLimit;
  } catch (err) {
    console.error(err);
    setStatus(err.message, true);
  }
}

form.addEventListener('submit', async (evt) => {
  evt.preventDefault();
  const payload = {
    sampleRateHz: parseInt($('sampleRateHz').value, 10),
    bufferSize: parseInt($('bufferSize').value, 10),
    historyLimit: parseInt($('historyLimit').value, 10),
  };

  try {
    setStatus('Saving...');
    const res = await fetch('/api/config/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const errText = await res.text();
      throw new Error(errText || `Failed with status ${res.status}`);
    }
    await res.json();
    setStatus('Configuration updated');
  } catch (err) {
    console.error(err);
    setStatus(err.message, true);
  }
});

loadConfig();
