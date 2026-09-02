/* ==========================================================================
   HYDRASTREAM ONVIF CAMERA SCANNER & PROBE MODAL COMPONENT
   ========================================================================== */

import { discoverONVIFAPI, probeONVIFAPI, importONVIFStreamAPI } from '../api_onvif.js';
import { loadStreams } from '../views/stream-matrix.js';

export function openONVIFModal() {
  const modal = document.getElementById('onvifModalOverlay');
  if (modal) modal.style.display = 'flex';
}

export function closeONVIFModal() {
  const modal = document.getElementById('onvifModalOverlay');
  if (modal) modal.style.display = 'none';
}

export async function handleONVIFScan() {
  const statusEl = document.getElementById('onvifScanStatus');
  const resultsEl = document.getElementById('onvifResultsList');
  if (statusEl) statusEl.innerHTML = '<span class="glow-cyan">🔍 Broadcasting WS-Discovery UDP Probes (239.255.255.250)...</span>';
  if (resultsEl) resultsEl.innerHTML = '';

  const res = await discoverONVIFAPI();
  if (!res || !res.devices || res.devices.length === 0) {
    if (statusEl) statusEl.innerHTML = '<span class="glow-yellow">⚠️ No cameras responded to multicast probe. Use Manual Probe below.</span>';
    return;
  }

  if (statusEl) statusEl.innerHTML = `<span class="glow-green">✓ Found ${res.devices.length} ONVIF Camera(s) on local subnet:</span>`;
  renderONVIFDevices(res.devices);
}

export async function handleManualProbe() {
  const ip = document.getElementById('onvifIpInput')?.value?.trim();
  const port = document.getElementById('onvifPortInput')?.value?.trim() || '80';
  const user = document.getElementById('onvifUserInput')?.value?.trim() || '';
  const pass = document.getElementById('onvifPassInput')?.value || '';
  const statusEl = document.getElementById('onvifScanStatus');

  if (!ip) {
    alert('Please enter a valid IP address.');
    return;
  }

  if (statusEl) statusEl.innerHTML = `<span class="glow-cyan">📡 Connecting to ONVIF Device Service on ${ip}:${port}...</span>`;
  try {
    const dev = await probeONVIFAPI(ip, port, user, pass);
    if (statusEl) statusEl.innerHTML = `<span class="glow-green">✓ Connected: ${dev.manufacturer} ${dev.model}</span>`;
    renderONVIFDevices([dev]);
  } catch (err) {
    if (statusEl) statusEl.innerHTML = `<span class="glow-magenta">❌ Error: ${err.message}</span>`;
  }
}

function renderONVIFDevices(devices) {
  const resultsEl = document.getElementById('onvifResultsList');
  if (!resultsEl) return;

  resultsEl.innerHTML = devices.map(dev => `
    <div class="onvif-card">
      <div class="onvif-card-header">
        <span class="onvif-mfg badge badge-neon">${dev.manufacturer || 'ONVIF'}</span>
        <strong class="glow-cyan">${dev.name || dev.model}</strong>
        <span class="text-mono" style="color:#94a3b8; font-size:0.75rem;">${dev.ip_address}:${dev.port}</span>
      </div>
      <div class="onvif-rtsp-preview text-mono">${dev.rtsp_url || 'RTSP URL Pending'}</div>
      <div class="onvif-actions">
        <button class="btn btn-cyber-primary" onclick="window.ingestONVIFCamera('${dev.device_id}', '${dev.name}', '${dev.rtsp_url}')">
          ⚡ INGEST STREAM (ZERO-COPY)
        </button>
      </div>
    </div>
  `).join('');
}

window.ingestONVIFCamera = async (id, name, url) => {
  try {
    await importONVIFStreamAPI({ stream_id: id, name, source_url: url, ingest_fps: 30 });
    alert(`[HYDRASTREAM] Stream '${id}' successfully ingested and registered in Zero-Copy Engine!`);
    closeONVIFModal();
    loadStreams();
  } catch (err) {
    alert(`Failed to import stream: ${err.message}`);
  }
};
