/* ==========================================================================
   HYDRASTREAM CONTROL PANEL VIEW MODULE
   ========================================================================== */

import { fetchSystemInfoAPI } from '../api.js';

export async function initControlPanel() {
  const info = await fetchSystemInfoAPI();
  if (info) {
    const elUptime = document.getElementById('hudUptimeText');
    if (elUptime) elUptime.innerText = `${info.uptime_seconds}s UPTIME`;
  }
}
