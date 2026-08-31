/* ==========================================================================
   HYDRASTREAM TABLE RENDERER MODULE (LIVE BACKEND STREAM FEED)
   ========================================================================== */

import { state } from '../state.js';
import { fetchStreamsAPI } from '../api.js';

export function getThresholdColor(val, metricType) {
  const num = parseFloat(val) || 0;
  if (metricType === 'latency') return num > 3.5 ? '#f43f5e' : (num > 2.0 ? '#fbbf24' : '#34d399');
  if (metricType === 'resource_score') return num > 90.0 ? '#f43f5e' : (num > 75.0 ? '#fbbf24' : '#a855f7');
  if (metricType === 'network_flow') return num < 500 ? '#f43f5e' : (num < 2000 ? '#fbbf24' : '#38bdf8');
  return '#38bdf8';
}

export async function renderStreamTable() {
  const tbody = document.getElementById('streamTableBody');
  if (!tbody) return;

  const tenantFilter = state.currentTabFilter === 'all' ? '' : state.currentTabFilter;
  const data = await fetchStreamsAPI(state.searchQuery, tenantFilter, state.sortBy, state.currentPage, state.pageSize);
  const streams = (data && data.streams) ? data.streams : [];
  state.rawStreams = streams;
  const totalItems = (data && typeof data.total_count === 'number') ? data.total_count : streams.length;
  const totalPages = Math.ceil(totalItems / state.pageSize) || 1;

  if (state.currentPage > totalPages && totalPages > 0) state.currentPage = totalPages;

  const startIndex = (state.currentPage - 1) * state.pageSize;
  const endIndex = Math.min(startIndex + state.pageSize, totalItems);

  const setTxt = (id, txt) => { const el = document.getElementById(id); if (el) el.innerText = txt; };
  setTxt('pageRangeText', totalItems > 0 ? `${startIndex + 1}-${endIndex}` : '0-0');
  setTxt('totalItemsText', totalItems);
  setTxt('currentPageText', state.currentPage);
  setTxt('totalPagesText', totalPages);

  const btnPrev = document.getElementById('btnPrevPage');
  if (btnPrev) btnPrev.disabled = (state.currentPage <= 1);
  const btnNext = document.getElementById('btnNextPage');
  if (btnNext) btnNext.disabled = (state.currentPage >= totalPages);

  if (streams.length === 0) {
    tbody.innerHTML = '<tr><td colspan="8" class="text-mono" style="color: var(--cb-text-muted); text-align: center; padding: 2rem;">// NO ACTIVE PIPELINES REGISTERED IN BACKEND</td></tr>';
    return;
  }

  tbody.innerHTML = streams.map(st => {
    const isRTSP = st.source_url.startsWith('rtsp://');
    const isMP4 = st.source_url.startsWith('file://') || st.source_url.endsWith('.mp4');
    const badge = isRTSP ? '<span class="badge-protocol">RTSP LIVE</span>' : (isMP4 ? '<span class="badge-protocol mp4">MP4 LOOP</span>' : '<span class="badge-protocol imageseq">SYNTHETIC SHM</span>');
    const consumers = Array.isArray(st.consumers) && st.consumers.length > 0 ? st.consumers.map(c => `${c.analytic_type} @ ${c.target_fps} FPS`).join(' | ') : 'NONE';
    const score = st.resource_score || 80.0;
    const netKbps = st.network_kbps || 4850;
    const latency = st.decode_latency_ms || 1.42;

    return `
      <tr onclick="window.openStreamDrawer('${st.stream_id}')" title="Click to inspect Media Source Parameters">
        <td><div class="cell-id">${st.stream_id.toUpperCase()}</div><span class="cell-tenant">${st.tenant_id}</span></td>
        <td>${badge}<div class="cell-mono" style="margin-top: 0.2rem;">${st.source_url}</div></td>
        <td><span class="cell-metric" style="color: ${getThresholdColor(netKbps, 'network_flow')};">${netKbps.toLocaleString()} Kbps</span></td>
        <td><span class="cell-metric" style="color: ${getThresholdColor(latency, 'latency')};">${latency.toFixed(2)} ms</span></td>
        <td><span class="cell-metric" style="color: ${getThresholdColor(score, 'resource_score')};">${score.toFixed(1)} pts</span></td>
        <td><span class="cell-mono" style="font-size: 0.8rem;">${consumers}</span></td>
        <td><span class="cell-transport">${st.decoding_engine || 'POSIX SHM'}</span></td>
        <td><span class="status-badge">${st.status || 'ONLINE'}</span></td>
      </tr>
    `;
  }).join('');
}

export function changePage(delta) {
  state.currentPage += delta;
  renderStreamTable();
}
