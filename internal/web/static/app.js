const state = { window: "24h", selected: null };
const panel = document.getElementById("panel");
const css = getComputedStyle(document.documentElement);
const color = (name) => css.getPropertyValue(name).trim();

const cy = cytoscape({
  container: document.getElementById("graph"),
  wheelSensitivity: 0.3,
  style: [
    {
      selector: "node",
      style: {
        label: "data(name)",
        color: color("--text"),
        "font-size": 11,
        "text-valign": "bottom",
        "text-margin-y": 4,
        width: (n) => 22 + 5 * n.degree(),
        height: (n) => 22 + 5 * n.degree(),
        "background-color": (n) => impactColor(n.data("impact")),
        "border-width": 2,
        "border-color": color("--bg"),
      },
    },
    {
      selector: "node[events > 0][sources < 2]",
      style: { "border-color": color("--warn"), "border-style": "dashed" },
    },
    {
      selector: "edge",
      style: {
        width: (e) => 1 + 5 * e.data("weight"),
        "line-color": color("--border"),
        "target-arrow-color": color("--border"),
        "target-arrow-shape": "triangle",
        "curve-style": "bezier",
        opacity: 0.9,
      },
    },
    { selector: "edge[?exclusive]", style: { "line-color": "#3d4a63", "target-arrow-color": "#3d4a63" } },
    { selector: "edge[status = 'proposed']", style: { "line-style": "dashed", "line-color": color("--warn"), "target-arrow-color": color("--warn") } },
    { selector: ":selected", style: { "border-color": color("--accent"), "line-color": color("--accent"), "target-arrow-color": color("--accent") } },
  ],
});

function impactColor(impact) {
  if (Math.abs(impact) < 0.05) return color("--neutral");
  const mix = Math.min(1, Math.abs(impact));
  const target = impact > 0 ? [47, 191, 113] : [229, 72, 77];
  const base = [107, 114, 128];
  const rgb = base.map((b, i) => Math.round(b + (target[i] - b) * (0.35 + 0.65 * mix)));
  return `rgb(${rgb.join(",")})`;
}

function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (key.startsWith("on")) el.addEventListener(key.slice(2), value);
    else el.setAttribute(key, value);
  }
  el.append(...children.flat().filter((c) => c !== null && c !== undefined && c !== false));
  return el;
}

async function api(path, body) {
  const res = await fetch(path, body ? {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  } : undefined);
  if (!res.ok) throw new Error(`${res.status} ${path}`);
  return res.json();
}

async function loadGraph() {
  const view = await api(`/api/graph?window=${state.window}`);
  let changed = false;
  const keep = new Set();

  for (const node of view.nodes || []) {
    keep.add(node.id);
    changed = upsert({ group: "nodes", data: node }) || changed;
  }
  for (const edge of view.edges || []) {
    const id = `e${edge.id}`;
    keep.add(id);
    changed = upsert({ group: "edges", data: { ...edge, id, source: edge.from, target: edge.to } }) || changed;
  }
  cy.elements().filter((el) => !keep.has(el.id())).forEach((el) => { el.remove(); changed = true; });

  if (changed) cy.layout({ name: "cose", animate: false, nodeRepulsion: () => 9000, idealEdgeLength: () => 90 }).run();
  if (state.selected) renderSelected();
}

function upsert(element) {
  const existing = cy.getElementById(element.data.id);
  if (existing.nonempty()) {
    existing.data(element.data);
    return false;
  }
  cy.add(element);
  return true;
}

let pending;
function scheduleReload() {
  clearTimeout(pending);
  pending = setTimeout(loadGraph, 400);
}

function connect() {
  const live = document.getElementById("live");
  const stream = new EventSource("/api/stream");
  stream.addEventListener("ready", () => { live.className = "live"; live.textContent = "ao vivo"; scheduleReload(); });
  stream.addEventListener("graph", scheduleReload);
  stream.onerror = () => { live.className = "live off"; live.textContent = "reconectando"; };
}

function renderSelected() {
  const el = cy.getElementById(state.selected);
  if (el.empty()) return renderLegend();
  return el.isNode() ? renderCompany(el.data()) : renderEdge(el.data());
}

function renderLegend() {
  state.selected = null;
  panel.replaceChildren(
    h("h2", {}, "Grafo de dependências"),
    h("p", { class: "muted" }, "Clique numa empresa para ver as notícias, ou numa aresta para ver a relação."),
    h("div", { class: "legend" },
      h("div", {}, h("span", { style: `background:${color("--up")}` }), "impacto positivo"),
      h("div", {}, h("span", { style: `background:${color("--down")}` }), "impacto negativo"),
      h("div", {}, h("span", { style: `background:${color("--neutral")}` }), "sem notícias na janela"),
      h("div", {}, h("span", { style: `border:2px dashed ${color("--warn")}` }), "não confirmado (1 fonte)"),
      h("div", {}, "Aresta tracejada amarela: relação proposta pela IA"),
      h("div", {}, "Seta: fornecedor → cliente; espessura = dependência"),
    ),
  );
}

async function renderCompany(node) {
  const events = (await api(`/api/companies/${encodeURIComponent(node.id)}/events?window=${state.window}`)) || [];
  if (state.selected !== node.id) return;

  const impactClass = node.impact > 0.05 ? "up" : node.impact < -0.05 ? "down" : "muted";
  panel.replaceChildren(
    h("h2", {}, node.name),
    h("div", { class: "muted" }, [node.country, node.sector].filter(Boolean).join(" · ")),
    node.events > 0 && node.sources < 2 ? h("span", { class: "badge warn" }, "não confirmado") : null,
    h("div", { class: "stats" },
      h("div", {}, h("b", { class: impactClass }, node.impact.toFixed(2)), h("span", { class: "muted" }, "impacto")),
      h("div", {}, h("b", {}, String(node.events)), h("span", { class: "muted" }, "eventos")),
      h("div", {}, h("b", {}, String(node.sources)), h("span", { class: "muted" }, "fontes")),
    ),
    h("h3", {}, "Notícias"),
    events.length ? events.map((ev) => renderEvent(ev, node.id)) : h("p", { class: "muted" }, "Nenhuma notícia nesta janela."),
  );
}

function renderEvent(ev, companyID) {
  const arrow = ev.direction > 0 ? h("span", { class: "up" }, "▲") : ev.direction < 0 ? h("span", { class: "down" }, "▼") : h("span", { class: "muted" }, "●");
  const label = (body) => async () => {
    await api(`/api/articles/${ev.article_id}/label`, { company: companyID, ...body });
    renderSelected();
  };

  return h("div", { class: "event" },
    h("a", { href: ev.url, target: "_blank", rel: "noopener noreferrer" }, ev.title || ev.url),
    h("div", { class: "meta" },
      arrow, ` ${ev.event_type} · gravidade ${ev.severity} · ${ev.domain} · ${timeAgo(ev.seen_at)}`,
      ev.factual ? "" : " · opinião",
      ev.labeler === "human" ? " · corrigido" : "",
    ),
    ev.reason ? h("div", { class: "reason" }, ev.reason) : null,
    h("div", { class: "actions" },
      h("button", { onclick: label({ valid: true, direction: 1 }) }, "▲ positivo"),
      h("button", { onclick: label({ valid: true, direction: -1 }) }, "▼ negativo"),
      h("button", { onclick: label({ valid: false, direction: ev.direction }) }, "✕ inválido"),
    ),
  );
}

async function renderEdge(edge) {
  const evidence = (await api(`/api/edges/${edge.id.slice(1)}/evidence`)) || [];
  if (state.selected !== edge.id) return;

  const from = cy.getElementById(edge.source).data("name");
  const to = cy.getElementById(edge.target).data("name");
  const setStatus = (status) => async () => {
    await api(`/api/edges/${edge.id.slice(1)}/status`, { status });
    state.selected = null;
    renderLegend();
  };

  panel.replaceChildren(
    h("h2", {}, `${from} → ${to}`),
    h("div", { class: "muted" }, edge.product || "produto não informado"),
    h("div", { class: "stats" },
      h("div", {}, h("b", {}, edge.weight.toFixed(2)), h("span", { class: "muted" }, "dependência")),
      h("div", {}, h("b", {}, edge.exclusive ? "sim" : "não"), h("span", { class: "muted" }, "exclusivo")),
    ),
    edge.status === "proposed" ? h("div", { class: "actions" },
      h("span", { class: "badge warn" }, "proposta pela IA"),
      h("button", { onclick: setStatus("approved") }, "✓ aprovar"),
      h("button", { onclick: setStatus("rejected") }, "✕ rejeitar"),
    ) : null,
    h("h3", {}, "Evidências"),
    evidence.length
      ? evidence.map((ev) => h("div", { class: "event" },
          h("a", { href: ev.url, target: "_blank", rel: "noopener noreferrer" }, ev.title || ev.url),
          h("div", { class: "meta" }, `${ev.domain} · ${timeAgo(ev.seen_at)}`),
        ))
      : h("p", { class: "muted" }, "Relação cadastrada manualmente."),
  );
}

function timeAgo(iso) {
  const minutes = Math.round((Date.now() - new Date(iso)) / 60000);
  if (minutes < 60) return `${minutes} min`;
  if (minutes < 1440) return `${Math.round(minutes / 60)} h`;
  return `${Math.round(minutes / 1440)} d`;
}

cy.on("tap", "node, edge", (e) => { state.selected = e.target.id(); renderSelected(); });
cy.on("tap", (e) => { if (e.target === cy) renderLegend(); });
document.getElementById("window").addEventListener("change", (e) => { state.window = e.target.value; loadGraph(); });

renderLegend();
loadGraph();
connect();
