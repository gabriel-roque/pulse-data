const form = document.querySelector('#query-form');
const apiKeyInput = document.querySelector('#api-key');
const fromInput = document.querySelector('#from');
const toInput = document.querySelector('#to');
const typeInput = document.querySelector('#event-type');
const submitButton = form.querySelector('button[type="submit"]');
const submitLabel = document.querySelector('#submit-label');
const status = document.querySelector('#system-status');
const statusLabel = document.querySelector('#status-label');
const errorMessage = document.querySelector('#error-message');
const chart = document.querySelector('#chart');
const resultsBody = document.querySelector('#results-body');
const totalEvents = document.querySelector('#total-events');
const typeCount = document.querySelector('#type-count');
const rangeLabel = document.querySelector('#range-label');
const resultCount = document.querySelector('#result-count');
const numberFormat = new Intl.NumberFormat('en-US');

function toInputValue(date) {
  const offset = date.getTimezoneOffset() * 60000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}

function setRange(hours) {
  const to = new Date();
  const from = new Date(to.getTime() - hours * 60 * 60 * 1000);
  fromInput.value = toInputValue(from);
  toInput.value = toInputValue(to);
}

function toRFC3339(value) {
  return new Date(value).toISOString();
}

function setStatus(state, label) {
  status.dataset.state = state;
  statusLabel.textContent = label;
}

function setError(message) {
  errorMessage.textContent = message;
  errorMessage.hidden = !message;
}

function escapeHTML(value) {
  return String(value).replace(/[&<>'"]/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'
  }[character]));
}

function renderResults(rows) {
  const sortedRows = [...rows].sort((left, right) => Number(right.count) - Number(left.count));
  const total = sortedRows.reduce((sum, row) => sum + Number(row.count || 0), 0);
  const maximum = Math.max(...sortedRows.map((row) => Number(row.count || 0)), 1);

  totalEvents.textContent = numberFormat.format(total);
  typeCount.textContent = numberFormat.format(sortedRows.length);
  resultCount.textContent = `${sortedRows.length} ${sortedRows.length === 1 ? 'row' : 'rows'}`;

  if (sortedRows.length === 0) {
    chart.className = 'bar-chart empty-state';
    chart.innerHTML = '<div class="empty-icon" aria-hidden="true">0</div><p>No events found for this range.</p>';
    resultsBody.innerHTML = '<tr class="table-empty"><td colspan="3">No events found for this range.</td></tr>';
    return;
  }

  chart.className = 'bar-chart';
  chart.innerHTML = sortedRows.map((row) => {
    const count = Number(row.count || 0);
    const width = Math.max((count / maximum) * 100, 2);
    return `<div class="bar-row">
      <span class="bar-label" title="${escapeHTML(row.type)}">${escapeHTML(row.type)}</span>
      <span class="bar-track"><span class="bar-fill" style="width:${width}%"></span></span>
      <span class="bar-value">${numberFormat.format(count)}</span>
    </div>`;
  }).join('');

  resultsBody.innerHTML = sortedRows.map((row) => {
    const count = Number(row.count || 0);
    const share = total ? `${((count / total) * 100).toFixed(1)}%` : '0%';
    return `<tr>
      <td>${escapeHTML(row.type)}</td>
      <td>${numberFormat.format(count)}</td>
      <td class="align-right">${share}</td>
    </tr>`;
  }).join('');
}

async function runQuery(event) {
  event.preventDefault();
  setError('');
  submitButton.disabled = true;
  submitLabel.textContent = 'Querying...';
  setStatus('loading', 'Querying ClickHouse');

  const params = new URLSearchParams({
    from: toRFC3339(fromInput.value),
    to: toRFC3339(toInput.value)
  });
  if (typeInput.value.trim()) params.set('type', typeInput.value.trim());

  try {
    const response = await fetch(`/api/v1/analytics/summary?${params}`, {
      headers: { Authorization: `Bearer ${apiKeyInput.value.trim()}` }
    });
    const body = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(body.error || `Query failed with HTTP ${response.status}`);
    renderResults(Array.isArray(body) ? body : []);
    const from = new Date(fromInput.value);
    const to = new Date(toInput.value);
    rangeLabel.textContent = `${from.toLocaleDateString()} - ${to.toLocaleDateString()}`;
    setStatus('success', 'Query complete');
  } catch (error) {
    setError(error.message || 'Could not query analytics.');
    setStatus('error', 'Query failed');
  } finally {
    submitButton.disabled = false;
    submitLabel.textContent = 'Run query';
  }
}

document.querySelectorAll('.preset').forEach((button) => {
  button.addEventListener('click', () => setRange(Number(button.dataset.hours)));
});

form.addEventListener('submit', runQuery);
setRange(24);
