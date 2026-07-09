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
  const headRow = document.createElement("tr");

  for (const col of tableData.columns) {
    const th = document.createElement("th");
    th.textContent = col.label;
    if (col.section) {
      th.title = col.section;
    }
    headRow.appendChild(th);
  }

  thead.appendChild(headRow);
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
