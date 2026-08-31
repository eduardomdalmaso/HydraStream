/* ==========================================================================
   HYDRASTREAM CONTROL PANEL VIEW MODULE (LIVE CYBERPUNK TELEMETRY FEED)
   ========================================================================== */

import { fetchControlPanelTelemetryAPI, fetchTopologyAPI } from '../api.js';

let telemetryInterval = null;

function renderBandwidthChart(hist) {
  const box = document.getElementById('ctrlBandwidthSvgBox');
  if (!box || !hist || hist.length === 0) return;
  const maxVal = Math.max(...hist, 80), w = 500, h = 160, step = w / (hist.length - 1);
  const pts = hist.map((v, i) => ({ x: Math.round(i * step), y: Math.round(h - (v / maxVal) * (h - 40) - 20) }));

  let d = `M ${pts[0].x},${pts[0].y}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i === 0 ? 0 : i - 1], p1 = pts[i], p2 = pts[i + 1], p3 = pts[i + 2] || p2;
    d += ` C ${(p1.x + (p2.x - p0.x) / 6).toFixed(1)},${(p1.y + (p2.y - p0.y) / 6).toFixed(1)} ${(p2.x - (p3.x - p1.x) / 6).toFixed(1)},${(p2.y - (p3.y - p1.y) / 6).toFixed(1)} ${p2.x},${p2.y}`;
  }
  const last = pts[pts.length - 1], val = hist[hist.length - 1];

  box.innerHTML = `
    <svg viewBox="0 0 500 160" style="width:100%;height:100%;" preserveAspectRatio="none">
      <defs><linearGradient id="cyberBwGrad" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="#00f0ff" stop-opacity="0.38"/><stop offset="100%" stop-color="#00f0ff" stop-opacity="0.0"/></linearGradient></defs>
      <line x1="0" y1="35" x2="500" y2="35" class="chart-grid-line"/><line x1="0" y1="75" x2="500" y2="75" class="chart-grid-line"/><line x1="0" y1="115" x2="500" y2="115" class="chart-grid-line"/>
      <text x="8" y="32" class="chart-axis-label">80M</text><text x="8" y="72" class="chart-axis-label">40M</text><text x="8" y="112" class="chart-axis-label">20M</text>
      <path d="${d} L ${w},${h} L 0,${h} Z" fill="url(#cyberBwGrad)"/><path d="${d}" fill="none" stroke="rgba(0, 240, 255, 0.35)" stroke-width="5"/><path d="${d}" fill="none" stroke="#00f0ff" stroke-width="2" class="chart-glow-line"/>
      <circle cx="${last.x}" cy="${last.y}" r="4" fill="#fcee0a" stroke="#00f0ff" stroke-width="2" class="hud-pulse"/>
      <text x="${Math.max(10, last.x - 65)}" y="${Math.max(18, last.y - 10)}" font-family="var(--font-mono)" font-size="11" font-weight="bold" fill="#fcee0a">${val.toFixed(1)}M</text>
    </svg>
  `;
}

function renderLatencyChart(hist) {
  const box = document.getElementById('ctrlLatencySvgBox');
  if (!box || !hist || hist.length === 0) return;
  const maxVal = Math.max(...hist, 2.5);
  const items = hist.map((v, i) => {
    const x = 18 + i * 68, h = Math.max(12, Math.round((v / maxVal) * 95)), y = 138 - h;
    const col = v > 2.0 ? 'var(--cb-magenta)' : v > 1.6 ? 'var(--cb-yellow)' : 'var(--cb-cyan)';
    const tag = i === hist.length - 1 ? 'LIVE' : `T-${hist.length - 1 - i}`;
    return `<rect x="${x}" y="${y}" width="42" height="${h}" fill="${col}" opacity="0.85" rx="3"/><text x="${x + 21}" y="${y - 4}" text-anchor="middle" font-family="var(--font-mono)" font-size="10" font-weight="bold" fill="#fff">${v.toFixed(2)}ms</text><text x="${x + 21}" y="152" text-anchor="middle" font-family="var(--font-mono)" font-size="9" fill="#94a3b8">${tag}</text>`;
  }).join('');

  box.innerHTML = `
    <svg viewBox="0 0 500 160" style="width:100%;height:100%;" preserveAspectRatio="none">
      <line x1="0" y1="80" x2="500" y2="80" stroke="rgba(252, 238, 10, 0.4)" stroke-dasharray="3 3"/>
      <text x="8" y="76" font-family="var(--font-mono)" font-size="9" fill="#fcee0a">SLA TARGET (1.50ms)</text>
      ${items}
    </svg>
  `;
}

export async function refreshControlPanel() {
  const stats = await fetchControlPanelTelemetryAPI();
  if (stats) {
    const setTxt = (id, txt) => { const el = document.getElementById(id); if (el) el.innerText = txt; };
    setTxt('ctrlHealthScore', `${stats.health_score.toFixed(2)} %`);
    setTxt('ctrlActiveNodes', stats.active_cluster_nodes);
    setTxt('ctrlNodesSummary', stats.nodes_summary);
    setTxt('ctrlDecodeLatency', `${stats.avg_decode_latency_ms.toFixed(2)} ms`);
    setTxt('ctrlDecoderEngine', stats.decoder_engine_name);
    setTxt('ctrlShmOccupancy', `${stats.posix_shm_occupancy.toFixed(1)} %`);
    setTxt('ctrlShmStatus', stats.shm_lock_free_status);
    setTxt('ctrlBandwidthPeak', `PEAK: ${stats.peak_bandwidth_mbps.toFixed(1)} Mbps`);
    setTxt('ctrlLatencyAvg', `AVG: ${stats.avg_decode_latency_ms.toFixed(2)} ms`);

    const elSla = document.getElementById('ctrlSlaBadge');
    if (elSla) {
      elSla.innerText = `● ${stats.sla_status}`;
      elSla.style.color = stats.health_score < 90 ? 'var(--cb-magenta)' : stats.health_score < 99 ? 'var(--cb-yellow)' : 'var(--cb-green)';
    }

    renderBandwidthChart(stats.bandwidth_history);
    renderLatencyChart(stats.latency_history);
  }

  const topo = await fetchTopologyAPI();
  const tbody = document.getElementById('ctrlTopologyTableBody');
  if (tbody && topo && topo.nodes) {
    tbody.innerHTML = topo.nodes.map(n => `
      <tr>
        <td class="cell-id">${n.node_name}</td>
        <td class="cell-mono">${n.node_ip}</td>
        <td class="cell-mono">${n.cpu_architecture}</td>
        <td class="cell-hardware">${n.gpu_hardware}</td>
        <td class="cell-engine">${n.decoder_engine}</td>
        <td><span class="status-badge">${n.status}</span></td>
      </tr>
    `).join('');
  }
}

export function initControlPanel() {
  refreshControlPanel();
  if (!telemetryInterval) telemetryInterval = setInterval(refreshControlPanel, 1500);
}
