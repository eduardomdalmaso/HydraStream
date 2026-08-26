/* ==========================================================================
   HYDRASTREAM TABLE RENDERER MODULE
   ========================================================================== */

import { state, getFilteredStreams } from '../state.js';

export function getThresholdColor(val, metricType) {
  const num = parseFloat(val) || 0;
  if (metricType === 'latency') return num > 3.5 ? '#f43f5e' : (num > 2.0 ? '#fbbf24' : '#34d399');
  if (metricType === 'resource_score') return num > 90.0 ? '#f43f5e' : (num > 75.0 ? '#fbbf24' : '#a855f7');
  if (metricType === 'network_flow') return num < 500 ? '#f43f5e' : (num < 2000 ? '#fbbf24' : '#38bdf8');
  return '#38bdf8';
}

export function renderStreamTable() {
  const tbody = document.getElementById('streamTableBody');
  if (!tbody) return;

  const filtered = getFilteredStreams();
  const totalItems = filtered.length;
  const totalPages = Math.ceil(totalItems / state.pageSize) || 1;
  if (state.currentPage > totalPages) state.currentPage = totalPages;

  const startIndex = (state.currentPage - 1) * state.pageSize;
  const endIndex = Math.min(startIndex + state.pageSize, totalItems);
  const pageItems = filtered.slice(startIndex, endIndex);

  const elRange = document.getElementById('pageRangeText');
  if (elRange) elRange.innerText = totalItems > 0 ? `${startIndex + 1}-${endIndex}` : '0-0';
  const elTotal = document.getElementById('totalItemsText');
  if (elTotal) elTotal.innerText = totalItems;
  const elCurrent = document.getElementById('currentPageText');
  if (elCurrent) elCurrent.innerText = state.currentPage;
  const elTotalP = document.getElementById('totalPagesText');
  if (elTotalP) elTotalP.innerText = totalPages;

  const btnPrev = document.getElementById('btnPrevPage');
  if (btnPrev) btnPrev.disabled = (state.currentPage <= 1);
  const btnNext = document.getElementById('btnNextPage');
  if (btnNext) btnNext.disabled = (state.currentPage >= totalPages);

  if (!pageItems || pageItems.length === 0) {
    tbody.innerHTML = '<tr><td colspan="8" class="text-mono" style="color: var(--cb-text-muted); text-align: center;">// NO INPUT SOURCES MATCH FILTER</td></tr>';
    return;
  }

  tbody.innerHTML = pageItems.map(st => {
    const protocolBadge = st.type === 'rtsp' 
      ? '<span class="badge-protocol">RTSP LIVE</span>' 
      : (st.type === 'mp4' ? '<span class="badge-protocol mp4">MP4 LOOP</span>' : '<span class="badge-protocol imageseq">IMAGE SEQ</span>');

    const consumersText = Array.isArray(st.consumers) 
      ? st.consumers.map(c => typeof c === 'string' ? c : `${c.analytic_type} @ ${c.target_fps} FPS`).join(' | ') 
      : 'NONE';

    const resScoreVal = st.resource_score || 80;
    const netKbpsVal = st.network_kbps || 4850;

    return `
      <tr onclick="window.openStreamDrawer('${st.stream_id}')" title="Click to inspect Media Source Parameters">
        <td><div class="cell-id">${st.stream_id.toUpperCase()}</div><span class="cell-tenant">${st.tenant_id}</span></td>
        <td>${protocolBadge}<div class="cell-mono" style="margin-top: 0.2rem;">${st.source_url}</div></td>
        <td><span class="cell-metric" style="color: ${getThresholdColor(netKbpsVal, 'network_flow')};">${netKbpsVal.toLocaleString()} Kbps</span></td>
        <td><span class="cell-metric" style="color: ${getThresholdColor(st.latency || 1.42, 'latency')};">${st.latency || '1.42 ms'}</span></td>
        <td><span class="cell-metric" style="color: ${getThresholdColor(resScoreVal, 'resource_score')};">${resScoreVal.toFixed(1)} pts</span></td>
        <td><span class="cell-mono" style="font-size: 0.8rem;">${consumersText}</span></td>
        <td><span class="cell-transport">${st.transport || 'POSIX SHM'}</span></td>
        <td><span class="status-badge">${st.status || 'ONLINE'}</span></td>
      </tr>
    `;
  }).join('');
}

export function changePage(delta) {
  state.currentPage += delta;
  renderStreamTable();
}
