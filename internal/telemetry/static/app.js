const angleChart = new Chart(document.getElementById('angleChart'), {
  type: 'line',
  data: { labels: [], datasets: [{ label: 'Angle (deg)', data: [], borderColor: '#2f80ed', tension: 0.2 }] },
  options: { scales: { x: { display: false }, y: { title: { display: true, text: 'Degrees' } } }, animation: false }
});

const historyBody = document.querySelector('#historyTable tbody');

function addSample(sample) {
  const timestamp = new Date(sample.timestamp).toLocaleTimeString();
  angleChart.data.labels.push(timestamp);
  angleChart.data.datasets[0].data.push(sample.angleDeg);
  if (angleChart.data.labels.length > 200) {
    angleChart.data.labels.shift();
    angleChart.data.datasets[0].data.shift();
  }
  angleChart.update();

  const row = document.createElement('tr');
  row.innerHTML = `<td>${timestamp}</td><td>${sample.angleDeg.toFixed(2)}</td><td>${sample.peak.toFixed(2)}</td>`;
  historyBody.prepend(row);
  while (historyBody.children.length > 100) {
    historyBody.removeChild(historyBody.lastChild);
  }
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
