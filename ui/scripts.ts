import { Chart, ChartDataset } from "chart.js/auto";
import { DateTime } from "luxon";

// -----------------------------------------------------------------------------
// Types & Constants
// -----------------------------------------------------------------------------

type Protocol = "IPv4" | "IPv6";
type ProtocolFilter = Protocol | "IPv4 + IPv6";
type Theme = "light" | "dark";
type ViewState = "loading" | "error" | "empty" | "content";
type LatencyTimeFilter = "today" | "1h" | "5h" | "12h";

type StatusClass =
  "status-ghost" | "status-error" | "status-warning" | "status-success";

type StatusLabel = "No Data" | "High" | "Elevated" | "Normal";

interface LatencyStatus {
  cls: StatusClass;
  label: StatusLabel;
}

interface SpeedtestEntry {
  download: number;
  upload: number;
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
  timestamp: number;
  formattedTime: string;
  date: string;
  speedtest?: SpeedtestEntry;
  latency?: LatencyTarget;
}

interface LatencyStatEntry extends NormalizedLatencyEntry {
  cls: StatusClass;
  label: StatusLabel;
  baseline: number;
}

interface CalculatedLatencyStat extends LatencyStatus {
  target: string;
  latest: number;
  latestEntries: LatencyStatEntry[];
}

interface ThemeColors {
  text: string;
  grid: string;
}

interface LatencySeries {
  latency: Array<number | null>;
  loss: Array<number | null>;
}

interface LatencySeriesEntry extends LatencySeries {
  key: string;
  avgLatency: number;
}

interface UIElements {
  dateFilter: HTMLSelectElement;
  protocolFilter: HTMLSelectElement;
  latencyTargetFilter: HTMLSelectElement;
  latencyTimeFilter: HTMLSelectElement;
  latencyTimeControlGroup: HTMLElement;
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

interface ChartRegistry {
  latency: Chart | null;
  speedtest: Chart | null;
}

const DEFAULT_TIME_FILTER: LatencyTimeFilter = "today";
const DEFAULT_PROTOCOL: Protocol = "IPv4";
const DEFAULT_PACKET_LOSS = 0;
const DEFAULT_LATENCY = 0;
const DEFAULT_THEME: Theme = "dark";

const ALL_LATENCY_TARGETS = "__all__" as const;

const METRICS_URL = "metrics.json";
const THEME_STORAGE_KEY = "theme";

const TIME_FILTER_DURATIONS = {
  "1h": 60 * 60 * 1000,
  "5h": 5 * 60 * 60 * 1000,
  "12h": 12 * 60 * 60 * 1000,
} as const;

const LATENCY_CONFIG = {
  high: {
    abs: 75,
    relative: 0.75,
  },
  elevated: {
    abs: 35,
    relative: 0.35,
  },
} as const;

const DEFAULT_CHART_TEXT = "#ffffff";
const DEFAULT_CHART_GRID = "rgba(255,255,255,0.1)";

const DEFAULT_CHART_PALETTE = [
  "#00d2ff",
  "#39ff14",
  "#ff9900",
  "#ff4d4d",
  "#a349eb",
  "#22d3ee",
] as const;

// -----------------------------------------------------------------------------
// State
// -----------------------------------------------------------------------------

let rawData: ParsedDataPoint[] = [];
let localDates: string[] = [];
let styleCache: CSSStyleDeclaration | null = null;

const charts: ChartRegistry = {
  latency: null,
  speedtest: null,
};

const latencyCache = new Map<string, CalculatedLatencyStat[]>();

// -----------------------------------------------------------------------------
// Type Guards & Validation
// -----------------------------------------------------------------------------

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isProtocol(value: unknown): value is Protocol {
  return value === "IPv4" || value === "IPv6";
}

function isProtocolFilter(value: unknown): value is ProtocolFilter {
  return isProtocol(value) || value === "IPv4 + IPv6";
}

function isLatencyTimeFilter(value: unknown): value is LatencyTimeFilter {
  return (
    value === "today" || value === "1h" || value === "5h" || value === "12h"
  );
}

function isTheme(value: unknown): value is Theme {
  return value === "light" || value === "dark";
}

function toNonNegativeFiniteNumber(value: unknown, fallback: number): number {
  return isFiniteNumber(value) && value >= 0 ? value : fallback;
}

function normalizeProtocol(value: unknown): Protocol {
  return isProtocol(value) ? value : DEFAULT_PROTOCOL;
}

// -----------------------------------------------------------------------------
// DOM
// -----------------------------------------------------------------------------

function getElement<T extends HTMLElement>(
  id: string,
  isType: (element: HTMLElement) => element is T,
): T {
  const element = document.getElementById(id);

  if (!element) {
    throw new Error(`Required element "${id}" was not found.`);
  }

  if (!isType(element)) {
    throw new Error(
      `Required element "${id}" has an unexpected HTML element type.`,
    );
  }

  return element;
}

const ui: UIElements = {
  dateFilter: getElement(
    "dateFilter",
    (element): element is HTMLSelectElement =>
      element instanceof HTMLSelectElement,
  ),

  protocolFilter: getElement(
    "protocolFilter",
    (element): element is HTMLSelectElement =>
      element instanceof HTMLSelectElement,
  ),

  latencyTargetFilter: getElement(
    "latencyTargetFilter",
    (element): element is HTMLSelectElement =>
      element instanceof HTMLSelectElement,
  ),

  latencyTimeFilter: getElement(
    "latencyTimeFilter",
    (element): element is HTMLSelectElement =>
      element instanceof HTMLSelectElement,
  ),

  latencyTimeControlGroup: getElement(
    "latencyTimeControlGroup",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  themeToggle: getElement(
    "themeToggle",
    (element): element is HTMLButtonElement =>
      element instanceof HTMLButtonElement,
  ),

  status: getElement(
    "statusContainer",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  error: getElement(
    "errorAlert",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  empty: getElement(
    "emptyAlert",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  loading: getElement(
    "loadingSkeleton",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  main: getElement(
    "mainContent",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  speedCard: getElement(
    "speedtestCard",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  speedSection: getElement(
    "speedtestChartSection",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  latencyCards: getElement(
    "latencyCardsContainer",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  latestDownload: getElement(
    "latestDownload",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  latestUpload: getElement(
    "latestUpload",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),

  speedTime: getElement(
    "speedtestTime",
    (element): element is HTMLElement => element instanceof HTMLElement,
  ),
};

// -----------------------------------------------------------------------------
// CSS / Theme Helpers
// -----------------------------------------------------------------------------

function getCSSVar(name: string, fallback = ""): string {
  styleCache ??= getComputedStyle(document.documentElement);

  return styleCache.getPropertyValue(name).trim() || fallback;
}

function getThemeColors(): ThemeColors {
  return {
    text: getCSSVar("--chart-text", DEFAULT_CHART_TEXT),
    grid: getCSSVar("--chart-grid", DEFAULT_CHART_GRID),
  };
}

function getChartPalette(): string[] {
  return DEFAULT_CHART_PALETTE.map((fallback, index) =>
    getCSSVar(`--chart-c${index + 1}`, fallback),
  );
}

function withOpacity(color: string, opacity = 0.2): string {
  if (!Number.isFinite(opacity)) {
    return color;
  }

  const alpha = Math.min(1, Math.max(0, opacity));

  const match = color.match(
    /^rgb\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*\)$/i,
  );

  if (!match) {
    return color;
  }

  return `rgba(${match[1]}, ${match[2]}, ${match[3]}, ${alpha})`;
}

function setView(state: ViewState): void {
  ui.status.classList.toggle("hidden", state === "content");
  ui.loading.classList.toggle("hidden", state !== "loading");
  ui.error.classList.toggle("hidden", state !== "error");
  ui.empty.classList.toggle("hidden", state !== "empty");
  ui.main.classList.toggle("hidden", state !== "content");
}

// -----------------------------------------------------------------------------
// Data Parsing
// -----------------------------------------------------------------------------

function parseSpeedtestEntry(value: unknown): SpeedtestEntry | undefined {
  if (!isRecord(value)) {
    return undefined;
  }

  const download = toNonNegativeFiniteNumber(value.download, NaN);
  const upload = toNonNegativeFiniteNumber(value.upload, NaN);

  if (!Number.isFinite(download) || !Number.isFinite(upload)) {
    return undefined;
  }

  return {
    download,
    upload,
  };
}

function parseSpeedtest(value: unknown): SpeedtestEntry | undefined {
  if (!Array.isArray(value) || value.length === 0) {
    return undefined;
  }

  return parseSpeedtestEntry(value[0]);
}

function normalizeLatency(value: unknown): LatencyTarget | undefined {
  if (!isRecord(value)) {
    return undefined;
  }

  const result: LatencyTarget = Object.create(null) as LatencyTarget;

  for (const [target, rawEntries] of Object.entries(value)) {
    if (!Array.isArray(rawEntries)) {
      continue;
    }

    const entries = rawEntries.flatMap((rawEntry): NormalizedLatencyEntry[] => {
      if (!isRecord(rawEntry)) {
        return [];
      }

      const average = toNonNegativeFiniteNumber(
        rawEntry.Average ?? rawEntry.average,
        DEFAULT_LATENCY,
      );

      const packetLoss = Math.min(
        100,
        toNonNegativeFiniteNumber(
          rawEntry.PacketLoss ?? rawEntry.packetLoss,
          DEFAULT_PACKET_LOSS,
        ),
      );

      return [
        {
          average,
          protocol: normalizeProtocol(rawEntry.Protocol ?? rawEntry.protocol),
          packetLoss,
        },
      ];
    });

    if (entries.length > 0) {
      result[target] = entries;
    }
  }

  return Object.keys(result).length > 0 ? result : undefined;
}

function parseRawDataPayload(value: unknown): RawDataPayload {
  if (!isRecord(value)) {
    throw new Error("Metrics payload must be a JSON object.");
  }

  const result: RawDataPayload = Object.create(null) as RawDataPayload;

  for (const [dateKey, rawTimes] of Object.entries(value)) {
    if (!isRecord(rawTimes)) {
      continue;
    }

    const times: Record<string, RawDataEntry> = Object.create(null) as Record<
      string,
      RawDataEntry
    >;

    for (const [timeKey, rawEntry] of Object.entries(rawTimes)) {
      if (!isRecord(rawEntry)) {
        continue;
      }

      times[timeKey] = {
        speedtest: rawEntry.speedtest,
        latency: rawEntry.latency,
      };
    }

    if (Object.keys(times).length > 0) {
      result[dateKey] = times;
    }
  }

  return result;
}

function parseData(json: RawDataPayload): void {
  const dates = new Set<string>();
  const points: ParsedDataPoint[] = [];

  for (const [dateKey, times] of Object.entries(json)) {
    for (const [timeKey, entry] of Object.entries(times)) {
      const dateTime = DateTime.fromISO(`${dateKey}T${timeKey}`, {
        zone: "utc",
      }).toLocal();

      if (!dateTime.isValid) {
        continue;
      }

      const localDate = dateTime.toFormat("yyyy-MM-dd");

      dates.add(localDate);

      points.push({
        timestamp: dateTime.toMillis(),
        formattedTime: dateTime.toFormat("HH:mm"),
        date: localDate,
        speedtest: parseSpeedtest(entry.speedtest),
        latency: normalizeLatency(entry.latency),
      });
    }
  }

  rawData = points.sort((a, b) => a.timestamp - b.timestamp);

  localDates = [...dates].sort().reverse();

  latencyCache.clear();
}

// -----------------------------------------------------------------------------
// Filters
// -----------------------------------------------------------------------------

function getSelectedProtocol(): ProtocolFilter {
  const value = ui.protocolFilter.value;

  return isProtocolFilter(value) ? value : DEFAULT_PROTOCOL;
}

function getToday(): string {
  return DateTime.now().toFormat("yyyy-MM-dd");
}

function isCurrentDay(date: string): boolean {
  return date === getToday();
}

function getLatencyTimeFilter(): LatencyTimeFilter {
  if (!isCurrentDay(ui.dateFilter.value)) {
    return DEFAULT_TIME_FILTER;
  }

  const value = ui.latencyTimeFilter.value;

  return isLatencyTimeFilter(value) ? value : DEFAULT_TIME_FILTER;
}

function getFilteredData(): ParsedDataPoint[] {
  const selectedDate = ui.dateFilter.value;
  const selectedProtocol = getSelectedProtocol();

  return rawData.filter((point) => {
    if (point.date !== selectedDate) {
      return false;
    }

    if (!point.latency || selectedProtocol === "IPv4 + IPv6") {
      return true;
    }

    return Object.values(point.latency).some((entries) =>
      entries.some((entry) => entry.protocol === selectedProtocol),
    );
  });
}

function getLatencyTimeCutoff(
  data: readonly ParsedDataPoint[],
  selectedDate: string,
  timeFilter: LatencyTimeFilter,
): number | null {
  if (!isCurrentDay(selectedDate) || timeFilter === "today") {
    return null;
  }

  // At this point timeFilter is narrowed to:
  // "1h" | "5h" | "12h"
  const duration = TIME_FILTER_DURATIONS[timeFilter];

  let latestLatencyTimestamp = 0;

  for (let index = data.length - 1; index >= 0; index -= 1) {
    if (data[index]?.latency) {
      latestLatencyTimestamp = data[index].timestamp;
      break;
    }
  }

  const referenceTime = Math.max(Date.now(), latestLatencyTimestamp);

  return referenceTime - duration;
}

function filterLatencyDataByTime(
  data: readonly ParsedDataPoint[],
  selectedDate: string,
  timeFilter: LatencyTimeFilter,
): ParsedDataPoint[] {
  const cutoff = getLatencyTimeCutoff(data, selectedDate, timeFilter);

  return cutoff === null
    ? [...data]
    : data.filter((point) => point.timestamp >= cutoff);
}

// -----------------------------------------------------------------------------
// Latency Calculations
// -----------------------------------------------------------------------------

function average(values: readonly number[]): number {
  if (values.length === 0) {
    return 0;
  }

  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function buildLatencyHistory(
  data: readonly ParsedDataPoint[],
): Map<string, Map<Protocol, number[]>> {
  const history = new Map<string, Map<Protocol, number[]>>();

  for (const point of data) {
    if (!point.latency) {
      continue;
    }

    for (const [target, entries] of Object.entries(point.latency)) {
      const targetHistory =
        history.get(target) ?? new Map<Protocol, number[]>();

      history.set(target, targetHistory);

      for (const entry of entries) {
        const protocolHistory = targetHistory.get(entry.protocol) ?? [];

        protocolHistory.push(entry.average);
        targetHistory.set(entry.protocol, protocolHistory);
      }
    }
  }

  return history;
}

function getLatencyStatus(baseline: number, latest: number): LatencyStatus {
  if (latest === 0) {
    return {
      cls: "status-ghost",
      label: "No Data",
    };
  }

  const delta = latest - baseline;

  const { high, elevated } = LATENCY_CONFIG;

  if (delta > Math.max(high.abs, baseline * high.relative)) {
    return {
      cls: "status-error",
      label: "High",
    };
  }

  if (delta > Math.max(elevated.abs, baseline * elevated.relative)) {
    return {
      cls: "status-warning",
      label: "Elevated",
    };
  }

  return {
    cls: "status-success",
    label: "Normal",
  };
}

function getWorstStatus(entries: readonly LatencyStatEntry[]): LatencyStatus {
  if (entries.length === 0) {
    return {
      cls: "status-ghost",
      label: "No Data",
    };
  }

  if (entries.some((entry) => entry.cls === "status-error")) {
    return {
      cls: "status-error",
      label: "High",
    };
  }

  if (entries.some((entry) => entry.cls === "status-warning")) {
    return {
      cls: "status-warning",
      label: "Elevated",
    };
  }

  return {
    cls: "status-success",
    label: "Normal",
  };
}

function computeLatencyStats(
  target: string,
  history: Map<string, Map<Protocol, number[]>>,
  latestPoint: ParsedDataPoint,
  protocolFilter: ProtocolFilter,
): CalculatedLatencyStat {
  const targetHistory = history.get(target);

  const rawEntries = latestPoint.latency?.[target] ?? [];

  if (!targetHistory) {
    return {
      target,
      latest: 0,
      latestEntries: [],
      cls: "status-ghost",
      label: "No Data",
    };
  }

  const latestEntries = rawEntries
    .filter(
      (entry) =>
        protocolFilter === "IPv4 + IPv6" || entry.protocol === protocolFilter,
    )
    .map((entry): LatencyStatEntry => {
      const values = targetHistory.get(entry.protocol) ?? [entry.average];

      const baseline = average(values);
      const status = getLatencyStatus(baseline, entry.average);

      return {
        ...entry,
        baseline,
        ...status,
      };
    });

  return {
    target,
    latest: average(latestEntries.map((entry) => entry.average)),
    latestEntries,
    ...getWorstStatus(latestEntries),
  };
}

function computeAllLatencyStats(
  data: readonly ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
): CalculatedLatencyStat[] {
  const history = buildLatencyHistory(data);

  const latestPoint = [...data].reverse().find((point) => point.latency);

  if (!latestPoint?.latency || history.size === 0) {
    return [];
  }

  const targetOrder = new Set<string>([
    ...Object.keys(latestPoint.latency),
    ...history.keys(),
  ]);

  return [...targetOrder]
    .map((target) =>
      computeLatencyStats(target, history, latestPoint, protocolFilter),
    )
    .filter((stat) => stat.latestEntries.length > 0);
}

function getCachedLatencyStats(
  selectedDate: string,
  protocolFilter: ProtocolFilter,
  data: ParsedDataPoint[],
): CalculatedLatencyStat[] {
  if (data.length === 0) {
    return [];
  }

  const cacheKey = `${selectedDate}|${protocolFilter}|${data.length}`;

  const cached = latencyCache.get(cacheKey);

  if (cached) {
    return cached;
  }

  const result = computeAllLatencyStats(data, protocolFilter);

  latencyCache.set(cacheKey, result);

  return result;
}

// -----------------------------------------------------------------------------
// DOM Rendering
// -----------------------------------------------------------------------------

function createElement<K extends keyof HTMLElementTagNameMap>(
  tagName: K,
  options: {
    className?: string;
    textContent?: string;
    styles?: Partial<CSSStyleDeclaration>;
  } = {},
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
  stat: CalculatedLatencyStat,
): void {
  const card = createElement("div", {
    className: "instrument-box",
  });

  const header = createElement("div", {
    className: "instrument-label",
  });

  const target = createElement("span", {
    textContent: stat.target,
  });

  const tags = createElement("div", {
    styles: {
      display: "flex",
      gap: "6px",
      alignItems: "center",
    },
  });

  for (const entry of stat.latestEntries) {
    if (entry.packetLoss <= 0) {
      continue;
    }

    const tagGroup = createElement("div", {
      styles: {
        display: "flex",
        alignItems: "center",
      },
    });

    tagGroup.append(
      createElement("span", {
        className: "protocol-tag",
        textContent: entry.protocol,
      }),

      createElement("span", {
        className: "loss-tag",
        textContent: `${entry.packetLoss.toFixed(1)}% LOSS`,
      }),
    );

    tags.appendChild(tagGroup);
  }

  header.append(target, tags);

  const values = createElement("div", {
    styles: {
      display: "flex",
      flexDirection: "column",
      alignItems: "center",
      justifyContent: "center",
      flex: "1",
      width: "100%",
    },
  });

  for (const entry of stat.latestEntries) {
    const row = createElement("div", {
      styles: {
        fontSize: "1.2rem",
        display: "flex",
        justifyContent: "space-between",
        alignItems: "baseline",
        width: "100%",
      },
    });

    const protocol = createElement("span", {
      textContent: entry.protocol,
      styles: {
        fontSize: "0.9rem",
      },
    });

    const value = createElement("span", {
      className: entry.cls,
      textContent: entry.average.toFixed(2),
    });

    value.appendChild(
      createElement("span", {
        className: "instrument-unit",
        textContent: "ms",
      }),
    );

    row.append(protocol, value);
    values.appendChild(row);
  }

  const footer = createElement("div", {
    className: "instrument-footer",
    textContent: "CURRENT LATENCY",
  });

  card.append(header, values, footer);
  container.appendChild(card);
}

function updateLatencyCards(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
): void {
  ui.latencyCards.replaceChildren(ui.speedCard);

  const stats = getCachedLatencyStats(
    ui.dateFilter.value,
    protocolFilter,
    data,
  );

  if (stats.length === 0 && ui.speedCard.classList.contains("hidden")) {
    ui.latencyCards.appendChild(
      createElement("div", {
        textContent: "No latency data.",
        styles: {
          gridColumn: "1 / -1",
        },
      }),
    );
  }

  for (const stat of stats) {
    renderLatencyCard(ui.latencyCards, stat);
  }
}

function findLatest<T>(
  data: readonly ParsedDataPoint[],
  selector: (point: ParsedDataPoint) => T | undefined,
): { point: ParsedDataPoint; value: T } | undefined {
  for (let index = data.length - 1; index >= 0; index -= 1) {
    const point = data[index];

    if (!point) {
      continue;
    }

    const value = selector(point);

    if (value !== undefined) {
      return {
        point,
        value,
      };
    }
  }

  return undefined;
}

function updateSpeedCard(data: ParsedDataPoint[]): void {
  const latest = findLatest(data, (point) => point.speedtest);

  if (!latest) {
    ui.speedCard.classList.add("hidden");
    ui.speedSection.classList.add("hidden");
    return;
  }

  ui.speedCard.classList.remove("hidden");
  ui.speedSection.classList.remove("hidden");

  ui.latestDownload.textContent = latest.value.download.toFixed(0);

  ui.latestUpload.textContent = latest.value.upload.toFixed(0);

  ui.speedTime.textContent = `Speedtest (${latest.point.formattedTime})`;
}

// -----------------------------------------------------------------------------
// Charts
// -----------------------------------------------------------------------------

function destroyCharts(): void {
  charts.latency?.destroy();
  charts.speedtest?.destroy();

  charts.latency = null;
  charts.speedtest = null;
}

function buildLatencySeries(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
  targetFilter: string,
): Map<string, LatencySeries> {
  const latencyData = data.filter((point) => point.latency);

  const series = new Map<string, LatencySeries>();

  latencyData.forEach((point, index) => {
    if (!point.latency) {
      return;
    }

    for (const [target, entries] of Object.entries(point.latency)) {
      if (targetFilter !== ALL_LATENCY_TARGETS && target !== targetFilter) {
        continue;
      }

      for (const entry of entries) {
        if (
          protocolFilter !== "IPv4 + IPv6" &&
          entry.protocol !== protocolFilter
        ) {
          continue;
        }

        const key =
          protocolFilter === "IPv4 + IPv6"
            ? `${target} (${entry.protocol})`
            : target;

        const targetSeries = series.get(key) ?? {
          latency: new Array<number | null>(latencyData.length).fill(null),

          loss: new Array<number | null>(latencyData.length).fill(null),
        };

        targetSeries.latency[index] = entry.average;

        targetSeries.loss[index] =
          entry.packetLoss > 0 ? entry.packetLoss : null;

        series.set(key, targetSeries);
      }
    }
  });

  return series;
}

function toLatencySeriesEntries(
  series: Map<string, LatencySeries>,
): LatencySeriesEntry[] {
  return [...series.entries()].map(([key, data]) => {
    const validValues = data.latency.filter(
      (value): value is number => value !== null,
    );

    return {
      key,
      ...data,
      avgLatency: average(validValues) || Infinity,
    };
  });
}

function createLatencyDatasets(
  entries: readonly LatencySeriesEntry[],
  palette: readonly string[],
): ChartDataset<"line">[] {
  return entries.flatMap(({ key, latency, loss }, index) => {
    const color = palette[index % palette.length];

    const datasets: ChartDataset<"line">[] = [
      {
        label: key,
        data: latency,
        borderColor: color,
        backgroundColor: withOpacity(color, 0.1),
        borderWidth: 2,
        pointRadius: 2,
        tension: 0.3,
        spanGaps: true,
        yAxisID: "y",
      },
    ];

    const hasLoss = loss.some((value) => value !== null && value > 0);

    if (hasLoss) {
      datasets.push({
        label: `${key} Loss (%)`,
        data: loss,
        borderColor: color,
        backgroundColor: "transparent",
        borderDash: [5, 5],
        borderWidth: 2,
        pointRadius: 3,
        tension: 0.3,
        spanGaps: true,
        yAxisID: "y1",
      });
    }

    return datasets;
  });
}

function renderLatencyChart(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
  targetFilter: string,
  timeFilter: LatencyTimeFilter,
  text: string,
  grid: string,
  palette: readonly string[],
): void {
  charts.latency?.destroy();

  const filteredData = filterLatencyDataByTime(
    data,
    ui.dateFilter.value,
    timeFilter,
  );

  const latencyData = filteredData.filter((point) => point.latency);

  const labels = latencyData.map((point) => point.formattedTime);

  const seriesEntries = toLatencySeriesEntries(
    buildLatencySeries(filteredData, protocolFilter, targetFilter),
  );

  const latencyCtx = getElement(
    "latencyChart",
    (element): element is HTMLCanvasElement =>
      element instanceof HTMLCanvasElement,
  );

  charts.latency = new Chart(latencyCtx, {
    type: "line",

    data: {
      labels,
      datasets: createLatencyDatasets(seriesEntries, palette),
    },

    options: {
      responsive: true,
      maintainAspectRatio: false,

      plugins: {
        legend: {
          position: "top",
          align: "start",

          labels: {
            boxWidth: 10,
            font: {
              size: 10,
            },
          },
        },
      },

      scales: {
        x: {
          grid: {
            color: grid,
          },

          ticks: {
            font: {
              size: 9,
            },
          },
        },

        y: {
          type: "linear",
          display: true,
          position: "left",

          title: {
            display: true,
            text: "Latency (ms)",
            font: {
              size: 10,
            },
          },

          grid: {
            color: grid,
          },

          ticks: {
            font: {
              size: 10,
            },
          },
        },

        y1: {
          type: "linear",
          display: true,
          position: "right",
          min: 0,
          max: 100,

          title: {
            display: true,
            text: "Packet Loss (%)",
            font: {
              size: 10,
            },
          },

          grid: {
            drawOnChartArea: false,
          },

          ticks: {
            font: {
              size: 10,
            },
          },
        },
      },

      interaction: {
        mode: "index",
        intersect: false,
      },
    },
  });

  Chart.defaults.color = text;
}

function renderSpeedtestChart(data: ParsedDataPoint[], grid: string): void {
  const speed = data.filter(
    (
      point,
    ): point is ParsedDataPoint & {
      speedtest: SpeedtestEntry;
    } => point.speedtest !== undefined,
  );

  if (speed.length === 0) {
    ui.speedSection.classList.add("hidden");
    return;
  }

  ui.speedSection.classList.remove("hidden");

  const downloadColor = getCSSVar("--neon-blue", "#00d2ff");

  const uploadColor = getCSSVar("--neon-orange", "#ff9900");

  const speedCtx = getElement(
    "speedtestChart",
    (element): element is HTMLCanvasElement =>
      element instanceof HTMLCanvasElement,
  );

  charts.speedtest = new Chart(speedCtx, {
    type: "bar",

    data: {
      labels: speed.map((point) => point.formattedTime),

      datasets: [
        {
          label: "Download",

          data: speed.map((point) => point.speedtest.download),

          backgroundColor: downloadColor,

          borderColor: downloadColor,

          borderWidth: 1,
          borderRadius: 4,
          categoryPercentage: 0.9,
          barPercentage: 0.95,
        },

        {
          label: "Upload",

          data: speed.map((point) => point.speedtest.upload),

          backgroundColor: uploadColor,

          borderColor: uploadColor,

          borderWidth: 1,
          borderRadius: 4,
          categoryPercentage: 0.9,
          barPercentage: 0.95,
        },
      ],
    },

    options: {
      responsive: true,
      maintainAspectRatio: false,

      plugins: {
        legend: {
          position: "top",
          align: "start",
        },
      },

      scales: {
        x: {
          grid: {
            color: grid,
          },

          ticks: {
            font: {
              size: 9,
            },
          },
        },

        y: {
          title: {
            display: true,
            text: "Speed (Mbps)",
            font: {
              size: 10,
            },
          },

          grid: {
            color: grid,
          },

          ticks: {
            font: {
              size: 10,
            },
          },
        },
      },

      interaction: {
        mode: "index",
        intersect: false,
      },
    },
  });
}

function renderCharts(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
  targetFilter: string,
  timeFilter: LatencyTimeFilter,
): void {
  destroyCharts();

  const { text, grid } = getThemeColors();

  const palette = getChartPalette();

  Chart.defaults.color = text;
  Chart.defaults.borderColor = grid;
  Chart.defaults.font.family = "Inter";

  renderLatencyChart(
    data,
    protocolFilter,
    targetFilter,
    timeFilter,
    text,
    grid,
    palette,
  );

  renderSpeedtestChart(data, grid);
}

function updateLatencyChart(): void {
  const data = getFilteredData();

  if (data.length === 0) {
    return;
  }

  const { text, grid } = getThemeColors();

  renderLatencyChart(
    data,
    getSelectedProtocol(),
    ui.latencyTargetFilter.value,
    getLatencyTimeFilter(),
    text,
    grid,
    getChartPalette(),
  );
}

// -----------------------------------------------------------------------------
// Filter Population
// -----------------------------------------------------------------------------

function populateFilters(): void {
  const fragment = document.createDocumentFragment();

  for (const date of localDates) {
    const option = document.createElement("option");

    option.value = date;
    option.textContent = date;

    fragment.appendChild(option);
  }

  ui.dateFilter.replaceChildren(fragment);

  if (!ui.dateFilter.value && localDates.length > 0) {
    ui.dateFilter.value = localDates[0];
  }
}

function populateLatencyTargetFilter(data: readonly ParsedDataPoint[]): void {
  const targets = new Set<string>();

  for (const point of data) {
    if (!point.latency) {
      continue;
    }

    for (const target of Object.keys(point.latency)) {
      targets.add(target);
    }
  }

  const previousSelection = ui.latencyTargetFilter.value;

  const fragment = document.createDocumentFragment();

  const allOption = document.createElement("option");

  allOption.value = ALL_LATENCY_TARGETS;

  allOption.textContent = "All";

  fragment.appendChild(allOption);

  for (const target of [...targets].sort((a, b) => a.localeCompare(b))) {
    const option = document.createElement("option");

    option.value = target;
    option.textContent = target;

    fragment.appendChild(option);
  }

  ui.latencyTargetFilter.replaceChildren(fragment);

  ui.latencyTargetFilter.value =
    previousSelection === ALL_LATENCY_TARGETS || targets.has(previousSelection)
      ? previousSelection
      : ALL_LATENCY_TARGETS;
}

// -----------------------------------------------------------------------------
// Application
// -----------------------------------------------------------------------------

function applyFilters(): void {
  const data = getFilteredData();

  if (data.length === 0) {
    updateSpeedCard([]);
    ui.latencyCards.replaceChildren();
    destroyCharts();
    setView("empty");
    return;
  }

  const currentDay = isCurrentDay(ui.dateFilter.value);

  ui.latencyTimeControlGroup.classList.toggle("hidden", !currentDay);

  if (!currentDay) {
    ui.latencyTimeFilter.value = DEFAULT_TIME_FILTER;
  }

  populateLatencyTargetFilter(data);

  setView("content");

  updateSpeedCard(data);

  const protocolFilter = getSelectedProtocol();

  updateLatencyCards(data, protocolFilter);

  renderCharts(
    data,
    protocolFilter,
    ui.latencyTargetFilter.value,
    getLatencyTimeFilter(),
  );
}

function initFilters(): void {
  ui.dateFilter.addEventListener("change", applyFilters);

  ui.protocolFilter.addEventListener("change", applyFilters);

  ui.latencyTargetFilter.addEventListener("change", updateLatencyChart);

  ui.latencyTimeFilter.addEventListener("change", updateLatencyChart);
}

// -----------------------------------------------------------------------------
// Loading
// -----------------------------------------------------------------------------

async function load(): Promise<void> {
  setView("loading");

  try {
    const response = await fetch(METRICS_URL, {
      method: "GET",
      credentials: "same-origin",
      cache: "no-cache",

      headers: {
        Accept: "application/json",
      },
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
    console.error("Failed to load metrics.", error);

    destroyCharts();
    setView("error");
  }
}

// -----------------------------------------------------------------------------
// Theme Management
// -----------------------------------------------------------------------------

function getStoredTheme(): Theme {
  try {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);

    return isTheme(stored) ? stored : DEFAULT_THEME;
  } catch {
    return DEFAULT_THEME;
  }
}

function updateThemeToggleUI(theme: Theme): void {
  const isLight = theme === "light";

  const label = isLight ? "Switch to dark theme" : "Switch to light theme";

  ui.themeToggle.setAttribute("aria-label", label);

  ui.themeToggle.setAttribute("title", label);

  ui.themeToggle
    .querySelector<HTMLElement>(".sun-icon")
    ?.classList.toggle("hidden", !isLight);

  ui.themeToggle
    .querySelector<HTMLElement>(".moon-icon")
    ?.classList.toggle("hidden", isLight);
}

function setStoredTheme(theme: Theme): void {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch {
    // Persistent storage may be unavailable.
  }
}

function applyTheme(theme: Theme): void {
  styleCache = null;

  setStoredTheme(theme);

  document.documentElement.setAttribute("data-theme", theme);

  updateThemeToggleUI(theme);

  if (rawData.length === 0) {
    return;
  }

  const data = getFilteredData();

  if (data.length === 0) {
    return;
  }

  renderCharts(
    data,
    getSelectedProtocol(),
    ui.latencyTargetFilter.value,
    getLatencyTimeFilter(),
  );
}

function toggleTheme(): void {
  applyTheme(getStoredTheme() === "dark" ? "light" : "dark");
}

function initTheme(): void {
  const theme = getStoredTheme();

  document.documentElement.setAttribute("data-theme", theme);

  updateThemeToggleUI(theme);

  ui.themeToggle.addEventListener("click", toggleTheme);
}

// -----------------------------------------------------------------------------
// Bootstrap
// -----------------------------------------------------------------------------

initFilters();
initTheme();
void load();
