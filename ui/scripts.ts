import { Chart, ChartDataset } from "chart.js/auto";
import { DateTime } from "luxon";

// --- Types & Constants ---

// Protocol families supported for latency measurement targets.
type Protocol = "IPv4" | "IPv6";
type ProtocolFilter = Protocol | "IPv4 + IPv6";

// Theme modes supported by the dashboard. Dark mode is the default.
type Theme = "light" | "dark";

// UI visual view states.
type ViewState = "loading" | "error" | "empty" | "content";

// Status badge styling and textual label based on latency degradation.
type LatencyStatus = {
  cls: "status-ghost" | "status-error" | "status-warning" | "status-success";
  label: "No Data" | "High" | "Elevated" | "Normal";
};

const DEFAULT_PROTOCOL: Protocol = "IPv4";
const DEFAULT_PACKET_LOSS = 0;
const DEFAULT_LATENCY = 0;

// Threshold configuration for determining latency health status.
// Health is evaluated against both an absolute delta (ms above baseline)
// and a relative percentage increase (e.g. 75% or 35% above baseline).
const LATENCY_CONFIG = {
  thresholds: {
    high: { abs: 75, relative: 0.75 }, // Delta > max(75ms, 75% of baseline) => High
    elevated: { abs: 35, relative: 0.35 }, // Delta > max(35ms, 35% of baseline) => Elevated
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

const METRICS_URL = "metrics.json";
const THEME_STORAGE_KEY = "theme";
const DEFAULT_THEME: Theme = "dark";
const ALL_LATENCY_TARGETS = "__all__" as const;

// Speedtest bandwidth measurements in Mbps.
interface SpeedtestEntry {
  download: number;
  upload: number;
}

// Raw unvalidated latency entry shape from JSON payload (handles PascalCase or camelCase).
interface RawLatencyEntry {
  Average?: unknown;
  average?: unknown;
  Protocol?: unknown;
  protocol?: unknown;
  PacketLoss?: unknown;
  packetLoss?: unknown;
}

// Normalized and sanitized latency entry.
interface NormalizedLatencyEntry {
  average: number;
  protocol: Protocol;
  packetLoss: number;
}

// Latency target mapping: target name -> array of measurements (one per protocol).
type LatencyTarget = Record<string, NormalizedLatencyEntry[]>;

// Raw entry format inside a time slot in metrics.json.
interface RawDataEntry {
  speedtest?: unknown;
  latency?: unknown;
}

// Full raw payload schema: date ("yyyy-MM-dd") -> time ("HH:mm:ssZ") -> RawDataEntry.
type RawDataPayload = Record<string, Record<string, RawDataEntry>>;

// Parsed and localized data point ready for filtering and charting.
interface ParsedDataPoint {
  timestamp: number; // Epoch milliseconds for fast numeric sorting and date comparison.
  formattedTime: string; // Formatted user-local time string (HH:mm) for chart axis labels.
  date: string; // Formatted user-local date (yyyy-MM-dd) for daily dropdown filtering.
  speedtest?: SpeedtestEntry;
  latency?: LatencyTarget;
}

// Latency entry augmented with baseline comparisons and visual status class.
interface LatencyStatEntry extends NormalizedLatencyEntry {
  cls: LatencyStatus["cls"];
  label: LatencyStatus["label"];
  baseline: number;
}

// Aggregated health metrics for a target instrument card.
interface CalculatedLatencyStat {
  target: string;
  latest: number;
  latestEntries: LatencyStatEntry[];
  cls: LatencyStatus["cls"];
  label: LatencyStatus["label"];
}

interface ThemeColors {
  text: string;
  grid: string;
}

interface ChartRegistry {
  latency: Chart | null;
  speedtest: Chart | null;
}

// Cached references to static HTML DOM elements.
interface UIElements {
  dateFilter: HTMLSelectElement;
  protocolFilter: HTMLSelectElement;
  latencyTargetFilter: HTMLSelectElement;
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

// Parallel data arrays aligned with chart timestamp labels.
interface LatencySeries {
  latency: Array<number | null>; // Latency values (ms), null if target wasn't probed at timestamp.
  loss: Array<number | null>; // Packet loss percentages (0-100), null if 0% or unprobed.
}

interface LatencySeriesEntry extends LatencySeries {
  key: string;
  avgLatency: number;
}

// --- State ---

// In-memory collection of all localized data points across all dates.
let rawData: ParsedDataPoint[] = [];
// List of unique local dates available for selection in descending chronological order.
let localDates: string[] = [];
// Cached computed style declaration to avoid repeated expensive DOM style lookups.
let styleCache: CSSStyleDeclaration | null = null;

// Active Chart.js instances.
const charts: ChartRegistry = {
  latency: null,
  speedtest: null,
};

// Memoization cache for computed latency stats keyed by `${date}|${protocol}|${dataLength}`.
const latencyCache = new Map<string, CalculatedLatencyStat[]>();

// --- Type Guards & Validation ---

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isProtocol(value: unknown): value is Protocol {
  return value === "IPv4" || value === "IPv6";
}

// Validates numeric values and guarantees non-negative finite results.
function toNonNegativeFiniteNumber(value: unknown, fallback: number): number {
  return isFiniteNumber(value) && value >= 0 ? value : fallback;
}

function normalizeProtocol(value: unknown): Protocol {
  return isProtocol(value) ? value : DEFAULT_PROTOCOL;
}

// Validates and extracts speedtest upload/download values.
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

// Normalizes latency entries from raw JSON, handling property case variations and clamping packet loss (0-100).
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
        toNonNegativeFiniteNumber(rawPacketLoss, DEFAULT_PACKET_LOSS),
      );

      entries.push({
        average,
        protocol: normalizeProtocol(rawProtocol),
        packetLoss,
      });
    }

    if (entries.length > 0) {
      result[target] = entries;
    }
  }

  return Object.keys(result).length > 0 ? result : undefined;
}

// Validates the top-level structure of the metrics payload.
function parseRawDataPayload(value: unknown): RawDataPayload {
  if (!isRecord(value)) {
    throw new Error("Metrics payload must be a JSON object.");
  }

  const result: RawDataPayload = Object.create(null) as RawDataPayload;

  for (const [dateKey, rawTimes] of Object.entries(value)) {
    if (!isRecord(rawTimes)) continue;

    const times: Record<string, RawDataEntry> = Object.create(null) as Record<
      string,
      RawDataEntry
    >;

    for (const [timeKey, rawEntry] of Object.entries(rawTimes)) {
      if (!isRecord(rawEntry)) continue;
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

// --- DOM Helpers ---

// Retrieves a required DOM element by ID and asserts its type, throwing if not found.
function getElement<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id);
  if (!element) {
    throw new Error(`Required element with id "${id}" was not found.`);
  }
  return element as T;
}

const ui: UIElements = {
  dateFilter: getElement<HTMLSelectElement>("dateFilter"),
  protocolFilter: getElement<HTMLSelectElement>("protocolFilter"),
  latencyTargetFilter: getElement<HTMLSelectElement>("latencyTargetFilter"),
  themeToggle: getElement<HTMLButtonElement>("themeToggle"),

  status: getElement<HTMLElement>("statusContainer"),
  error: getElement<HTMLElement>("errorAlert"),
  empty: getElement<HTMLElement>("emptyAlert"),
  loading: getElement<HTMLElement>("loadingSkeleton"),
  main: getElement<HTMLElement>("mainContent"),

  speedCard: getElement<HTMLElement>("speedtestCard"),
  speedSection: getElement<HTMLElement>("speedtestChartSection"),
  latencyCards: getElement<HTMLElement>("latencyCardsContainer"),

  latestDownload: getElement<HTMLElement>("latestDownload"),
  latestUpload: getElement<HTMLElement>("latestUpload"),
  speedTime: getElement<HTMLElement>("speedtestTime"),
};

// --- CSS Helpers ---

// Reads a CSS custom property from :root, using an in-memory cache to minimize DOM reflows.
function getCSSVar(name: string, fallback = ""): string {
  if (!styleCache) {
    styleCache = getComputedStyle(document.documentElement);
  }

  const value = styleCache.getPropertyValue(name).trim();
  return value || fallback;
}

// Retrieves active theme text and grid lines colors for Chart.js.
function getThemeColors(): ThemeColors {
  return {
    text: getCSSVar("--chart-text", DEFAULT_CHART_TEXT),
    grid: getCSSVar("--chart-grid", DEFAULT_CHART_GRID),
  };
}

// Retrieves the distinct multi-series color palette defined in CSS.
function getChartPalette(): string[] {
  return DEFAULT_CHART_PALETTE.map((fallback, index) =>
    getCSSVar(`--chart-c${index + 1}`, fallback),
  );
}

// Converts an rgb(...) color string to rgba(..., opacity) for chart background fills.
function withOpacity(color: string, opacity = 0.2): string {
  if (!Number.isFinite(opacity)) return color;

  const clampedOpacity = Math.min(1, Math.max(0, opacity));
  const rgbMatch = color.match(
    /^rgb\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*\)$/i,
  );

  if (!rgbMatch) return color;

  return `rgba(${rgbMatch[1]}, ${rgbMatch[2]}, ${rgbMatch[3]}, ${clampedOpacity})`;
}

// Controls visibility of UI state containers (loading skeleton, error box, empty message, content).
function setView(state: ViewState): void {
  ui.status.classList.toggle("hidden", state === "content");
  ui.loading.classList.toggle("hidden", state !== "loading");
  ui.error.classList.toggle("hidden", state !== "error");
  ui.empty.classList.toggle("hidden", state !== "empty");
  ui.main.classList.toggle("hidden", state !== "content");
}

// --- Data Parsing ---

// Converts the raw UTC JSON data from metrics.json into chronological local time data points.
function parseData(json: RawDataPayload): void {
  const localDateSet = new Set<string>();
  const parsedPoints: ParsedDataPoint[] = [];

  for (const [dateKey, times] of Object.entries(json)) {
    for (const [timeKey, entry] of Object.entries(times)) {
      // Parse ISO UTC timestamp ("yyyy-MM-ddTHH:mm:ssZ") and convert to client's local timezone.
      const dt = DateTime.fromISO(`${dateKey}T${timeKey}`, {
        zone: "utc",
      }).toLocal();

      if (!dt.isValid) continue;

      const userLocalDate = dt.toFormat("yyyy-MM-dd");
      localDateSet.add(userLocalDate);

      parsedPoints.push({
        timestamp: dt.toMillis(),
        formattedTime: dt.toFormat("HH:mm"),
        date: userLocalDate,
        speedtest: parseSpeedtest(entry.speedtest),
        latency: normalizeLatency(entry.latency),
      });
    }
  }

  // Sort strictly ascending by timestamp.
  rawData = parsedPoints.sort((a, b) => a.timestamp - b.timestamp);
  // Sort date dropdown choices descending (most recent date first).
  localDates = Array.from(localDateSet).sort().reverse();
  latencyCache.clear();
}

// Filters global rawData by the currently selected date and IP protocol.
function getFilteredData(): ParsedDataPoint[] {
  const selectedDate = ui.dateFilter.value;
  const selectedProtocol = ui.protocolFilter.value as ProtocolFilter;

  return rawData.filter((point) => {
    if (point.date !== selectedDate) return false;
    if (!point.latency || selectedProtocol === "IPv4 + IPv6") return true;

    // Filter points to only those containing samples matching the selected protocol.
    return Object.values(point.latency).some((entries) =>
      entries.some((entry) => entry.protocol === selectedProtocol),
    );
  });
}

// --- Latency Calculations ---

// Builds a lookup map of historical ping averages per target and protocol across all data points.
function buildLatencyHistory(
  data: ParsedDataPoint[],
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

// Evaluates whether current latency represents Normal, Elevated, or High degradation
// relative to baseline using hybrid thresholds (both absolute ms and percentage delta).
function getLatencyStatus(
  baselineAverage: number,
  latest: number,
  delta: number,
): LatencyStatus {
  if (latest === 0) {
    return { cls: "status-ghost", label: "No Data" };
  }

  const { high, elevated } = LATENCY_CONFIG.thresholds;

  if (delta > Math.max(high.abs, baselineAverage * high.relative)) {
    return { cls: "status-error", label: "High" };
  }

  if (delta > Math.max(elevated.abs, baselineAverage * elevated.relative)) {
    return { cls: "status-warning", label: "Elevated" };
  }

  return { cls: "status-success", label: "Normal" };
}

// Calculates arithmetic mean of a numbers array.
function average(values: readonly number[]): number {
  if (values.length === 0) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

// Computes current metrics and health status for an individual target against its historical baseline.
function computeLatencyStats(
  target: string,
  history: Map<string, Map<Protocol, number[]>>,
  latestPoint: ParsedDataPoint,
  protocolFilter: ProtocolFilter,
): CalculatedLatencyStat {
  const targetHistory = history.get(target);
  const rawLatestEntries = latestPoint.latency?.[target] ?? [];

  if (!targetHistory) {
    return {
      target,
      latest: 0,
      latestEntries: [],
      cls: "status-ghost",
      label: "No Data",
    };
  }

  const latestEntries: LatencyStatEntry[] = rawLatestEntries
    .filter(
      (entry) =>
        protocolFilter === "IPv4 + IPv6" || entry.protocol === protocolFilter,
    )
    .map((entry) => {
      const protocolValues = targetHistory.get(entry.protocol) ?? [
        entry.average,
      ];
      const protocolBaseline = average(protocolValues);
      const delta = entry.average - protocolBaseline;
      const status = getLatencyStatus(protocolBaseline, entry.average, delta);

      return {
        ...entry,
        ...status,
        baseline: protocolBaseline,
      };
    });

  const latest = average(latestEntries.map((entry) => entry.average));

  // Determine worst-case overall status for this target (Error > Warning > Success).
  let overallStatus: LatencyStatus = {
    cls: "status-success",
    label: "Normal",
  };

  for (const entry of latestEntries) {
    if (entry.cls === "status-error") {
      overallStatus = { cls: "status-error", label: "High" };
      break;
    }

    if (
      entry.cls === "status-warning" &&
      overallStatus.cls !== "status-error"
    ) {
      overallStatus = { cls: "status-warning", label: "Elevated" };
    }
  }

  if (latestEntries.length === 0) {
    overallStatus = { cls: "status-ghost", label: "No Data" };
  }

  return {
    target,
    latest,
    latestEntries,
    ...overallStatus,
  };
}

// Computes latency health stats for all monitored targets in the dataset.
function computeAllLatencyStats(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
): CalculatedLatencyStat[] {
  const history = buildLatencyHistory(data);
  let latestPoint: ParsedDataPoint | undefined;

  // Find the most recent data point that contains latency measurements.
  for (let index = data.length - 1; index >= 0; index -= 1) {
    if (data[index]?.latency) {
      latestPoint = data[index];
      break;
    }
  }

  if (!history.size || !latestPoint || !latestPoint.latency) return [];

  // Order targets based on the latest data point to preserve the backend's configured order
  // (e.g. Gateway, Cloudflare DNS, Youtube), falling back to historical targets if any were missed.
  const targetOrder = new Set<string>([
    ...Object.keys(latestPoint.latency),
    ...history.keys(),
  ]);

  return Array.from(targetOrder)
    .map((target) =>
      computeLatencyStats(target, history, latestPoint, protocolFilter),
    )
    .filter((stat) => stat.latestEntries.length > 0);
}

// Retrieves cached latency stats or calculates and memoizes them.
function getCachedLatencyStats(
  selectedDate: string,
  protocolFilter: ProtocolFilter,
  data: ParsedDataPoint[],
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

// Creates a DOM element with optional CSS class, text content, and inline style overrides.
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

// Renders an instrument card for a single latency target, showing current average latency,
// color-coded health status (Normal, Elevated, High), and packet loss badges if non-zero.
function renderLatencyCard(
  container: HTMLElement,
  stat: CalculatedLatencyStat,
): void {
  const card = createElement("div", { className: "instrument-box" });
  const header = createElement("div", { className: "instrument-label" });
  const target = createElement("span", { textContent: stat.target });
  const tags = createElement("div", {
    styles: {
      display: "flex",
      gap: "6px",
      alignItems: "center",
    },
  });

  // Display packet loss warning tags for protocols with packet drop > 0%.
  for (const entry of stat.latestEntries) {
    if (entry.packetLoss <= 0) continue;

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

  // Render numerical latency values per protocol (e.g. IPv4 and IPv6 rows).
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

    const protocolLabel = createElement("span", {
      textContent: entry.protocol,
      styles: {
        fontSize: "0.9rem",
      },
    });

    const value = createElement("span", {
      className: entry.cls,
      textContent: entry.average.toFixed(2),
    });

    const unit = createElement("span", {
      className: "instrument-unit",
      textContent: "ms",
    });

    value.appendChild(unit);
    row.append(protocolLabel, value);
    values.appendChild(row);
  }

  const footer = createElement("div", {
    className: "instrument-footer",
    textContent: "CURRENT LATENCY",
  });

  card.append(header, values, footer);
  container.appendChild(card);
}

// Re-renders all latency instrument cards inside the cards container.
function updateLatencyCards(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
): void {
  ui.latencyCards.replaceChildren();

  // Preserve the existing speedtest card position in the DOM grid.
  ui.latencyCards.appendChild(ui.speedCard);

  const stats = getCachedLatencyStats(
    ui.dateFilter.value,
    protocolFilter,
    data,
  );

  if (!stats.length && ui.speedCard.classList.contains("hidden")) {
    const message = createElement("div", {
      textContent: "No latency data.",
      styles: { gridColumn: "1 / -1" },
    });
    ui.latencyCards.appendChild(message);
  }

  for (const stat of stats) {
    renderLatencyCard(ui.latencyCards, stat);
  }
}

// Finds the most recent data point containing speedtest results.
function findLatestSpeedtest(
  data: ParsedDataPoint[],
): ParsedDataPoint | undefined {
  for (let index = data.length - 1; index >= 0; index -= 1) {
    if (data[index]?.speedtest) return data[index];
  }
  return undefined;
}

// Updates the download/upload speed overview card or hides it if speedtest data is absent.
function updateSpeedCard(data: ParsedDataPoint[]): void {
  const latest = findLatestSpeedtest(data);

  if (!latest?.speedtest) {
    ui.speedCard.classList.add("hidden");
    ui.speedSection.classList.add("hidden");
    return;
  }

  ui.speedCard.classList.remove("hidden");
  ui.speedSection.classList.remove("hidden");

  ui.latestDownload.textContent = latest.speedtest.download.toFixed(0);
  ui.latestUpload.textContent = latest.speedtest.upload.toFixed(0);
  ui.speedTime.textContent = `Speedtest (${latest.formattedTime})`;
}

// --- Charts ---

// Cleans up existing Chart.js instances to prevent memory leaks and duplicate canvas bindings.
function destroyCharts(): void {
  charts.latency?.destroy();
  charts.speedtest?.destroy();
  charts.latency = null;
  charts.speedtest = null;
}

// Constructs aligned data series for each target and protocol.
// Ensures every series array matches the exact length and timestamps of the X-axis labels,
// using null to fill timestamps where a target had no measurement.
function buildLatencySeries(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
  targetFilter: string,
): Map<string, LatencySeries> {
  const series = new Map<string, LatencySeries>();
  const latencyData = data.filter((point) => point.latency);

  latencyData.forEach((point, index) => {
    if (!point.latency) return;

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

        let targetSeries = series.get(key);
        if (!targetSeries) {
          targetSeries = {
            latency: new Array<number | null>(latencyData.length).fill(null),
            loss: new Array<number | null>(latencyData.length).fill(null),
          };
          series.set(key, targetSeries);
        }

        targetSeries.latency[index] = entry.average;
        targetSeries.loss[index] =
          entry.packetLoss > 0 ? entry.packetLoss : null;
      }
    }
  });

  return series;
}

// Converts series map into an array sorted by overall average latency.
function toLatencySeriesEntries(
  series: Map<string, LatencySeries>,
): LatencySeriesEntry[] {
  return Array.from(series.entries()).map(([key, data]) => {
    const validPoints = data.latency.filter(
      (value): value is number => value !== null,
    );

    return {
      key,
      ...data,
      avgLatency: average(validPoints) || Infinity,
    };
  });
}

// Renders the multi-series line chart with dual Y-axes (Latency in ms on left, Packet Loss % on right).
function renderLatencyChart(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
  targetFilter: string,
  text: string,
  grid: string,
  palette: string[],
): void {
  const latencyCtx = getElement<HTMLCanvasElement>("latencyChart");
  const latencyData = data.filter((point) => point.latency);
  const labels = latencyData.map((point) => point.formattedTime);
  const targetEntries = toLatencySeriesEntries(
    buildLatencySeries(data, protocolFilter, targetFilter),
  );
  const latencyDatasets: ChartDataset<"line">[] = [];

  targetEntries.forEach(({ key, latency, loss }, index) => {
    const color = palette[index % palette.length];

    // Solid line dataset for round-trip latency.
    latencyDatasets.push({
      label: key,
      data: latency,
      borderColor: color,
      backgroundColor: withOpacity(color, 0.1),
      borderWidth: 2,
      pointRadius: 2,
      tension: 0.3,
      spanGaps: true,
      yAxisID: "y",
    });

    // Dashed line dataset for packet loss % on secondary Y-axis (only added if packet loss occurred).
    const hasLoss = loss.some((value) => value !== null && value > 0);
    if (hasLoss) {
      latencyDatasets.push({
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
  });

  charts.latency = new Chart(latencyCtx, {
    type: "line",
    data: { labels, datasets: latencyDatasets },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          position: "top",
          align: "start",
          labels: {
            boxWidth: 10,
            font: { size: 10 },
          },
        },
      },
      scales: {
        x: {
          grid: { color: grid },
          ticks: { font: { size: 9 } },
        },
        y: {
          type: "linear",
          display: true,
          position: "left",
          title: {
            display: true,
            text: "Latency (ms)",
            font: { size: 10 },
          },
          grid: { color: grid },
          ticks: { font: { size: 10 } },
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
            font: { size: 10 },
          },
          grid: { drawOnChartArea: false },
          ticks: { font: { size: 10 } },
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

// Renders the speedtest bar chart comparing download and upload throughput.
function renderSpeedtestChart(data: ParsedDataPoint[], grid: string): void {
  const speed = data.filter(
    (point): point is ParsedDataPoint & { speedtest: SpeedtestEntry } =>
      point.speedtest !== undefined,
  );

  if (speed.length === 0) {
    ui.speedSection.classList.add("hidden");
    return;
  }

  ui.speedSection.classList.remove("hidden");

  const speedCtx = getElement<HTMLCanvasElement>("speedtestChart");
  const downloadColor = getCSSVar("--neon-blue", "#00d2ff");
  const uploadColor = getCSSVar("--neon-orange", "#ff9900");

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
          grid: { color: grid },
          ticks: { font: { size: 9 } },
        },
        y: {
          title: {
            display: true,
            text: "Speed (Mbps)",
            font: { size: 10 },
          },
          grid: { color: grid },
          ticks: { font: { size: 10 } },
        },
      },
      interaction: {
        mode: "index",
        intersect: false,
      },
    },
  });
}

// Configures global Chart.js styling defaults and renders both charts.
function renderCharts(
  data: ParsedDataPoint[],
  protocolFilter: ProtocolFilter,
  targetFilter: string,
): void {
  destroyCharts();

  const { text, grid } = getThemeColors();
  const palette = getChartPalette();

  Chart.defaults.color = text;
  Chart.defaults.borderColor = grid;
  Chart.defaults.font.family = "Inter";

  renderLatencyChart(data, protocolFilter, targetFilter, text, grid, palette);
  renderSpeedtestChart(data, grid);
}

// --- App Lifecycle ---

// Populates the date selector dropdown with available chronological dates.
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

// Populates the latency target selector from the available addresses/targets.
// The "All" option is always first and is the default when no previous selection exists.
function populateLatencyTargetFilter(data: ParsedDataPoint[]): void {
  const targets = new Set<string>();

  for (const point of data) {
    if (!point.latency) continue;

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

  for (const target of Array.from(targets).sort((a, b) => a.localeCompare(b))) {
    const option = document.createElement("option");
    option.value = target;
    option.textContent = target;
    fragment.appendChild(option);
  }

  ui.latencyTargetFilter.replaceChildren(fragment);

  const hasPreviousSelection =
    previousSelection === ALL_LATENCY_TARGETS || targets.has(previousSelection);

  ui.latencyTargetFilter.value = hasPreviousSelection
    ? previousSelection
    : ALL_LATENCY_TARGETS;
}

// Applies active date and protocol filters, updating telemetry cards, overview widgets, and charts.
function applyFilters(): void {
  const data = getFilteredData();

  if (!data.length) {
    updateSpeedCard([]);
    ui.latencyCards.replaceChildren();
    destroyCharts();
    setView("empty");
    return;
  }

  const protocolFilter = ui.protocolFilter.value as ProtocolFilter;

  populateLatencyTargetFilter(data);
  const targetFilter = ui.latencyTargetFilter.value;

  setView("content");
  updateSpeedCard(data);
  updateLatencyCards(data, protocolFilter);
  renderCharts(data, protocolFilter, targetFilter);
}

// Attaches event listeners to filter dropdowns.
function initFilters(): void {
  ui.dateFilter.addEventListener("change", applyFilters);
  ui.protocolFilter.addEventListener("change", applyFilters);
  ui.latencyTargetFilter.addEventListener("change", applyFilters);
}

// Fetches the live metrics.json dataset from the server and initialises the dashboard.
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

// --- Theme Management ---

// Reads the saved light/dark theme preference, defaulting to dark.
function getStoredTheme(): Theme {
  try {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    return stored === "light" || stored === "dark" ? stored : DEFAULT_THEME;
  } catch {
    // Storage may be unavailable due to privacy settings or browser policy.
    return DEFAULT_THEME;
  }
}

// Updates theme toggle icon and accessibility attributes.
function updateThemeToggleUI(theme: Theme): void {
  const isLight = theme === "light";
  const labelText = isLight ? "Switch to dark theme" : "Switch to light theme";

  ui.themeToggle.setAttribute("aria-label", labelText);
  ui.themeToggle.setAttribute("title", labelText);

  ui.themeToggle
    .querySelector<HTMLElement>(".sun-icon")
    ?.classList.toggle("hidden", !isLight);
  ui.themeToggle
    .querySelector<HTMLElement>(".moon-icon")
    ?.classList.toggle("hidden", isLight);
}

// Persists the explicit light/dark theme preference to localStorage.
function setStoredTheme(theme: Theme): void {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch {
    // Theme rendering still works when persistent storage is unavailable.
  }
}

// Applies the selected theme to the DOM and re-renders charts with theme-matching colors.
function applyTheme(theme: Theme): void {
  styleCache = null; // Invalidate cached CSS variables.
  setStoredTheme(theme);

  document.documentElement.setAttribute("data-theme", theme);
  updateThemeToggleUI(theme);

  if (rawData.length > 0) {
    const data = getFilteredData();
    if (data.length > 0) {
      renderCharts(
        data,
        ui.protocolFilter.value as ProtocolFilter,
        ui.latencyTargetFilter.value,
      );
    }
  }
}

// Toggles between the explicit light and dark themes.
function toggleTheme(): void {
  const current = getStoredTheme();
  const next: Theme = current === "dark" ? "light" : "dark";
  applyTheme(next);
}

// Initializes theme state using the saved preference, with dark as the default.
// No operating-system color-scheme detection is performed.
function initTheme(): void {
  const theme = getStoredTheme();

  document.documentElement.setAttribute("data-theme", theme);
  updateThemeToggleUI(theme);

  ui.themeToggle.addEventListener("click", toggleTheme);
}

// --- Bootstrap ---

initFilters();
initTheme();
void load();
