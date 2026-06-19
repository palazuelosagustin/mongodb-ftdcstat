const palette = ["#a44a3f", "#2f687d", "#647b3c", "#9a5d96", "#cb7c2d", "#38555f", "#996f2d", "#4e7a6d"];

let state = {
  metadata: null,
  data: null,
  selected: new Set(),
  filter: "",
  fullTimeMin: null,
  fullTimeMax: null,
  visibleTimeMin: null,
  visibleTimeMax: null,
  timeline: []
};

Promise.all([
  fetch("/api/metadata").then((r) => r.json()),
  fetch("/api/data").then((r) => r.json())
]).then(([metadata, data]) => {
  state.metadata = metadata;
  state.data = data;
  initializeTimeDomain();
  setDefaults();
  renderPage();

  document.getElementById("search").addEventListener("input", (event) => {
    state.filter = event.target.value.toLowerCase();
    renderMetricGroups();
    renderCharts();
  });
  document.getElementById("defaults").addEventListener("click", () => {
    setDefaults();
    renderMetricGroups();
    renderCharts();
  });
  document.getElementById("clear").addEventListener("click", () => {
    state.selected = new Set();
    renderMetricGroups();
    renderCharts();
  });
  document.getElementById("reset-zoom").addEventListener("click", () => {
    resetZoom();
  });
}).catch((error) => {
  document.getElementById("subtitle").textContent = `Failed to load capture: ${error}`;
});

function initializeTimeDomain() {
  state.timeline = [];
  for (const row of state.data.rows) {
    const timestamp = Date.parse(row.datetime);
    if (!Number.isFinite(timestamp)) continue;
    row.__ts = timestamp;
    state.timeline.push(timestamp);
  }
  if (!state.timeline.length) return;
  state.fullTimeMin = state.timeline[0];
  state.fullTimeMax = state.timeline[state.timeline.length - 1];
  state.visibleTimeMin = state.fullTimeMin;
  state.visibleTimeMax = state.fullTimeMax;
}

function setDefaults() {
  state.selected = new Set();
  for (const section of state.metadata.sections) {
    for (const metric of section.metrics) {
      if (metric.default) state.selected.add(metricKey(section.name, metric.jsonName));
    }
  }
}

function renderPage() {
  const topMeta = document.getElementById("top-meta");
  const subtitle = document.getElementById("subtitle");
  const avg = state.metadata.avg.enabled ? `avg ${state.metadata.avg.bucket}` : "raw intervals";
  subtitle.textContent = `${state.metadata.view} view • ${state.metadata.rowCount} plotted rows • ${avg} • ${formatRange(state.metadata.timeRange)}`;
  topMeta.innerHTML = "";
  addPill(topMeta, `view=${state.metadata.view}`);
  addPill(topMeta, state.metadata.avg.enabled ? `avg=${state.metadata.avg.bucket}` : "avg=off");
  addPill(topMeta, `${state.metadata.rowCount} rows`);
  if (state.metadata.timeRange.from) addPill(topMeta, `from=${state.metadata.timeRange.from}`);
  if (state.metadata.timeRange.to) addPill(topMeta, `to=${state.metadata.timeRange.to}`);
  document.getElementById("metadata").textContent = state.metadata.headerText || "";
  renderZoomStatus();
  renderWarnings();
  renderMetricGroups();
  renderCharts();
}

function renderZoomStatus() {
  const status = document.getElementById("zoom-status");
  const reset = document.getElementById("reset-zoom");
  const zoomed = isZoomed();
  if (zoomed) {
    status.textContent = `Zoom: ${formatTooltipTime(new Date(state.visibleTimeMin).toISOString())} -> ${formatTooltipTime(new Date(state.visibleTimeMax).toISOString())}`;
    reset.hidden = false;
  } else {
    status.textContent = "Zoom: full loaded range";
    reset.hidden = true;
  }
}

function isZoomed() {
  return state.visibleTimeMin !== null &&
    state.visibleTimeMax !== null &&
    (state.visibleTimeMin !== state.fullTimeMin || state.visibleTimeMax !== state.fullTimeMax);
}

function setZoomRange(minTs, maxTs) {
  if (!Number.isFinite(minTs) || !Number.isFinite(maxTs)) return;
  const nextMin = Math.max(state.fullTimeMin, Math.min(minTs, maxTs));
  const nextMax = Math.min(state.fullTimeMax, Math.max(minTs, maxTs));
  if (nextMax <= nextMin) return;
  state.visibleTimeMin = nextMin;
  state.visibleTimeMax = nextMax;
  redrawAllCharts();
}

function resetZoom() {
  if (state.fullTimeMin === null || state.fullTimeMax === null) return;
  state.visibleTimeMin = state.fullTimeMin;
  state.visibleTimeMax = state.fullTimeMax;
  redrawAllCharts();
}

function redrawAllCharts() {
  renderZoomStatus();
  renderCharts();
}

function addPill(target, text) {
  const pill = document.createElement("div");
  pill.className = "meta-pill";
  pill.textContent = text;
  target.appendChild(pill);
}

function renderWarnings() {
  const root = document.getElementById("warnings");
  root.innerHTML = "";
  for (const warning of state.metadata.warnings || []) {
    const el = document.createElement("div");
    el.className = "warning panel";
    el.textContent = warning.source ? `${warning.source}: ${warning.message}` : warning.message;
    root.appendChild(el);
  }
}

function renderMetricGroups() {
  const root = document.getElementById("metric-groups");
  root.innerHTML = "";
  for (const section of state.metadata.sections) {
    const metrics = section.metrics.filter((metric) => matchesFilter(section.name, metric));
    if (!metrics.length) continue;
    const group = document.createElement("section");
    group.className = "metric-group";
    const title = document.createElement("h3");
    title.textContent = section.name;
    group.appendChild(title);
    const checks = document.createElement("div");
    checks.className = "checks";
    for (const metric of metrics) {
      const label = document.createElement("label");
      label.className = "metric";
      const input = document.createElement("input");
      input.type = "checkbox";
      input.checked = state.selected.has(metricKey(section.name, metric.jsonName));
      input.addEventListener("change", () => {
        const key = metricKey(section.name, metric.jsonName);
        if (input.checked) {
          state.selected.add(key);
        } else {
          state.selected.delete(key);
        }
        renderCharts();
      });
      const text = document.createElement("span");
      text.textContent = metric.label;
      label.append(input, text);
      checks.appendChild(label);
    }
    group.appendChild(checks);
    root.appendChild(group);
  }
}

function renderCharts() {
  const root = document.getElementById("charts");
  root.innerHTML = "";
  for (const section of state.metadata.sections) {
    const selectedMetrics = section.metrics.filter((metric) => state.selected.has(metricKey(section.name, metric.jsonName)) && matchesFilter(section.name, metric));
    if (!selectedMetrics.length) continue;
    const block = document.createElement("section");
    block.className = "section-block";
    const title = document.createElement("h2");
    title.textContent = section.name;
    block.appendChild(title);

    const grid = document.createElement("div");
    grid.className = "chart-grid";
    grid.appendChild(sectionChart(section.name, selectedMetrics));
    block.appendChild(grid);
    root.appendChild(block);
  }
  if (!root.children.length) {
    const empty = document.createElement("div");
    empty.className = "empty panel";
    empty.textContent = "No metrics selected.";
    root.appendChild(empty);
  }
}

function sectionChart(sectionName, metrics) {
  const card = document.createElement("article");
  card.className = "chart-card panel";
  const head = document.createElement("div");
  head.className = "chart-head";
  const title = document.createElement("div");
  title.className = "chart-title";
  title.textContent = sectionName;
  const stats = document.createElement("div");
  stats.className = "chart-stats";
  stats.textContent = metrics.map((metric) => metric.label).join(" • ");
  head.append(title, stats);
  card.appendChild(head);

  const viewport = document.createElement("div");
  viewport.className = "chart-viewport";
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 900 240");
  const tooltip = document.createElement("div");
  tooltip.className = "tooltip";
  tooltip.hidden = true;
  viewport.append(svg, tooltip);
  card.appendChild(viewport);

  const series = metrics.map((metric, index) => ({
    metric: { ...metric, sectionName },
    color: palette[index % palette.length],
    points: numericPoints(sectionName, metric)
  })).filter((item) => item.points.length > 0);

  if (!series.length) {
    const empty = document.createElement("div");
    empty.className = "empty";
    empty.textContent = "No numeric data for the selected metrics.";
    card.appendChild(empty);
    return card;
  }

  drawMultiSeriesChart(svg, tooltip, series);
  return card;
}

function drawMultiSeriesChart(svg, tooltip, series) {
  const width = 900;
  const height = 240;
  const padLeft = 14;
  const padRight = 14;
  const padTop = 18;
  const padBottom = 20;
  const plotWidth = width - padLeft - padRight;
  const plotHeight = height - padTop - padBottom;
  const domain = visibleDomain();
  const visibleSeries = series.map((item) => ({
    ...item,
    visiblePoints: item.points.filter((point) => point.ts >= domain.min && point.ts <= domain.max)
  }));
  const allValues = visibleSeries.flatMap((item) => item.visiblePoints.map((point) => point.value));
  if (!allValues.length) {
    svg.innerHTML = "";
    svg.append(
      textNode(width / 2, height / 2, "No data in the selected zoom range.", "middle")
    );
    tooltip.hidden = true;
    return;
  }

  let min = Math.min(...allValues);
  let max = Math.max(...allValues);
  if (min === max) {
    min -= 1;
    max += 1;
  }

  svg.innerHTML = "";
  svg.append(
    lineNode(padLeft, height - padBottom, width - padRight, height - padBottom, "#d4cab6", "2 6"),
    lineNode(padLeft, height / 2, width - padRight, height / 2, "#e4dbc8", "2 8"),
    textNode(width - 6, 14, formatAxis(max), "end"),
    textNode(width - 6, height - 6, formatAxis(min), "end")
  );

  for (const [index, item] of visibleSeries.entries()) {
    const poly = document.createElementNS("http://www.w3.org/2000/svg", "polyline");
    poly.setAttribute("fill", "none");
    poly.setAttribute("stroke", item.color);
    poly.setAttribute("stroke-width", "3");
    poly.setAttribute("stroke-linejoin", "round");
    poly.setAttribute("stroke-linecap", "round");
    poly.setAttribute("points", item.visiblePoints.map((point) => {
      const x = timestampToSvgX(point.ts, domain.min, domain.max, padLeft, plotWidth);
      const y = padTop + (plotHeight * (1 - ((point.value - min) / (max - min))));
      return `${x},${y}`;
    }).join(" "));
    svg.appendChild(poly);

    const label = textNode(padLeft + 4, 16 + index * 13, item.metric.label, "start");
    label.setAttribute("fill", item.color);
    svg.appendChild(label);
  }

  const crosshair = lineNode(0, padTop, 0, height - padBottom, "rgba(28, 35, 46, 0.35)", "");
  crosshair.setAttribute("shape-rendering", "crispEdges");
  crosshair.hidden = true;
  svg.appendChild(crosshair);

  const selection = document.createElementNS("http://www.w3.org/2000/svg", "rect");
  selection.setAttribute("y", String(padTop));
  selection.setAttribute("height", String(plotHeight));
  selection.setAttribute("fill", "rgba(47, 104, 125, 0.16)");
  selection.setAttribute("stroke", "rgba(47, 104, 125, 0.6)");
  selection.setAttribute("stroke-width", "1");
  selection.hidden = true;
  svg.appendChild(selection);

  const overlay = document.createElementNS("http://www.w3.org/2000/svg", "rect");
  overlay.setAttribute("x", String(padLeft));
  overlay.setAttribute("y", String(padTop));
  overlay.setAttribute("width", String(plotWidth));
  overlay.setAttribute("height", String(plotHeight));
  overlay.setAttribute("fill", "transparent");
  overlay.style.cursor = "crosshair";
  svg.appendChild(overlay);

  const interaction = {
    pointerDown: false,
    dragging: false,
    startX: 0
  };

  overlay.addEventListener("mousedown", (event) => {
    event.preventDefault();
    const pointerX = clampPlotX(clientXToSvgX(event, svg), padLeft, plotWidth);
    interaction.pointerDown = true;
    interaction.dragging = false;
    interaction.startX = pointerX;
    selection.hidden = false;
    selection.setAttribute("x", String(pointerX));
    selection.setAttribute("width", "0");
    crosshair.hidden = true;
    tooltip.hidden = true;
  });

  overlay.addEventListener("mousemove", (event) => {
    const rect = svg.getBoundingClientRect();
    const pointerX = clampPlotX(clientXToSvgX(event, svg), padLeft, plotWidth);

    if (interaction.pointerDown) {
      const left = Math.min(interaction.startX, pointerX);
      const dragWidth = Math.abs(pointerX - interaction.startX);
      interaction.dragging = dragWidth >= 4;
      selection.hidden = false;
      selection.setAttribute("x", String(left));
      selection.setAttribute("width", String(dragWidth));
      crosshair.hidden = true;
      tooltip.hidden = true;
      return;
    }

    const hoverTs = svgXToTimestamp(pointerX, domain.min, domain.max, padLeft, plotWidth);
    const hoverIndex = nearestTimelineIndex(hoverTs, domain.min, domain.max);
    if (hoverIndex < 0) {
      crosshair.hidden = true;
      tooltip.hidden = true;
      return;
    }

    crosshair.hidden = false;
    crosshair.setAttribute("x1", String(pointerX));
    crosshair.setAttribute("x2", String(pointerX));

    const row = state.data.rows[hoverIndex];
    const lines = [formatTooltipTime(row.datetime)];
    for (const item of series) {
      const section = row[item.metric.sectionName] || {};
      const value = section[item.metric.jsonName];
      if (typeof value !== "number" || Number.isNaN(value)) continue;
      lines.push(`${item.metric.label}: ${formatValue(value, item.metric.format)}`);
    }
    tooltip.hidden = false;
    tooltip.textContent = lines.join("\n");
    tooltip.style.left = `${Math.min(rect.width - 190, Math.max(8, event.clientX - rect.left + 14))}px`;
    tooltip.style.top = `${Math.max(8, event.clientY - rect.top - 12)}px`;
  });

  const finishSelection = (event) => {
    if (!interaction.pointerDown) return;
    const pointerX = clampPlotX(clientXToSvgX(event, svg), padLeft, plotWidth);
    const minX = Math.min(interaction.startX, pointerX);
    const maxX = Math.max(interaction.startX, pointerX);
    interaction.pointerDown = false;
    selection.hidden = true;
    if (!interaction.dragging || Math.abs(maxX - minX) < 4) {
      interaction.dragging = false;
      return;
    }
    interaction.dragging = false;
    const minTs = svgXToTimestamp(minX, domain.min, domain.max, padLeft, plotWidth);
    const maxTs = svgXToTimestamp(maxX, domain.min, domain.max, padLeft, plotWidth);
    setZoomRange(minTs, maxTs);
  };

  overlay.addEventListener("mouseup", finishSelection);
  overlay.addEventListener("mouseleave", (event) => {
    if (interaction.pointerDown) {
      finishSelection(event);
      return;
    }
    crosshair.hidden = true;
    tooltip.hidden = true;
  });
}

function numericPoints(sectionName, metric) {
  const points = [];
  for (const row of state.data.rows) {
    if (!Number.isFinite(row.__ts)) continue;
    const section = row[sectionName] || {};
    const value = section[metric.jsonName];
    if (typeof value !== "number" || Number.isNaN(value)) continue;
    points.push({ ts: row.__ts, value });
  }
  return points;
}

function visibleDomain() {
  return {
    min: state.visibleTimeMin ?? state.fullTimeMin ?? 0,
    max: state.visibleTimeMax ?? state.fullTimeMax ?? 1
  };
}

function clientXToSvgX(event, svg) {
  const point = svg.createSVGPoint();
  point.x = event.clientX;
  point.y = event.clientY;
  const ctm = svg.getScreenCTM();
  if (!ctm) return 0;
  return point.matrixTransform(ctm.inverse()).x;
}

function clampPlotX(x, plotLeft, plotWidth) {
  return Math.max(plotLeft, Math.min(plotLeft + plotWidth, x));
}

function svgXToTimestamp(svgX, visibleTimeMin, visibleTimeMax, plotLeft, plotWidth) {
  const ratio = plotWidth <= 0 ? 0 : (svgX - plotLeft) / plotWidth;
  return visibleTimeMin + (Math.max(0, Math.min(1, ratio)) * (visibleTimeMax - visibleTimeMin));
}

function timestampToSvgX(timestamp, visibleTimeMin, visibleTimeMax, plotLeft, plotWidth) {
  if (visibleTimeMax <= visibleTimeMin) return plotLeft;
  const ratio = (timestamp - visibleTimeMin) / (visibleTimeMax - visibleTimeMin);
  return plotLeft + (Math.max(0, Math.min(1, ratio)) * plotWidth);
}

function nearestTimelineIndex(timestamp, minTs, maxTs) {
  let bestIndex = -1;
  let bestDistance = Infinity;
  for (let index = 0; index < state.data.rows.length; index += 1) {
    const rowTs = state.data.rows[index].__ts;
    if (!Number.isFinite(rowTs)) continue;
    if (rowTs < minTs || rowTs > maxTs) continue;
    const distance = Math.abs(rowTs - timestamp);
    if (distance < bestDistance) {
      bestDistance = distance;
      bestIndex = index;
    }
  }
  return bestIndex;
}

function metricKey(sectionName, jsonName) {
  return `${sectionName}:${jsonName}`;
}

function matchesFilter(sectionName, metric) {
  if (!state.filter) return true;
  return sectionName.toLowerCase().includes(state.filter) || metric.label.toLowerCase().includes(state.filter);
}

function formatRange(range) {
  if (!range.from && !range.to) return "full capture";
  if (range.from && range.to) return `${range.from} to ${range.to}`;
  return range.from ? `from ${range.from}` : `to ${range.to}`;
}

function formatValue(value, format) {
  switch (format) {
    case "integer":
    case "bool":
      return value.toFixed(0);
    case "seconds":
      return value.toFixed(3);
    default:
      return value.toFixed(1);
  }
}

function formatAxis(value) {
  if (Math.abs(value) >= 1000 || (Math.abs(value) > 0 && Math.abs(value) < 0.1)) return value.toExponential(1);
  return value.toFixed(1);
}

function formatTooltipTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  const year = date.getUTCFullYear();
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const day = String(date.getUTCDate()).padStart(2, "0");
  const hour = String(date.getUTCHours()).padStart(2, "0");
  const minute = String(date.getUTCMinutes()).padStart(2, "0");
  const second = String(date.getUTCSeconds()).padStart(2, "0");
  return `${year}-${month}-${day} ${hour}:${minute}:${second} UTC`;
}

function lineNode(x1, y1, x2, y2, stroke, dash) {
  const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
  line.setAttribute("x1", x1);
  line.setAttribute("y1", y1);
  line.setAttribute("x2", x2);
  line.setAttribute("y2", y2);
  line.setAttribute("stroke", stroke);
  line.setAttribute("stroke-width", "1");
  if (dash) line.setAttribute("stroke-dasharray", dash);
  return line;
}

function textNode(x, y, value, anchor) {
  const text = document.createElementNS("http://www.w3.org/2000/svg", "text");
  text.setAttribute("x", x);
  text.setAttribute("y", y);
  text.setAttribute("fill", "#5c6675");
  text.setAttribute("font-size", "11");
  text.setAttribute("text-anchor", anchor);
  text.setAttribute("font-family", "SFMono-Regular, Consolas, Menlo, monospace");
  text.textContent = value;
  return text;
}
