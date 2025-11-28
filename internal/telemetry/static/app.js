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

const MAX_POINTS = 500;

const historyBody = document.querySelector('#historyTable tbody');

function addSample(sample) {
  const timestamp = new Date(sample.timestamp).toLocaleTimeString();
  pushPoint(angleChart, timestamp, sample.angleDeg);
  pushPoint(peakChart, timestamp, sample.peak);

  const row = document.createElement('tr');
  row.innerHTML = `<td>${timestamp}</td><td>${sample.angleDeg.toFixed(2)}</td><td>${sample.peak.toFixed(2)}</td>`;
  historyBody.prepend(row);
  while (historyBody.children.length > 100) {
    historyBody.removeChild(historyBody.lastChild);
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

fetch('/api/history')
  .then((res) => res.json())
  .then((data) => data.forEach(addSample))
  .catch((err) => console.error('history', err));

const source = new EventSource('/api/live');
source.onmessage = (event) => {
  try {
    const sample = JSON.parse(event.data);
    addSample(sample);
  } catch (err) {
    console.error('parse sample', err);
  }
};
source.onerror = (err) => console.error('sse error', err);
