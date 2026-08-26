/* ==========================================================================
   HYDRASTREAM INSPECTION DRAWER MODULE
   ========================================================================== */

import { state } from '../state.js';
import { updateConsumerFPSAPI } from '../api.js';

export function openStreamDrawer(streamId) {
  state.activeDrawerStreamId = streamId;
  const st = (state.rawStreams && state.rawStreams.length > 0)
    ? state.rawStreams.find(s => s.stream_id === streamId) 
    : state.sampleStreams[0];

  if (!st) return;

  const elTitle = document.getElementById('drawerStreamTitle');
  if (elTitle) elTitle.innerText = `// ${st.stream_id.toUpperCase()}`;

  const elTenant = document.getElementById('drawerTenantSubtitle');
  if (elTenant) elTenant.innerText = `TENANT: ${st.tenant_id}`;

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
    closeDrawer();
  } else {
    alert(`[HYDRASTREAM] Updated locally (Mock Server Mode): ${fps} FPS (${fmt})`);
    closeDrawer();
  }
}
