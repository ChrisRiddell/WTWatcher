import { Chart, ChartDataset } from 'chart.js/auto';
import { DateTime } from 'luxon';

// --- Types & Interfaces ---

const LATENCY_CONFIG = {
  thresholds: {
    high: { abs: 75, relative: 0.75 },
    elevated: { abs: 35, relative: 0.35 }
  }
} as const;

type Protocol = 'IPv4' | 'IPv6';

interface SpeedtestEntry {
  download: number;
  upload: number;
}

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
  protocol: Protocol;
  packetLoss: number;
}

type LatencyTarget = Record<string, NormalizedLatencyEntry[]>;

interface ParsedDataPoint {
  timestamp: number;     // Epoc ms for fast sorting/filtering
  formattedTime: string; // Local HH:mm for charts & displays
  date: string;          // User's local date (yyyy-MM-dd)
  speedtest?: SpeedtestEntry;
  latency?: LatencyTarget;
}

interface LatencyStatEntry extends NormalizedLatencyEntry {
  cls: string;
  label: string;
  baseline: number;
}

interface CalculatedLatencyStat {
  target: string;
  latest: number;
  latestEntries: LatencyStatEntry[];
  cls: string;
  label: string;
}

type RawDataPayload = Record<string, Record<string, {
  speedtest?: SpeedtestEntry[];
  latency?: Record<string, RawLatencyEntry[]>;
}>>;

// --- State ---

let rawData: ParsedDataPoint[] = [];
let localDates: string[] = [];

const charts: {
  latency: Chart | null;
  speedtest: Chart | null;
} = {
  latency: null,
  speedtest: null
};

const latencyCache = new Map<string, CalculatedLatencyStat[]>();

// --- DOM References ---

function getElement<T extends HTMLElement>(id: string): T {
  const el = document.getElementById(id);
  if (!el) throw new Error(`Element with id "${id}" not found.`);
  return el as T;
}

const ui = {
  dateFilter: getElement<HTMLSelectElement>('dateFilter'),
  protocolFilter: getElement<HTMLSelectElement>('protocolFilter'),

  status: getElement<HTMLElement>('statusContainer'),
  error: getElement<HTMLElement>('errorAlert'),
  empty: getElement<HTMLElement>('emptyAlert'),
  loading: getElement<HTMLElement>('loadingSkeleton'),
  main: getElement<HTMLElement>('mainContent'),

  speedCard: getElement<HTMLElement>('speedtestCard'),
  speedSection: getElement<HTMLElement>('speedtestChartSection'),
  latencyCards: getElement<HTMLElement>('latencyCardsContainer'),

  latestDownload: getElement<HTMLElement>('latestDownload'),
  latestUpload: getElement<HTMLElement>('latestUpload'),
  speedTime: getElement<HTMLElement>('speedtestTime')
};

// --- CSS Helpers & Style Cache ---

let styleCache: CSSStyleDeclaration | null = null;

function getCSSVar(name: string, fallback = ''): string {
  if (!styleCache) {
    styleCache = getComputedStyle(document.documentElement);
  }
  return styleCache.getPropertyValue(name).trim() || fallback;
}

function getThemeColors() {
  return {
    text: getCSSVar('--chart-text', '#ffffff'),
    grid: getCSSVar('--chart-grid', 'rgba(255,255,255,0.1)')
  };
}

function getChartPalette(): string[] {
  return [
    getCSSVar('--chart-c1', '#00d2ff'),
    getCSSVar('--chart-c2', '#39ff14'),
    getCSSVar('--chart-c3', '#ff9900'),
    getCSSVar('--chart-c4', '#ff4d4d'),
    getCSSVar('--chart-c5', '#a349eb'),
    getCSSVar('--chart-c6', '#22d3ee')
  ];
}

function withOpacity(color: string, opacity = 0.2): string {
  if (color.startsWith('rgb')) {
    return color.replace('rgb', 'rgba').replace(')', `, ${opacity})`);
  }
  return color;
}

function setView(state: 'loading' | 'error' | 'empty' | 'content'): void {
  ui.status.classList.toggle('hidden', state === 'content');
  ui.loading.classList.toggle('hidden', state !== 'loading');
  ui.error.classList.toggle('hidden', state !== 'error');
  ui.empty.classList.toggle('hidden', state !== 'empty');
  ui.main.classList.toggle('hidden', state !== 'content');
}

// --- Data Normalization & Parsing ---

function normalizeLatency(latency?: Record<string, RawLatencyEntry[]>): LatencyTarget | undefined {
  if (!latency) return undefined;

  const result: LatencyTarget = {};

  for (const [target, entries] of Object.entries(latency)) {
    result[target] = entries.map(e => ({
      average: e.Average ?? e.average ?? 0,
      protocol: ((e.Protocol ?? e.protocol ?? 'IPv4') as Protocol),
      packetLoss: e.PacketLoss ?? e.packetLoss ?? 0
    }));
  }

  return result;
}

function parseData(json: RawDataPayload): void {
  const localDateSet = new Set<string>();
  const parsedPoints: ParsedDataPoint[] = [];

  for (const [dateKey, times] of Object.entries(json)) {
    for (const [timeKey, entry] of Object.entries(times)) {
      // Convert UTC timestamp from JSON to user's local timezone
      const dt = DateTime.fromISO(`${dateKey}T${timeKey}`, { zone: 'utc' }).toLocal();
      if (!dt.isValid) continue;

      const userLocalDate = dt.toFormat('yyyy-MM-dd');
      localDateSet.add(userLocalDate);

      parsedPoints.push({
        timestamp: dt.toMillis(),
        formattedTime: dt.toFormat('HH:mm'),
        date: userLocalDate,
        speedtest: entry.speedtest?.[0],
        latency: normalizeLatency(entry.latency)
      });
    }
  }

  // Chronological sort for chart rendering
  rawData = parsedPoints.sort((a, b) => a.timestamp - b.timestamp);

  // Create sorted list of actual local dates present in data
  localDates = Array.from(localDateSet).sort().reverse();
}

function getFilteredData(): ParsedDataPoint[] {
  const selectedDate = ui.dateFilter.value;
  const selectedProtocol = ui.protocolFilter.value;

  return rawData.filter(d => {
    if (d.date !== selectedDate) return false;
    if (!d.latency || selectedProtocol === 'Both') return true;

    for (const entries of Object.values(d.latency)) {
      if (entries.some(e => e.protocol === selectedProtocol)) return true;
    }
    return false;
  });
}

// --- Latency Calculations ---

function buildLatencyHistory(data: ParsedDataPoint[]): Map<string, Map<string, number[]>> {
  const history = new Map<string, Map<string, number[]>>();

  for (const d of data) {
    if (!d.latency) continue;

    for (const [target, entries] of Object.entries(d.latency)) {
      let targetHistory = history.get(target);
      if (!targetHistory) {
        targetHistory = new Map();
        history.set(target, targetHistory);
      }

      for (const e of entries) {
        let protoHistory = targetHistory.get(e.protocol);
        if (!protoHistory) {
          protoHistory = [];
          targetHistory.set(e.protocol, protoHistory);
        }
        protoHistory.push(e.average);
      }
    }
  }

  return history;
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

function computeLatencyStats(
  target: string,
  history: Map<string, Map<string, number[]>>,
  latestPoint: ParsedDataPoint,
  protocolFilter: string
): CalculatedLatencyStat {
  const targetHistory = history.get(target)!;
  const rawLatestEntries = latestPoint.latency?.[target] ?? [];

  const latestEntries: LatencyStatEntry[] = (protocolFilter === 'Both'
    ? rawLatestEntries
    : rawLatestEntries.filter(e => e.protocol === protocolFilter)
  ).map(e => {
    const protoValues = targetHistory.get(e.protocol) || [e.average];
    const protoBaseline = protoValues.reduce((a, b) => a + b, 0) / protoValues.length;

    const delta = e.average - protoBaseline;
    const status = getLatencyStatus(protoBaseline, e.average, delta);

    return { ...e, ...status, baseline: protoBaseline };
  });

  const latest = latestEntries.length > 0
    ? latestEntries.reduce((a, b) => a + b.average, 0) / latestEntries.length
    : 0;

  let overallStatus = { cls: 'status-success', label: 'Normal' };
  for (const e of latestEntries) {
    if (e.cls === 'status-error') {
      overallStatus = { cls: 'status-error', label: 'High' };
      break;
    } else if (e.cls === 'status-warning' && overallStatus.cls !== 'status-error') {
      overallStatus = { cls: 'status-warning', label: 'Elevated' };
    }
  }

  return { target, latest, latestEntries, ...overallStatus };
}

function computeAllLatencyStats(data: ParsedDataPoint[], protocol: string): CalculatedLatencyStat[] {
  const history = buildLatencyHistory(data);
  let latestPoint: ParsedDataPoint | undefined;

  for (let i = data.length - 1; i >= 0; i--) {
    if (data[i].latency) {
      latestPoint = data[i];
      break;
    }
  }

  if (!history.size || !latestPoint) return [];

  return Array.from(history.keys())
    .map(target => computeLatencyStats(target, history, latestPoint!, protocol))
    .filter(stat => stat.latestEntries.length > 0)
    .sort((a, b) => a.latest - b.latest);
}

function getCachedLatencyStats(
  selectedDate: string,
  protocol: string,
  data: ParsedDataPoint[]
): CalculatedLatencyStat[] {
  if (!data.length) return [];

  // Deterministic key based on explicitly selected UI parameters
  const cacheKey = `${selectedDate}_${protocol}_${data.length}`;

  if (latencyCache.has(cacheKey)) {
    return latencyCache.get(cacheKey)!;
  }

  const result = computeAllLatencyStats(data, protocol);
  latencyCache.set(cacheKey, result);

  return result;
}

// --- DOM Rendering ---

function renderLatencyCard(container: HTMLElement, stat: CalculatedLatencyStat): void {
  const el = document.createElement('div');
  el.className = 'instrument-box';

  const entries = stat.latestEntries || [];

  const lossTags = entries
    .filter(e => e.packetLoss > 0)
    .map(e => `
      <div style="display: flex; align-items: center;">
        <span class="protocol-tag">${e.protocol}</span>
        <span class="loss-tag">${e.packetLoss.toFixed(1)}% LOSS</span>
      </div>
    `)
    .join('');

  const valuesHtml = entries.map(e => `
    <div style="font-size: 1.2rem; display: flex; justify-content: space-between; align-items: baseline; width: 100%;">
      <span style="font-size: 0.7rem; color: var(--text-muted);">${e.protocol}</span>
      <span class="${e.cls}">${e.average.toFixed(2)}<span class="instrument-unit">ms</span></span>
    </div>
  `).join('');

  el.innerHTML = `
    <div class="instrument-label">
      <span>${stat.target}</span>
      <div style="display: flex; gap: 6px; align-items: center;">
        ${lossTags}
      </div>
    </div>
    <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; flex: 1; width: 100%;">
      ${valuesHtml}
    </div>
    <div class="instrument-footer">CURRENT LATENCY</div>
  `;

  container.appendChild(el);
}

function updateLatencyCards(data: ParsedDataPoint[], protocol: string): void {
  ui.latencyCards.replaceChildren();

  if (ui.speedCard) {
    ui.latencyCards.appendChild(ui.speedCard);
  }

  const stats = getCachedLatencyStats(ui.dateFilter.value, protocol, data);

  if (!stats.length && (!ui.speedCard || ui.speedCard.classList.contains('hidden'))) {
    const msg = document.createElement('div');
    msg.style.gridColumn = '1 / -1';
    msg.textContent = 'No latency data.';
    ui.latencyCards.appendChild(msg);
  }

  stats.forEach(stat => renderLatencyCard(ui.latencyCards, stat));
}

function updateSpeedCard(data: ParsedDataPoint[]): void {
  let latest: ParsedDataPoint | undefined;

  for (let i = data.length - 1; i >= 0; i--) {
    if (data[i].speedtest) {
      latest = data[i];
      break;
    }
  }

  if (!latest?.speedtest) {
    ui.speedCard.classList.add('hidden');
    ui.speedSection.classList.add('hidden');
    return;
  }

  ui.speedCard.classList.remove('hidden');
  ui.speedSection.classList.remove('hidden');

  ui.latestDownload.textContent = latest.speedtest.download.toFixed(0);
  ui.latestUpload.textContent = latest.speedtest.upload.toFixed(0);
  ui.speedTime.textContent = `LAST TEST AT ${latest.formattedTime}`;
}

function destroyCharts(): void {
  charts.latency?.destroy();
  charts.speedtest?.destroy();
  charts.latency = null;
  charts.speedtest = null;
}

function renderCharts(data: ParsedDataPoint[], protocol: string): void {
  destroyCharts();

  const { text, grid } = getThemeColors();
  const palette = getChartPalette();

  Chart.defaults.color = text;
  Chart.defaults.borderColor = grid;
  Chart.defaults.font.family = 'Inter';

  const latencyCtx = getElement<HTMLCanvasElement>('latencyChart');
  const latencyData = data.filter(d => d.latency);
  const labels = latencyData.map(d => d.formattedTime);

  const targetMap: Record<string, { latency: (number | null)[]; loss: (number | null)[] }> = {};

  latencyData.forEach((d, i) => {
    if (!d.latency) return;

    Object.entries(d.latency).forEach(([target, entries]) => {
      entries.forEach(e => {
        if (protocol !== 'Both' && e.protocol !== protocol) return;

        const key = protocol === 'Both' ? `${target} (${e.protocol})` : target;

        if (!targetMap[key]) {
          targetMap[key] = {
            latency: new Array(latencyData.length).fill(null),
            loss: new Array(latencyData.length).fill(null)
          };
        }

        targetMap[key].latency[i] = e.average;
        // Map 0 to null so zero-loss points don't draw lines or populate useless datasets
        targetMap[key].loss[i] = e.packetLoss > 0 ? e.packetLoss : null;
      });
    });
  });

  const targetEntries = Object.entries(targetMap)
    .map(([key, dataObj]) => {
      const validPoints = dataObj.latency.filter((val): val is number => val !== null);
      const avgLatency = validPoints.length > 0
        ? validPoints.reduce((sum, val) => sum + val, 0) / validPoints.length
        : Infinity;

      return { key, ...dataObj, avgLatency };
    })
    .sort((a, b) => a.avgLatency - b.avgLatency);

  const latencyDatasets: ChartDataset<'line'>[] = [];

  targetEntries.forEach(({ key, latency, loss }, i) => {
    const color = palette[i % palette.length];

    // Primary Solid Line: Latency (ms)
    latencyDatasets.push({
      label: key,
      data: latency,
      borderColor: color,
      backgroundColor: withOpacity(color, 0.1),
      borderWidth: 2,
      pointRadius: 2,
      tension: 0.3,
      spanGaps: true,
      yAxisID: 'y'
    });

    // Only add the dotted packet loss dataset if this metric actually experienced loss (> 0)
    const hasLoss = loss.some(val => val !== null && val > 0);
    if (hasLoss) {
      latencyDatasets.push({
        label: `${key} Loss (%)`,
        data: loss,
        borderColor: color,
        backgroundColor: 'transparent',
        borderDash: [5, 5],
        borderWidth: 2,
        pointRadius: 3,
        tension: 0.3,
        spanGaps: true,
        yAxisID: 'y1'
      });
    }
  });

  charts.latency = new Chart(latencyCtx, {
    type: 'line',
    data: { labels, datasets: latencyDatasets },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { position: 'top', align: 'start', labels: { boxWidth: 10, font: { size: 10 } } }
      },
      scales: {
        x: { grid: { color: grid }, ticks: { font: { size: 9 } } },
        y: {
          type: 'linear',
          display: true,
          position: 'left',
          title: { display: true, text: 'Latency (ms)', font: { size: 10 } },
          grid: { color: grid },
          ticks: { font: { size: 10 } }
        },
        y1: {
          type: 'linear',
          display: true,
          position: 'right',
          min: 0,
          max: 100,
          title: { display: true, text: 'Packet Loss (%)', font: { size: 10 } },
          grid: { drawOnChartArea: false },
          ticks: { font: { size: 10 } }
        }
      },
      interaction: { mode: 'index', intersect: false }
    }
  });

  // --- Speedtest Chart ---
  const speed = data.filter(d => d.speedtest);
  if (speed.length > 0) {
    ui.speedSection.classList.remove('hidden');
    const speedCtx = getElement<HTMLCanvasElement>('speedtestChart');

    charts.speedtest = new Chart(speedCtx, {
      type: 'bar',
      data: {
        labels: speed.map(d => d.formattedTime),
        datasets: [
          {
            label: 'Download',
            data: speed.map(d => d.speedtest!.download),
            backgroundColor: getCSSVar('--neon-blue', '#00d2ff'),
            borderColor: getCSSVar('--neon-blue', '#00d2ff'),
            borderWidth: 1,
            borderRadius: 4,
            categoryPercentage: 0.9,
            barPercentage: 0.95
          },
          {
            label: 'Upload',
            data: speed.map(d => d.speedtest!.upload),
            backgroundColor: getCSSVar('--neon-orange', '#ff9900'),
            borderColor: getCSSVar('--neon-orange', '#ff9900'),
            borderWidth: 1,
            borderRadius: 4,
            categoryPercentage: 0.9,
            barPercentage: 0.95
          }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { position: 'top', align: 'start' } },
        scales: {
          x: { grid: { color: grid }, ticks: { font: { size: 9 } } },
          y: {
            title: { display: true, text: 'Speed (Mbps)', font: { size: 10 } },
            grid: { color: grid },
            ticks: { font: { size: 10 } }
          }
        },
        interaction: { mode: 'index', intersect: false }
      }
    });
  } else {
    ui.speedSection.classList.add('hidden');
  }
}
// --- App Lifecycle ---

function populateFilters(): void {
  ui.dateFilter.replaceChildren(
    ...localDates.map(d => {
      const option = document.createElement('option');
      option.value = d;
      option.textContent = d;
      return option;
    })
  );

  if (!ui.dateFilter.value && localDates.length) {
    ui.dateFilter.value = localDates[0];
  }
}

function applyFilters(): void {
  const data = getFilteredData();
  if (!data.length) return setView('empty');

  setView('content');
  updateSpeedCard(data);
  updateLatencyCards(data, ui.protocolFilter.value);
  renderCharts(data, ui.protocolFilter.value);
}

function initFilters(): void {
  ui.dateFilter.addEventListener('change', applyFilters);
  ui.protocolFilter.addEventListener('change', applyFilters);
}

async function load(): Promise<void> {
  setView('loading');
  try {
    const res = await fetch('metrics.json');
    if (!res.ok) throw new Error(`HTTP error! Status: ${res.status}`);

    const json: RawDataPayload = await res.json();
    parseData(json);
    populateFilters();
    applyFilters();
  } catch (err) {
    console.error('Failed to load metrics:', err);
    setView('error');
  }
}

initFilters();
load();
