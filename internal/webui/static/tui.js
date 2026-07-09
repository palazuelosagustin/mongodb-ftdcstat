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
  const hasSubsections = tableData.columns.some((col) => col.subsection);
  if (hasSubsections) {
    thead.appendChild(buildSectionRow(tableData.columns));
    thead.appendChild(buildSubsectionRow(tableData.columns));
  }
  thead.appendChild(buildLabelRow(tableData.columns));
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

function buildSectionRow(columns) {
  const row = document.createElement("tr");
  for (const group of collapseGroups(columns, (col) => col.section || col.label)) {
    const th = document.createElement("th");
    th.textContent = group.label;
    th.colSpan = group.span;
    if (group.start === 0) {
      th.classList.add("sticky-col");
      th.rowSpan = 2;
    }
    row.appendChild(th);
  }
  return row;
}

function buildSubsectionRow(columns) {
  const row = document.createElement("tr");
  for (const group of collapseGroups(columns.slice(1), (col) => col.subsection || col.section || col.label)) {
    const th = document.createElement("th");
    th.textContent = group.label;
    th.colSpan = group.span;
    row.appendChild(th);
  }
  return row;
}

function buildLabelRow(columns) {
  const row = document.createElement("tr");
  for (const col of columns) {
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
