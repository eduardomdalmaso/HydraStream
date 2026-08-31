/* ==========================================================================
   HYDRASTREAM INSPECTION DRAWER MODULE (LIVE STREAM INSPECTION)
   ========================================================================== */

import { state } from '../state.js';
import { updateConsumerFPSAPI } from '../api.js';

export function openStreamDrawer(streamId) {
  state.activeDrawerStreamId = streamId;
  const st = (state.rawStreams || []).find(s => s.stream_id === streamId);
  if (!st) return;

  const setTxt = (id, txt) => { const el = document.getElementById(id); if (el) el.innerText = txt; };
  setTxt('drawerStreamTitle', `// ${st.stream_id.toUpperCase()}`);
  setTxt('drawerTenantSubtitle', `TENANT: ${st.tenant_id}`);

  const elUrl = document.getElementById('drawerSourceUrl');
  if (elUrl) elUrl.value = st.source_url;

  const drawerOverlay = document.getElementById('drawerOverlay');
  if (drawerOverlay) drawerOverlay.style.display = 'flex';
}

export function closeDrawer() {
  const drawerOverlay = document.getElementById('drawerOverlay');
  if (drawerOverlay) drawerOverlay.style.display = 'none';
}

export async function saveConsumerSettings() {
  const fpsInput = document.getElementById('drawerFpsInput');
  const fmtSelect = document.getElementById('drawerFormatSelect');
  if (!fpsInput || !fmtSelect || !state.activeDrawerStreamId) return;

  const fps = parseFloat(fpsInput.value) || 2.0;
  const fmt = fmtSelect.value;

  const success = await updateConsumerFPSAPI(state.activeDrawerStreamId, 'lpr_ocr', fps, fmt);
  if (success) {
    alert(`[HYDRASTREAM] Updated Consumer Sampling for ${state.activeDrawerStreamId}: ${fps} FPS (${fmt})`);
  } else {
    alert(`[HYDRASTREAM] Failed to update consumer settings on backend.`);
  }
  closeDrawer();
}
