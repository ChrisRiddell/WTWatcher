import { Chart, ChartDataset } from 'chart.js/auto';
import { DateTime } from 'luxon';

// --- Types & Constants ---

type Protocol = 'IPv4' | 'IPv6';
type ProtocolFilter = Protocol | 'Both';
type Theme = 'light' | 'dark' | 'system';
type ResolvedTheme = Exclude<Theme, 'system'>;
type ViewState = 'loading' | 'error' | 'empty' | 'content';

type LatencyStatus = {
  cls: 'status-ghost' | 'status-error' | 'status-warning' | 'status-success';
  label: 'No Data' | 'High' | 'Elevated' | 'Normal';
};

const DEFAULT_PROTOCOL: Protocol = 'IPv4';
const DEFAULT_PACKET_LOSS = 0;
const DEFAULT_LATENCY = 0;

const LATENCY_CONFIG = {
  thresholds: {
    high: { abs: 75, relative: 0.75 },
    elevated: { abs: 35, relative: 0.35 }
  }
} as const;

const DEFAULT_CHART_TEXT = '#ffffff';
const DEFAULT_CHART_GRID = 'rgba(255,255,255,0.1)';
const DEFAULT_CHART_PALETTE = [
  '#00d2ff',
  '#39ff14',
  '#ff9900',
  '#ff4d4d',
  '#a349eb',
  '#22d3ee'
] as const;

const METRICS_URL = 'metrics.json';
const THEME_STORAGE_KEY = 'theme';

interface SpeedtestEntry {
  download: number;
  upload: number;
}

interface RawLatencyEntry {
  Average?: unknown;
  average?: unknown;
  Protocol?: unknown;
  protocol?: unknown;
  PacketLoss?: unknown;
  packetLoss?: unknown;
}

interface NormalizedLatencyEntry {
  average: number;
  protocol: Protocol;
  packetLoss: number;
}

type LatencyTarget = Record<string, NormalizedLatencyEntry[]>;

interface RawDataEntry {
  speedtest?: unknown;
  latency?: unknown;
}

type RawDataPayload = Record<string, Record<string, RawDataEntry>>;

interface ParsedDataPoint {
  timestamp: number; // Epoch milliseconds for fast sorting/filtering.
  formattedTime: string; // User-local HH:mm for charts and displays.
  date: string; // User-local yyyy-MM-dd.
  speedtest?: SpeedtestEntry;
  latency?: LatencyTarget;
}

interface LatencyStatEntry extends NormalizedLatencyEntry {
  cls: LatencyStatus['cls'];
  label: LatencyStatus['label'];
  baseline: number;
}

interface CalculatedLatencyStat {
  target: string;
  latest: number;
  latestEntries: LatencyStatEntry[];
  cls: LatencyStatus['cls'];
  label: LatencyStatus['label'];
}

interface ThemeColors {
  text: string;
  grid: string;
}

interface ChartRegistry {
  latency: Chart | null;
  speedtest: Chart | null;
}

interface UIElements {
  dateFilter: HTMLSelectElement;
  protocolFilter: HTMLSelectElement;
  themeToggle: HTMLButtonElement;
  status: HTMLElement;
  error: HTMLElement;
  empty: HTMLElement;
  loading: HTMLElement;
  main: HTMLElement;
  speedCard: HTMLElement;
  speedSection: HTMLElement;
  latencyCards: HTMLElement;
  latestDownload: HTMLElement;
  latestUpload: HTMLElement;
  speedTime: HTMLElement;
}

interface LatencySeries {
  latency: Array<number | null>;
  loss: Array<number | null>;
}

interface LatencySeriesEntry extends LatencySeries {
  key: string;
  avgLatency: number;
}

// --- State ---

let rawData: ParsedDataPoint[] = [];
let localDates: string[] = [];
let styleCache: CSSStyleDeclaration | null = null;

const charts: ChartRegistry = {
  latency: null,
  speedtest: null
};

const latencyCache = new Map<string, CalculatedLatencyStat[]>();

// --- Type Guards & Validation ---

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

function isProtocol(value: unknown): value is Protocol {
  return value === 'IPv4' || value === 'IPv6';
}

function isTheme(value: unknown): value is Theme {
  return value === 'light' || value === 'dark' || value === 'system';
}

function toNonNegativeFiniteNumber(value: unknown, fallback: number): number {
  return isFiniteNumber(value) && value >= 0 ? value : fallback;
}

function normalizeProtocol(value: unknown): Protocol {
  return isProtocol(value) ? value : DEFAULT_PROTOCOL;
}

function parseSpeedtestEntry(value: unknown): SpeedtestEntry | undefined {
  if (!isRecord(value)) return undefined;

  const download = toNonNegativeFiniteNumber(value.download, NaN);
  const upload = toNonNegativeFiniteNumber(value.upload, NaN);

  if (!Number.isFinite(download) || !Number.isFinite(upload)) return undefined;

  return { download, upload };
}

function parseSpeedtest(value: unknown): SpeedtestEntry | undefined {
  if (!Array.isArray(value) || value.length === 0) return undefined;
  return parseSpeedtestEntry(value[0]);
}

function parseRawLatencyEntry(value: unknown): RawLatencyEntry | undefined {
  return isRecord(value) ? value : undefined;
}

function normalizeLatency(latency: unknown): LatencyTarget | undefined {
  if (!isRecord(latency)) return undefined;

  const result: LatencyTarget = Object.create(null) as LatencyTarget;

  for (const [target, rawEntries] of Object.entries(latency)) {
    if (!Array.isArray(rawEntries)) continue;

    const entries: NormalizedLatencyEntry[] = [];

    for (const rawEntryValue of rawEntries) {
      const entry = parseRawLatencyEntry(rawEntryValue);
      if (!entry) continue;

      const rawAverage = entry.Average ?? entry.average;
      const rawProtocol = entry.Protocol ?? entry.protocol;
      const rawPacketLoss = entry.PacketLoss ?? entry.packetLoss;

      const average = toNonNegativeFiniteNumber(rawAverage, DEFAULT_LATENCY);
      const packetLoss = Math.min(
        100,
        toNonNegativeFiniteNumber(rawPacketLoss, DEFAULT_PACKET_LOSS)
      );

      entries.push({
        average,
        protocol: normalizeProtocol(rawProtocol),
        packetLoss
      });
    }

    if (entries.length > 0) {
      result[target] = entries;
    }
  }

  return Object.keys(result).length > 0 ? result : undefined;
}

function parseRawDataPayload(value: unknown): RawDataPayload {
  if (!isRecord(value)) {
    throw new Error('Metrics payload must be a JSON object.');
  }

  const result: RawDataPayload = Object.create(null) as RawDataPayload;

  for (const [dateKey, rawTimes] of Object.entries(value)) {
    if (!isRecord(rawTimes)) continue;

    const times: Record<string, RawDataEntry> = Object.create(null) as Record<string, RawDataEntry>;

    for (const [timeKey, rawEntry] of Object.entries(rawTimes)) {
      if (!isRecord(rawEntry)) continue;
      times[timeKey] = {
        speedtest: rawEntry.speedtest,
        latency: rawEntry.latency
      };
    }

    if (Object.keys(times).length > 0) {
      result[dateKey] = times;
    }
  }

  return result;
}

// --- DOM Helpers ---

function getElement<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id);
  if (!element) {
    throw new Error(`Required element with id "${id}" was not found.`);
  }
  return element as T;
}

const ui: UIElements = {
  dateFilter: getElement<HTMLSelectElement>('dateFilter'),
  protocolFilter: getElement<HTMLSelectElement>('protocolFilter'),
  themeToggle: getElement<HTMLButtonElement>('themeToggle'),

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

// --- CSS Helpers ---

function getCSSVar(name: string, fallback = ''): string {
  if (!styleCache) {
    styleCache = getComputedStyle(document.documentElement);
  }

  const value = styleCache.getPropertyValue(name).trim();
  return value || fallback;
}

function getThemeColors(): ThemeColors {
  return {
    text: getCSSVar('--chart-text', DEFAULT_CHART_TEXT),
    grid: getCSSVar('--chart-grid', DEFAULT_CHART_GRID)
  };
}

function getChartPalette(): string[] {
  return DEFAULT_CHART_PALETTE.map((fallback, index) =>
    getCSSVar(`--chart-c${index + 1}`, fallback)
  );
}

function withOpacity(color: string, opacity = 0.2): string {
  if (!Number.isFinite(opacity)) return color;

  const clampedOpacity = Math.min(1, Math.max(0, opacity));
  const rgbMatch = color.match(
    /^rgb\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*\)$/i
  );

  if (!rgbMatch) return color;

  return `rgba(${rgbMatch[1]}, ${rgbMatch[2]}, ${rgbMatch[3]}, ${clampedOpacity})`;
}

function setView(state: ViewState): void {
  ui.status.classList.toggle('hidden', state === 'content');
  ui.loading.classList.toggle('hidden', state !== 'loading');
  ui.error.classList.toggle('hidden', state !== 'error');
  ui.empty.classList.toggle('hidden', state !== 'empty');
  ui.main.classList.toggle('hidden', state !== 'content');
}

// --- Data Parsing ---

function parseData(json: RawDataPayload): void {
  const localDateSet = new Set<string>();
  const parsedPoints: ParsedDataPoint[] = [];

  for (const [dateKey, times] of Object.entries(json)) {
    for (const [timeKey, entry] of Object.entries(times)) {
      // Convert the source UTC timestamp to the user's local timezone.
      const dt = DateTime.fromISO(`${dateKey}T${timeKey}`, {
        zone: 'utc'
      }).toLocal();

      if (!dt.isValid) continue;

      const userLocalDate = dt.toFormat('yyyy-MM-dd');
      localDateSet.add(userLocalDate);

      parsedPoints.push({
        timestamp: dt.toMillis(),
        formattedTime: dt.toFormat('HH:mm'),
        date: userLocalDate,
        speedtest: parseSpeedtest(entry.speedtest),
        latency: normalizeLatency(entry.latency)
      });
    }
  }

  rawData = parsedPoints.sort((a, b) => a.timestamp - b.timestamp);
  localDates = Array.from(localDateSet).sort().reverse();
  latencyCache.clear();
}

function getFilteredData(): ParsedDataPoint[] {
  const selectedDate = ui.dateFilter.value;
  const selectedProtocol = ui.protocolFilter.value as ProtocolFilter;

  return rawData.filter((point) => {
    if (point.date !== selectedDate) return false;
    if (!point.latency || selectedProtocol === 'Both') return true;

    return Object.values(point.latency).some((entries) =>
      entries.some((entry) => entry.protocol === selectedProtocol)
    );
  });
}

// --- Latency Calculations ---

function buildLatencyHistory(
  data: ParsedDataPoint[]
): Map<string, Map<Protocol, number[]>> {
  const history = new Map<string, Map<Protocol, number[]>>();

  for (const point of data) {
    if (!point.latency) continue;

    for (const [target, entries] of Object.entries(point.latency)) {
      let targetHistory = history.get(target);
      if (!targetHistory) {
        targetHistory = new Map<Protocol, number[]>();
        history.set(target, targetHistory);
      }

      for (const entry of entries) {
        let protocolHistory = targetHistory.get(entry.protocol);
        if (!protocolHistory) {
          protocolHistory = [];
          targetHistory.set(entry.protocol, protocolHistory);
        }

        protocolHistory.push(entry.average);
      }
    }
  }

  return history;
}

function getLatencyStatus(
  baselineAverage: number,
  latest: number,
  delta: number
): LatencyStatus {
  if (latest === 0) {
    return { cls: 'status-ghost', label: 'No Data' };
  }

  const { high, elevated } = LATENCY_CONFIG.thresholds;

  if (delta > Math.max(high.abs, baselineAverage * high.relative)) {
    return { cls: 'status-error', label: 'High' };
  }

  if (delta > Math.max(elevated.abs, baselineAverage * elevated.relative)) {
    return { cls: 'status-warning', label: 'Elevated' };
  }

  return { cls: 'status-success', label: 'Normal' };
}

function average(values: readonly number[]): number {
  if (values.length === 0) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function computeLatencyStats(
  target: string,
  history: Map<string, Map<Protocol, number[]>>,
  latestPoint: ParsedDataPoint,
  protocolFilter: ProtocolFilter
): CalculatedLatencyStat {
  const targetHistory = history.get(target);
  const rawLatestEntries = latestPoint.latency?.[target] ?? [];

  if (!targetHistory) {
    return {
      target,
      latest: 0,
      latestEntries: [],
      cls: 'status-ghost',
      label: 'No Data'
    };
  }

  const latestEntries: LatencyStatEntry[] = rawLatestEntries
    .filter(
      (entry) =>
        protocolFilter === 'Both' || entry.protocol === protocolFilter
    )
    .map((entry) => {
      const protocolValues = targetHistory.get(entry.protocol) ?? [entry.average];
      const protocolBaseline = average(protocolValues);
      const delta = entry.average - protocolBaseline;
      const status = getLatencyStatus(
        protocolBaseline,
        entry.average,
        delta
      );

      return {
        ...entry,
        ...status,
        baseline: protocolBaseline
      };
    });

  const latest = average(latestEntries.map((entry) => entry.average));

  let overallStatus: LatencyStatus = {
    cls: 'status-success',
    label: 'Normal'
  };

  for (const entry of latestEntries) {
    if (entry.cls === 'status-error') {
      overallStatus = { cls: 'status-error', label: 'High' };
      break;
    }

    if (
      entry.cls === 'status-warning' &&
      overallStatus.cls !== 'status-error'
    ) {
      overallStatus = { cls: 'status-warning', label: 'Elevated' };
    }
  }

  if (latestEntries.length === 0) {
    overallStatus = { cls: 'status-ghost', label: 'No Data' };
  }

  return {
    target,
    latest,
    latestEntries,
    ...overallStatus
  };
}

function computeAllLatencyStats(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter
): CalculatedLatencyStat[] {
  const history = buildLatencyHistory(data);
  let latestPoint: ParsedDataPoint | undefined;

  for (let index = data.length - 1; index >= 0; index -= 1) {
    if (data[index]?.latency) {
      latestPoint = data[index];
      break;
    }
  }

  if (!history.size || !latestPoint) return [];

  return Array.from(history.keys())
    .map((target) =>
      computeLatencyStats(target, history, latestPoint, protocolFilter)
    )
    .filter((stat) => stat.latestEntries.length > 0)
    .sort((a, b) => a.latest - b.latest);
}

function getCachedLatencyStats(
  selectedDate: string,
  protocolFilter: ProtocolFilter,
  data: ParsedDataPoint[]
): CalculatedLatencyStat[] {
  if (!data.length) return [];

  const cacheKey = `${selectedDate}|${protocolFilter}|${data.length}`;
  const cached = latencyCache.get(cacheKey);

  if (cached) return cached;

  const result = computeAllLatencyStats(data, protocolFilter);
  latencyCache.set(cacheKey, result);

  return result;
}

// --- DOM Rendering ---

function createElement<K extends keyof HTMLElementTagNameMap>(
  tagName: K,
  options: {
    className?: string;
    textContent?: string;
    styles?: Partial<CSSStyleDeclaration>;
  } = {}
): HTMLElementTagNameMap[K] {
  const element = document.createElement(tagName);

  if (options.className) {
    element.className = options.className;
  }

  if (options.textContent !== undefined) {
    element.textContent = options.textContent;
  }

  if (options.styles) {
    Object.assign(element.style, options.styles);
  }

  return element;
}

function renderLatencyCard(
  container: HTMLElement,
  stat: CalculatedLatencyStat
): void {
  const card = createElement('div', { className: 'instrument-box' });
  const header = createElement('div', { className: 'instrument-label' });
  const target = createElement('span', { textContent: stat.target });
  const tags = createElement('div', {
    styles: {
      display: 'flex',
      gap: '6px',
      alignItems: 'center'
    }
  });

  for (const entry of stat.latestEntries) {
    if (entry.packetLoss <= 0) continue;

    const tagGroup = createElement('div', {
      styles: {
        display: 'flex',
        alignItems: 'center'
      }
    });

    tagGroup.append(
      createElement('span', {
        className: 'protocol-tag',
        textContent: entry.protocol
      }),
      createElement('span', {
        className: 'loss-tag',
        textContent: `${entry.packetLoss.toFixed(1)}% LOSS`
      })
    );

    tags.appendChild(tagGroup);
  }

  header.append(target, tags);

  const values = createElement('div', {
    styles: {
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      flex: '1',
      width: '100%'
    }
  });

  for (const entry of stat.latestEntries) {
    const row = createElement('div', {
      styles: {
        fontSize: '1.2rem',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'baseline',
        width: '100%'
      }
    });

    const protocolLabel = createElement('span', {
      textContent: entry.protocol,
      styles: {
        fontSize: '0.7rem'
      }
    });

    const value = createElement('span', {
      className: entry.cls,
      textContent: entry.average.toFixed(2)
    });

    const unit = createElement('span', {
      className: 'instrument-unit',
      textContent: 'ms'
    });

    value.appendChild(unit);
    row.append(protocolLabel, value);
    values.appendChild(row);
  }

  const footer = createElement('div', {
    className: 'instrument-footer',
    textContent: 'CURRENT LATENCY'
  });

  card.append(header, values, footer);
  container.appendChild(card);
}

function updateLatencyCards(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter
): void {
  ui.latencyCards.replaceChildren();

  // Preserve the existing card placement, matching the original DOM lifecycle.
  ui.latencyCards.appendChild(ui.speedCard);

  const stats = getCachedLatencyStats(ui.dateFilter.value, protocolFilter, data);

  if (
    !stats.length &&
    ui.speedCard.classList.contains('hidden')
  ) {
    const message = createElement('div', {
      textContent: 'No latency data.',
      styles: { gridColumn: '1 / -1' }
    });
    ui.latencyCards.appendChild(message);
  }

  for (const stat of stats) {
    renderLatencyCard(ui.latencyCards, stat);
  }
}

function findLatestSpeedtest(
  data: ParsedDataPoint[]
): ParsedDataPoint | undefined {
  for (let index = data.length - 1; index >= 0; index -= 1) {
    if (data[index]?.speedtest) return data[index];
  }
  return undefined;
}

function updateSpeedCard(data: ParsedDataPoint[]): void {
  const latest = findLatestSpeedtest(data);

  if (!latest?.speedtest) {
    ui.speedCard.classList.add('hidden');
    ui.speedSection.classList.add('hidden');
    return;
  }

  ui.speedCard.classList.remove('hidden');
  ui.speedSection.classList.remove('hidden');

  ui.latestDownload.textContent = latest.speedtest.download.toFixed(0);
  ui.latestUpload.textContent = latest.speedtest.upload.toFixed(0);
  ui.speedTime.textContent = `Speedtest (${latest.formattedTime})`;
}

// --- Charts ---

function destroyCharts(): void {
  charts.latency?.destroy();
  charts.speedtest?.destroy();
  charts.latency = null;
  charts.speedtest = null;
}

function buildLatencySeries(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter
): Map<string, LatencySeries> {
  const series = new Map<string, LatencySeries>();
  const latencyData = data.filter((point) => point.latency);

  latencyData.forEach((point, index) => {
    if (!point.latency) return;

    for (const [target, entries] of Object.entries(point.latency)) {
      for (const entry of entries) {
        if (
          protocolFilter !== 'Both' &&
          entry.protocol !== protocolFilter
        ) {
          continue;
        }

        const key =
          protocolFilter === 'Both'
            ? `${target} (${entry.protocol})`
            : target;

        let targetSeries = series.get(key);
        if (!targetSeries) {
          targetSeries = {
            latency: new Array<number | null>(latencyData.length).fill(null),
            loss: new Array<number | null>(latencyData.length).fill(null)
          };
          series.set(key, targetSeries);
        }

        targetSeries.latency[index] = entry.average;
        targetSeries.loss[index] = entry.packetLoss > 0 ? entry.packetLoss : null;
      }
    }
  });

  return series;
}

function toSortedLatencySeries(
  series: Map<string, LatencySeries>
): LatencySeriesEntry[] {
  return Array.from(series.entries())
    .map(([key, data]) => {
      const validPoints = data.latency.filter(
        (value): value is number => value !== null
      );

      return {
        key,
        ...data,
        avgLatency: average(validPoints) || Infinity
      };
    })
    .sort((a, b) => a.avgLatency - b.avgLatency);
}

function renderLatencyChart(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
  text: string,
  grid: string,
  palette: string[]
): void {
  const latencyCtx = getElement<HTMLCanvasElement>('latencyChart');
  const latencyData = data.filter((point) => point.latency);
  const labels = latencyData.map((point) => point.formattedTime);
  const targetEntries = toSortedLatencySeries(
    buildLatencySeries(data, protocolFilter)
  );
  const latencyDatasets: ChartDataset<'line'>[] = [];

  targetEntries.forEach(({ key, latency, loss }, index) => {
    const color = palette[index % palette.length];

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

    const hasLoss = loss.some((value) => value !== null && value > 0);
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
        legend: {
          position: 'top',
          align: 'start',
          labels: {
            boxWidth: 10,
            font: { size: 10 }
          }
        }
      },
      scales: {
        x: {
          grid: { color: grid },
          ticks: { font: { size: 9 } }
        },
        y: {
          type: 'linear',
          display: true,
          position: 'left',
          title: {
            display: true,
            text: 'Latency (ms)',
            font: { size: 10 }
          },
          grid: { color: grid },
          ticks: { font: { size: 10 } }
        },
        y1: {
          type: 'linear',
          display: true,
          position: 'right',
          min: 0,
          max: 100,
          title: {
            display: true,
            text: 'Packet Loss (%)',
            font: { size: 10 }
          },
          grid: { drawOnChartArea: false },
          ticks: { font: { size: 10 } }
        }
      },
      interaction: {
        mode: 'index',
        intersect: false
      }
    }
  });

  Chart.defaults.color = text;
}

function renderSpeedtestChart(
  data: ParsedDataPoint[],
  grid: string
): void {
  const speed = data.filter(
    (point): point is ParsedDataPoint & { speedtest: SpeedtestEntry } =>
      point.speedtest !== undefined
  );

  if (speed.length === 0) {
    ui.speedSection.classList.add('hidden');
    return;
  }

  ui.speedSection.classList.remove('hidden');

  const speedCtx = getElement<HTMLCanvasElement>('speedtestChart');
  const downloadColor = getCSSVar('--neon-blue', '#00d2ff');
  const uploadColor = getCSSVar('--neon-orange', '#ff9900');

  charts.speedtest = new Chart(speedCtx, {
    type: 'bar',
    data: {
      labels: speed.map((point) => point.formattedTime),
      datasets: [
        {
          label: 'Download',
          data: speed.map((point) => point.speedtest.download),
          backgroundColor: downloadColor,
          borderColor: downloadColor,
          borderWidth: 1,
          borderRadius: 4,
          categoryPercentage: 0.9,
          barPercentage: 0.95
        },
        {
          label: 'Upload',
          data: speed.map((point) => point.speedtest.upload),
          backgroundColor: uploadColor,
          borderColor: uploadColor,
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
      plugins: {
        legend: {
          position: 'top',
          align: 'start'
        }
      },
      scales: {
        x: {
          grid: { color: grid },
          ticks: { font: { size: 9 } }
        },
        y: {
          title: {
            display: true,
            text: 'Speed (Mbps)',
            font: { size: 10 }
          },
          grid: { color: grid },
          ticks: { font: { size: 10 } }
        }
      },
      interaction: {
        mode: 'index',
        intersect: false
      }
    }
  });
}

function renderCharts(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter
): void {
  destroyCharts();

  const { text, grid } = getThemeColors();
  const palette = getChartPalette();

  Chart.defaults.color = text;
  Chart.defaults.borderColor = grid;
  Chart.defaults.font.family = 'Inter';

  renderLatencyChart(data, protocolFilter, text, grid, palette);
  renderSpeedtestChart(data, grid);
}

// --- App Lifecycle ---

function populateFilters(): void {
  const fragment = document.createDocumentFragment();

  for (const date of localDates) {
    const option = document.createElement('option');
    option.value = date;
    option.textContent = date;
    fragment.appendChild(option);
  }

  ui.dateFilter.replaceChildren(fragment);

  if (!ui.dateFilter.value && localDates.length > 0) {
    ui.dateFilter.value = localDates[0];
  }
}

function applyFilters(): void {
  const data = getFilteredData();

  if (!data.length) {
    updateSpeedCard([]);
    ui.latencyCards.replaceChildren();
    destroyCharts();
    setView('empty');
    return;
  }

  const protocolFilter = ui.protocolFilter.value as ProtocolFilter;

  setView('content');
  updateSpeedCard(data);
  updateLatencyCards(data, protocolFilter);
  renderCharts(data, protocolFilter);
}

function initFilters(): void {
  ui.dateFilter.addEventListener('change', applyFilters);
  ui.protocolFilter.addEventListener('change', applyFilters);
}

async function load(): Promise<void> {
  setView('loading');

  try {
    const response = await fetch(METRICS_URL, {
      method: 'GET',
      credentials: 'same-origin',
      cache: 'no-cache',
      headers: {
        Accept: 'application/json'
      }
    });

    if (!response.ok) {
      throw new Error(`Metrics request failed with HTTP ${response.status}.`);
    }

    const payload: unknown = await response.json();
    const json = parseRawDataPayload(payload);

    parseData(json);
    populateFilters();
    applyFilters();
  } catch (error: unknown) {
    console.error('Failed to load metrics.', error);
    destroyCharts();
    setView('error');
  }
}

// --- Theme Management ---

function getSystemTheme(): ResolvedTheme {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light';
}

function getStoredTheme(): Theme {
  try {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    return isTheme(stored) ? stored : 'system';
  } catch {
    // Storage may be unavailable due to privacy settings or browser policy.
    return 'system';
  }
}

function getEffectiveTheme(): ResolvedTheme {
  const stored = getStoredTheme();
  return stored === 'system' ? getSystemTheme() : stored;
}

function updateThemeToggleUI(effectiveTheme: ResolvedTheme): void {
  const isLight = effectiveTheme === 'light';
  const labelText = isLight
    ? 'Switch to dark theme'
    : 'Switch to light theme';

  ui.themeToggle.setAttribute('aria-label', labelText);
  ui.themeToggle.setAttribute('title', labelText);

  ui.themeToggle
    .querySelector<HTMLElement>('.sun-icon')
    ?.classList.toggle('hidden', !isLight);
  ui.themeToggle
    .querySelector<HTMLElement>('.moon-icon')
    ?.classList.toggle('hidden', isLight);
}

function setStoredTheme(theme: Theme): void {
  try {
    if (theme === 'system') {
      localStorage.removeItem(THEME_STORAGE_KEY);
    } else {
      localStorage.setItem(THEME_STORAGE_KEY, theme);
    }
  } catch {
    // Theme rendering still works when persistent storage is unavailable.
  }
}

function applyTheme(theme: Theme): void {
  styleCache = null;
  setStoredTheme(theme);

  const effectiveTheme = theme === 'system' ? getSystemTheme() : theme;
  document.documentElement.setAttribute('data-theme', effectiveTheme);
  updateThemeToggleUI(effectiveTheme);

  if (rawData.length > 0) {
    const data = getFilteredData();
    if (data.length > 0) {
      renderCharts(data, ui.protocolFilter.value as ProtocolFilter);
    }
  }
}

function toggleTheme(): void {
  const current = getEffectiveTheme();
  const next: Theme = current === 'dark' ? 'light' : 'dark';
  applyTheme(next);
}

function initTheme(): void {
  const effectiveTheme = getEffectiveTheme();
  document.documentElement.setAttribute('data-theme', effectiveTheme);
  updateThemeToggleUI(effectiveTheme);

  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
  mediaQuery.addEventListener('change', () => {
    if (getStoredTheme() === 'system') {
      applyTheme('system');
    }
  });

  ui.themeToggle.addEventListener('click', toggleTheme);
}

// --- Bootstrap ---

initFilters();
initTheme();
void load();
