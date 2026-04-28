import { Chart } from 'chart.js/auto';
import { DateTime } from 'luxon';

const LATENCY_CONFIG = {
  thresholds: {
    high: { abs: 15, relative: 0.5 },
    elevated: { abs: 5, relative: 0.25 }
  }
};

interface SpeedtestEntry { download: number; upload: number; }

interface RawLatencyEntry {
  Average?: number;
  average?: number;
  Protocol?: string;
  protocol?: string;
  PacketLoss?: number;
  packetLoss?: number;
}

interface NormalizedLatencyEntry {
  average: number;
  protocol: 'IPv4' | 'IPv6';
  packetLoss: number;
}

type LatencyTarget = Record<string, NormalizedLatencyEntry[]>;

interface ParsedDataPoint {
  dt: DateTime;
  date: string;
  speedtest?: SpeedtestEntry;
  latency?: LatencyTarget;
}

let rawData: ParsedDataPoint[] = [];
let localDates: string[] = [];
let charts = {
  latency: null as Chart | null,
  speedtest: null as Chart | null
};

const latencyCache = new Map<string, any[]>();

const $ = <T extends HTMLElement>(id: string) =>
  document.getElementById(id) as T;

const ui = {
  dateFilter: $('dateFilter') as HTMLSelectElement,
  protocolFilter: $('protocolFilter') as HTMLSelectElement,

  status: $('statusContainer'),
  error: $('errorAlert'),
  empty: $('emptyAlert'),
  loading: $('loadingSkeleton'),
  main: $('mainContent'),

  speedCard: $('speedtestCard'),
  speedSection: $('speedtestChartSection'),
  latencyCards: $('latencyCardsContainer'),

  latestDownload: $('latestDownload'),
  latestUpload: $('latestUpload'),
  speedTime: $('speedtestTime')
};

// --- CSS Helpers ---
function getCSSVar(name: string, fallback = '') {
  return getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim() || fallback;
}

function getThemeColors() {
  return {
    text: getCSSVar('--chart-text', '#ffffff'),
    grid: getCSSVar('--chart-grid', 'rgba(255,255,255,0.1)')
  };
}

function getChartPalette() {
  return [
    getCSSVar('--chart-c1', '#00d2ff'),
    getCSSVar('--chart-c2', '#39ff14'),
    getCSSVar('--chart-c3', '#ff9900'),
    getCSSVar('--chart-c4', '#ff4d4d'),
    getCSSVar('--chart-c5', '#a349eb'),
    getCSSVar('--chart-c6', '#22d3ee')
  ];
}

function withOpacity(color: string, opacity = 0.2) {
  if (color.startsWith('rgb')) {
    return color.replace('rgb', 'rgba').replace(')', `, ${opacity})`);
  }
  return color;
}

function setView(state: 'loading' | 'error' | 'empty' | 'content') {
  ui.status.classList.toggle('hidden', state === 'content');
  ui.loading.classList.toggle('hidden', state !== 'loading');
  ui.error.classList.toggle('hidden', state !== 'error');
  ui.empty.classList.toggle('hidden', state !== 'empty');
  ui.main.classList.toggle('hidden', state !== 'content');
}

function normalizeLatency(latency?: Record<string, RawLatencyEntry[]>): LatencyTarget | undefined {
  if (!latency) return;

  const result: LatencyTarget = {};

  for (const [target, entries] of Object.entries(latency)) {
    result[target] = entries.map(e => ({
      average: e.Average ?? e.average ?? 0,
      protocol: (e.Protocol ?? e.protocol ?? 'IPv4') as 'IPv4' | 'IPv6',
      packetLoss: e.PacketLoss ?? e.packetLoss ?? 0
    }));
  }

  return result;
}

function parseData(json: any) {
  const dates = new Set<string>();

  rawData = Object.entries(json).flatMap(([date, times]: any) =>
    Object.entries(times).map(([time, entry]: any) => {
      const dt = DateTime.fromISO(`${date}T${time}`, { zone: 'utc' }).toLocal();
      if (!dt.isValid) return null;

      const localDate = dt.toFormat('yyyy-MM-dd');
      dates.add(localDate);

      return {
        dt,
        date: localDate,
        speedtest: entry.speedtest?.[0],
        latency: normalizeLatency(entry.latency)
      } as ParsedDataPoint;
    })
  ).filter(Boolean) as ParsedDataPoint[];

  rawData.sort((a, b) => a.dt.toMillis() - b.dt.toMillis());
  localDates = [...dates].sort().reverse();
}

function getFilteredData() {
  const date = ui.dateFilter.value;
  const protocol = ui.protocolFilter.value;

  return rawData.filter(d =>
    d.date === date &&
    (!d.latency ||
      protocol === 'Both' ||
      Object.values(d.latency).some(entries =>
        entries.some(e => e.protocol === protocol)
      ))
  );
}

function buildLatencyHistory(data: ParsedDataPoint[]) {
  const history = new Map<string, Map<string, number[]>>();

  data.forEach(d => {
    Object.entries(d.latency ?? {}).forEach(([target, entries]) => {
      if (!history.has(target)) history.set(target, new Map());
      const targetHistory = history.get(target)!;

      entries.forEach(e => {
        if (!targetHistory.has(e.protocol)) targetHistory.set(e.protocol, []);
        targetHistory.get(e.protocol)!.push(e.average);
      });
    });
  });

  return history;
}

function findLatestLatencyPoint(data: ParsedDataPoint[]) {
  return [...data].reverse().find(d => d.latency);
}

function computeLatencyStats(
  target: string,
  history: Map<string, Map<string, number[]>>,
  latestPoint: ParsedDataPoint,
  protocolFilter: string
) {
  const targetHistory = history.get(target)!;
  const rawLatestEntries = latestPoint.latency?.[target] ?? [];

  const latestEntries = (protocolFilter === 'Both'
    ? rawLatestEntries
    : rawLatestEntries.filter(e => e.protocol === protocolFilter))
    .map(e => {
      const protoValues = targetHistory.get(e.protocol) || [e.average];
      const protoBaseline = protoValues.reduce((a, b) => a + b, 0) / protoValues.length;
      
      const d = e.average - protoBaseline;
      const s = getLatencyStatus(protoBaseline, e.average, d);
      return { ...e, ...s, baseline: protoBaseline };
    });

  const latest = latestEntries.length > 0
    ? latestEntries.reduce((a, b) => a + b.average, 0) / latestEntries.length
    : 0;

  let overallStatus = { cls: 'status-success', label: 'Normal' };
  latestEntries.forEach(e => {
    if (e.cls === 'status-error') overallStatus = { cls: 'status-error', label: 'High' };
    else if (e.cls === 'status-warning' && overallStatus.cls !== 'status-error') {
      overallStatus = { cls: 'status-warning', label: 'Elevated' };
    }
  });

  return { latest, latestEntries, ...overallStatus };
}

function getLatencyStatus(baselineAvg: number, latest: number, delta: number) {
  if (latest === 0) return { cls: 'status-ghost', label: 'No Data' };

  const { high, elevated } = LATENCY_CONFIG.thresholds;

  if (delta > Math.max(high.abs, baselineAvg * high.relative)) {
    return { cls: 'status-error', label: 'High' };
  }

  if (delta > Math.max(elevated.abs, baselineAvg * elevated.relative)) {
    return { cls: 'status-warning', label: 'Elevated' };
  }

  return { cls: 'status-success', label: 'Normal' };
}

function computeAllLatencyStats(data: ParsedDataPoint[], protocol: string) {
  const history = buildLatencyHistory(data);
  const latestPoint = findLatestLatencyPoint(data);

  if (!history.size || !latestPoint) return [];

  return Array.from(history.keys())
    .sort()
    .map(target => {
      return { target, ...computeLatencyStats(target, history, latestPoint, protocol) };
    })
    .filter(stat => stat.latestEntries.length > 0);
}

function getCachedLatencyStats(data: ParsedDataPoint[], protocol: string) {
  if (!data.length) return [];

  const key = `${data[0].date}_${protocol}`;

  if (latencyCache.has(key)) return latencyCache.get(key)!;

  const result = computeAllLatencyStats(data, protocol);
  latencyCache.set(key, result);

  return result;
}

function renderLatencyCard(container: HTMLElement, target: string, stat: any) {
  const el = document.createElement('div');
  el.className = 'instrument-box';

  const entries = stat.latestEntries || [];
  
  const protocolTags = entries
    .map((e: any) => `<span class="protocol-tag">${e.protocol}</span>`)
    .join('');

  const lossValue = entries.reduce((acc: number, e: any) => acc + e.packetLoss, 0) || 0;
  const lossTag = lossValue > 0 ? `<span class="loss-tag">${lossValue.toFixed(1)}% LOSS</span>` : '';

  let valuesHtml = '';
  if (entries.length > 1) {
    valuesHtml = entries.map((e: any) => `
      <div style="font-size: 1.2rem; display: flex; justify-content: space-between; align-items: baseline; width: 100%;">
        <span style="font-size: 0.7rem; color: var(--text-muted);">${e.protocol}</span>
        <span class="${e.cls}">${e.average.toFixed(2)}<span class="instrument-unit">ms</span></span>
      </div>
    `).join('');
  } else {
    valuesHtml = `
      <div class="instrument-value ${stat.cls}">
        ${stat.latest.toFixed(2)} <span class="instrument-unit">ms</span>
      </div>
    `;
  }

  el.innerHTML = `
    <div class="instrument-label">
      <span>${target}</span>
      <div style="display: flex; gap: 4px; align-items: center;">
        ${lossTag}
        ${protocolTags}
      </div>
    </div>
    <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; flex: 1;">
      ${valuesHtml}
    </div>
    <div class="instrument-footer">CURRENT LATENCY</div>
  `;

  container.appendChild(el);
}

function updateLatencyCards(data: ParsedDataPoint[], protocol: string) {
  const speedCard = ui.speedCard;
  ui.latencyCards.innerHTML = '';
  if (speedCard) ui.latencyCards.appendChild(speedCard);

  const stats = getCachedLatencyStats(data, protocol);

  if (!stats.length && (!speedCard || speedCard.classList.contains('hidden'))) {
    const msg = document.createElement('div');
    msg.style.gridColumn = '1 / -1';
    msg.textContent = 'No latency data.';
    ui.latencyCards.appendChild(msg);
  }

  stats.forEach(stat => {
    renderLatencyCard(ui.latencyCards, stat.target, stat);
  });
}

function updateSpeedCard(data: ParsedDataPoint[]) {
  const latest = [...data].reverse().find(d => d.speedtest);

  if (!latest?.speedtest) {
    ui.speedCard?.classList.add('hidden');
    ui.speedSection?.classList.add('hidden');
    return;
  }

  ui.speedCard?.classList.remove('hidden');
  ui.speedSection?.classList.remove('hidden');

  ui.latestDownload.textContent = latest.speedtest.download.toFixed(0);
  ui.latestUpload.textContent = latest.speedtest.upload.toFixed(0);
  ui.speedTime.textContent = `LAST TEST AT ${latest.dt.toFormat('HH:mm:ss')}`;
}

function destroyCharts() {
  Object.values(charts).forEach(c => c?.destroy());
  charts.latency = null;
  charts.speedtest = null;
}

function renderCharts(data: ParsedDataPoint[], protocol: string) {
  destroyCharts();

  const { text, grid } = getThemeColors();
  const palette = getChartPalette();

  Chart.defaults.color = text;
  Chart.defaults.borderColor = grid;
  Chart.defaults.font.family = 'Inter';

  const latencyCtx = $('latencyChart') as HTMLCanvasElement;
  const labels = data.map(d => d.dt.toFormat('HH:mm:ss'));
  const targetMap: Record<string, (number | null)[]> = {};

  data.forEach((d, i) => {
    Object.entries(d.latency ?? {}).forEach(([target, entries]) => {
      entries.forEach(e => {
        if (protocol !== 'Both' && e.protocol !== protocol) return;
        const key = protocol === 'Both' ? `${target} (${e.protocol})` : target;
        if (!targetMap[key]) targetMap[key] = Array(data.length).fill(null);
        targetMap[key][i] = e.average;
      });
    });
  });

  charts.latency = new Chart(latencyCtx, {
    type: 'line',
    data: {
      labels,
      datasets: Object.entries(targetMap).map(([k, v], i) => {
        const color = palette[i % palette.length];
        return {
          label: k,
          data: v,
          borderColor: color,
          backgroundColor: withOpacity(color, 0.1),
          borderWidth: 2,
          pointRadius: 2,
          tension: 0.3,
          spanGaps: true
        };
      })
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { position: 'top', align: 'start', labels: { boxWidth: 10, font: { size: 10 } } }
      },
      scales: {
        x: { grid: { color: grid }, ticks: { font: { size: 9 } } },
        y: { grid: { color: grid }, ticks: { font: { size: 10 } } }
      },
      interaction: { mode: 'index', intersect: false }
    }
  });

  const speed = data.filter(d => d.speedtest);
  if (speed.length > 0) {
    ui.speedSection?.classList.remove('hidden');
    const speedCtx = $('speedtestChart') as HTMLCanvasElement;
    charts.speedtest = new Chart(speedCtx, {
      type: 'line',
      data: {
        labels: speed.map(d => d.dt.toFormat('HH:mm:ss')),
        datasets: [
          {
            label: 'Download',
            data: speed.map(d => d.speedtest!.download),
            borderColor: getCSSVar('--neon-blue'),
            backgroundColor: withOpacity(getCSSVar('--neon-blue'), 0.1),
            borderWidth: 3,
            tension: 0.4,
            fill: true
          },
          {
            label: 'Upload',
            data: speed.map(d => d.speedtest!.upload),
            borderColor: getCSSVar('--neon-orange'),
            backgroundColor: withOpacity(getCSSVar('--neon-orange'), 0.1),
            borderWidth: 3,
            tension: 0.4,
            fill: true
          }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { position: 'top', align: 'start' } },
        scales: {
          x: { grid: { color: grid }, ticks: { font: { size: 9 } } },
          y: { grid: { color: grid }, ticks: { font: { size: 10 } } }
        },
        interaction: { mode: 'index', intersect: false }
      }
    });
  } else {
    ui.speedSection?.classList.add('hidden');
  }
}

function populateFilters() {
  ui.dateFilter.innerHTML = localDates
    .map(d => `<option value="${d}">${d}</option>`)
    .join('');
  if (!ui.dateFilter.value && localDates.length) ui.dateFilter.value = localDates[0];
}

function applyFilters() {
  const data = getFilteredData();
  if (!data.length) return setView('empty');
  setView('content');
  updateSpeedCard(data);
  updateLatencyCards(data, ui.protocolFilter.value);
  renderCharts(data, ui.protocolFilter.value);
}

function initFilters() {
  ui.dateFilter.addEventListener('change', applyFilters);
  ui.protocolFilter.addEventListener('change', applyFilters);
}

async function load() {
  setView('loading');
  try {
    const res = await fetch('metrics.json');
    if (!res.ok) throw new Error();
    const json = await res.json();
    parseData(json);
    populateFilters();
    applyFilters();
  } catch {
    setView('error');
  }
}

initFilters();
load();