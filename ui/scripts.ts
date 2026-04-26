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
}

interface NormalizedLatencyEntry {
  average: number;
  protocol: 'IPv4' | 'IPv6';
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

const latencyCache = new Map<string, ReturnType<typeof computeAllLatencyStats>>();

const $ = <T extends HTMLElement>(id: string) =>
  document.getElementById(id) as T;

const ui = {
  dateFilter: $('dateFilter') as HTMLSelectElement,
  protocolFilter: $('protocolFilter') as HTMLSelectElement,
  problemOnlyToggle: $('problemOnlyToggle') as HTMLInputElement,

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
    text: getCSSVar('--chart-text', '#1f2937'),
    grid: getCSSVar('--chart-grid', 'rgba(0,0,0,0.1)')
  };
}

function getChartPalette() {
  return [
    getCSSVar('--chart-c1', '#60a5fa'),
    getCSSVar('--chart-c2', '#34d399'),
    getCSSVar('--chart-c3', '#f59e0b'),
    getCSSVar('--chart-c4', '#f87171'),
    getCSSVar('--chart-c5', '#a78bfa'),
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
      protocol: (e.Protocol ?? e.protocol ?? 'IPv4') as 'IPv4' | 'IPv6'
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

function buildLatencyHistory(data: ParsedDataPoint[], protocol: string) {
  const history = new Map<string, number[]>();

  data.forEach(d => {
    Object.entries(d.latency ?? {}).forEach(([target, entries]) => {
      entries.forEach(e => {
        if (protocol !== 'Both' && e.protocol !== protocol) return;

        if (!history.has(target)) history.set(target, []);
        history.get(target)!.push(e.average);
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
  history: Map<string, number[]>,
  latestPoint: ParsedDataPoint,
  protocol: string
) {
  const values = history.get(target)!;
  const baselineAvg = values.reduce((a, b) => a + b, 0) / values.length;

  const rawLatestEntries = latestPoint.latency?.[target] ?? [];

  const latestEntries = protocol === 'Both'
    ? rawLatestEntries
    : rawLatestEntries.filter(e => e.protocol === protocol);

  const latest = latestEntries.length > 0
    ? latestEntries.reduce((a, b) => a + b.average, 0) / latestEntries.length
    : 0;

  const delta = latest > 0 ? latest - baselineAvg : 0;

  return { baselineAvg, latest, delta, latestEntries };
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
  const history = buildLatencyHistory(data, protocol);
  const latestPoint = findLatestLatencyPoint(data);

  if (!history.size || !latestPoint) return [];

  return Array.from(history.keys()).sort().map(target => {
    const stats = computeLatencyStats(target, history, latestPoint, protocol);
    const status = getLatencyStatus(stats.baselineAvg, stats.latest, stats.delta);

    return { target, ...stats, ...status };
  });
}

function getCachedLatencyStats(data: ParsedDataPoint[], protocol: string) {
  if (!data.length) return [];

  const key = `${data[0].date}_${protocol}`;

  if (latencyCache.has(key)) return latencyCache.get(key)!;

  const result = computeAllLatencyStats(data, protocol);
  latencyCache.set(key, result);

  return result;
}

function filterProblematicOnly(stats: ReturnType<typeof computeAllLatencyStats>, enabled: boolean) {
  if (!enabled) return stats;
  return stats.filter(s => s.cls === 'status-warning' || s.cls === 'status-error');
}

function renderLatencyCard(container: HTMLElement, target: string, stat: any) {
  const el = document.createElement('div');
  el.className = 'flex justify-between p-3 bg-base-200 rounded-lg';

  const isAnimated = stat.cls === 'status-error' || stat.cls === 'status-warning';

  const statusIndicator = isAnimated
    ? `
    <div class="inline-grid *:[grid-area:1/1]">
      <div class="status ${stat.cls} animate-ping"></div>
      <div class="status ${stat.cls}"></div>
    </div>
  `
    : `<span class="status ${stat.cls}"></span>`;

  el.innerHTML = `
    <div class="flex items-center gap-3">
      ${statusIndicator}
      <span>${target}</span>
    </div>
    <div class="text-right">
     <div class="font-bold">
  ${stat.latestEntries?.length
      ? stat.latestEntries
        .map((e: NormalizedLatencyEntry) => `${e.protocol}: ${e.average.toFixed(2)} <span class="text-xs">ms</span>`)
        .join('<br />')
      : `${stat.latest.toFixed(2)} <span class="text-xs">ms</span>`
    }
</div>
      <div class="text-xs opacity-60">
        avg ${stat.baselineAvg.toFixed(2)} ms • ${stat.label}
      </div>
    </div>
  `;

  container.appendChild(el);
}

function updateLatencyCards(
  data: ParsedDataPoint[],
  protocol: string,
  problematicOnly = false
) {
  ui.latencyCards.innerHTML = '';

  const stats = filterProblematicOnly(
    getCachedLatencyStats(data, protocol),
    problematicOnly
  );

  if (!stats.length) {
    ui.latencyCards.textContent = problematicOnly
      ? 'No latency issues detected 🎉'
      : 'No latency data.';
    return;
  }

  stats.forEach(stat => {
    renderLatencyCard(ui.latencyCards, stat.target, stat);
  });
}

function updateSpeedCard(data: ParsedDataPoint[]) {
  const latest = [...data].reverse().find(d => d.speedtest);

  if (!latest?.speedtest) {
    ui.speedCard.classList.add('hidden');
    ui.speedSection.classList.add('hidden');
    return;
  }

  ui.speedCard.classList.remove('hidden');

  ui.latestDownload.textContent = `${latest.speedtest.download.toFixed(0)} Mbps`;
  ui.latestUpload.textContent = `${latest.speedtest.upload.toFixed(0)} Mbps`;
  ui.speedTime.textContent = `At ${latest.dt.toFormat('HH:mm:ss')}`;
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
          backgroundColor: withOpacity(color, 0.2),
          tension: 0.25,
          spanGaps: true
        };
      })
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { grid: { color: grid } },
        y: { grid: { color: grid } }
      },
      interaction: {
        mode: 'index',
        intersect: false
      }
    }
  });

  const speed = data.filter(d => d.speedtest);
  const hasSpeed = speed.length > 0;

  ui.speedSection.classList.toggle('hidden', !hasSpeed);

  if (!hasSpeed) {
    charts.speedtest = null;
    return;
  }

  const speedCtx = $('speedtestChart') as HTMLCanvasElement;

  charts.speedtest = new Chart(speedCtx, {
    type: 'bar',
    data: {
      labels: speed.map(d => d.dt.toFormat('HH:mm:ss')),
      datasets: [
        {
          label: 'Download',
          data: speed.map(d => d.speedtest!.download),
          backgroundColor: getCSSVar('--chart-c1')
        },
        {
          label: 'Upload',
          data: speed.map(d => d.speedtest!.upload),
          backgroundColor: getCSSVar('--chart-c2')
        }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { grid: { color: grid } },
        y: { grid: { color: grid } }
      },
      interaction: {
        mode: 'index',
        intersect: false
      }
    }
  });
}

function populateFilters() {
  ui.dateFilter.innerHTML = localDates
    .map(d => `<option value="${d}">${d}</option>`)
    .join('');

  if (!ui.dateFilter.value && localDates.length) {
    ui.dateFilter.value = localDates[0];
  }
}

function applyFilters() {
  const data = getFilteredData();

  if (!data.length) return setView('empty');

  setView('content');

  updateSpeedCard(data);

  updateLatencyCards(
    data,
    ui.protocolFilter.value,
    ui.problemOnlyToggle?.checked
  );

  renderCharts(data, ui.protocolFilter.value);
}

function initFilters() {
  ui.dateFilter.addEventListener('change', applyFilters);
  ui.protocolFilter.addEventListener('change', applyFilters);
  ui.problemOnlyToggle?.addEventListener('change', applyFilters);
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

// --- Init ---
initFilters();
load();