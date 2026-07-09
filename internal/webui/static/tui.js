async function main() {
  const res = await fetch("/api/table");
  if (!res.ok) {
    throw new Error(`failed to load table: ${res.status}`);
  }

  const tableData = await res.json();
  renderTable(tableData);
  setupKeys();

  document.getElementById("status").textContent =
    `${tableData.rows.length} rows, ${tableData.columns.length} columns`;
}

function renderTable(tableData) {
  const table = document.getElementById("metrics-table");
  table.textContent = "";

  const thead = document.createElement("thead");
  const header = describeHeader(tableData.columns);
  if (header.hasSections) {
    thead.appendChild(buildSectionRow(tableData.columns, header));
    if (header.hasSubsections) {
      thead.appendChild(buildSubsectionRow(tableData.columns, header));
    }
  }
  thead.appendChild(buildLabelRow(tableData.columns, header));
  table.appendChild(thead);

  const tbody = document.createElement("tbody");
  for (const row of tableData.rows) {
    const tr = document.createElement("tr");
    for (const value of row.cells) {
      const td = document.createElement("td");
      td.textContent = value;
      tr.appendChild(td);
    }
    tbody.appendChild(tr);
  }
  table.appendChild(tbody);
}

function describeHeader(columns) {
  const leadFixedCount = countLeadingFixedColumns(columns);
  const metricColumns = columns.slice(leadFixedCount);
  const hasSections = metricColumns.some((col) => col.section);
  const hasSubsections = metricColumns.some((col) => col.subsection);
  const rowSpan = hasSections ? (hasSubsections ? 3 : 2) : 1;
  return { leadFixedCount, hasSections, hasSubsections, rowSpan };
}

function countLeadingFixedColumns(columns) {
  let count = 0;
  while (count < columns.length && columns[count].fixed) {
    count += 1;
  }
  return count;
}

function buildSectionRow(columns, header) {
  const row = document.createElement("tr");
  row.className = "section-row";

  for (const col of columns.slice(0, header.leadFixedCount)) {
    const th = document.createElement("th");
    th.textContent = col.label;
    th.rowSpan = header.rowSpan;
    th.classList.add("sticky-col", "section-cell");
    row.appendChild(th);
  }

  for (const group of collapseGroups(columns.slice(header.leadFixedCount), (col) => col.section || col.label)) {
    const th = document.createElement("th");
    th.textContent = group.label;
    th.colSpan = group.span;
    th.classList.add("section-cell");
    row.appendChild(th);
  }
  return row;
}

function buildSubsectionRow(columns, header) {
  const row = document.createElement("tr");
  row.className = "subsection-row";
  for (const group of collapseGroups(columns.slice(header.leadFixedCount), (col) => col.subsection || col.section || col.label)) {
    const th = document.createElement("th");
    th.textContent = group.label;
    th.colSpan = group.span;
    row.appendChild(th);
  }
  return row;
}

function buildLabelRow(columns, header) {
  const row = document.createElement("tr");
  row.className = "metric-row";
  const start = header.hasSections ? header.leadFixedCount : 0;
  for (const col of columns.slice(start)) {
    const th = document.createElement("th");
    th.textContent = col.label;
    if (col.section) {
      th.title = col.subsection ? `${col.section} / ${col.subsection}` : col.section;
    }
    row.appendChild(th);
  }
  return row;
}

function collapseGroups(columns, labelFor) {
  const groups = [];
  for (let index = 0; index < columns.length; index += 1) {
    const label = labelFor(columns[index]);
    const prev = groups[groups.length - 1];
    if (prev && prev.label === label) {
      prev.span += 1;
      continue;
    }
    groups.push({ label, span: 1, start: index });
  }
  return groups;
}

function setupKeys() {
  const wrap = document.getElementById("table-wrap");

  document.addEventListener("keydown", (event) => {
    switch (event.key) {
      case "h":
      case "ArrowLeft":
        wrap.scrollLeft -= 120;
        event.preventDefault();
        break;
      case "l":
      case "ArrowRight":
        wrap.scrollLeft += 120;
        event.preventDefault();
        break;
      case "j":
      case "ArrowDown":
        wrap.scrollTop += 24;
        event.preventDefault();
        break;
      case "k":
      case "ArrowUp":
        wrap.scrollTop -= 24;
        event.preventDefault();
        break;
      case "g":
        wrap.scrollTop = 0;
        event.preventDefault();
        break;
      case "G":
        wrap.scrollTop = wrap.scrollHeight;
        event.preventDefault();
        break;
    }
  });
}

main().catch((err) => {
  document.body.textContent = err.message;
});
