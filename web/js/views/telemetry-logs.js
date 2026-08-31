/* ==========================================================================
   HYDRASTREAM TELEMETRY LOGS VIEW MODULE (REAL TELEMETRY STREAM)
   ========================================================================== */

import { fetchControlPanelTelemetryAPI, fetchSystemInfoAPI } from '../api.js';

let isLogPaused = false;
let logIntervalId = null;

export function appendLogEntry(level, sys, msg) {
  const terminal = document.getElementById('telemetryLogTerminal');
  if (!terminal || isLogPaused) return;

  const time = new Date().toISOString().substring(11, 23);
  const row = document.createElement('div');
  row.className = `log-entry log-lvl-${level}`;
  row.dataset.level = level;
  row.dataset.sys = sys;

  row.innerHTML = `
    <span class="log-time">${time}</span>
    <span class="log-badge ${level}">${level}</span>
    <span class="log-subsys">[${sys}]</span>
    <span class="log-msg">${msg}</span>
  `;

  terminal.appendChild(row);
  if (terminal.childNodes.length > 150) {
    terminal.removeChild(terminal.firstChild);
  }
  terminal.scrollTop = terminal.scrollHeight;
}

export async function pollLiveSystemLogs() {
  const [stats, info] = await Promise.all([fetchControlPanelTelemetryAPI(), fetchSystemInfoAPI()]);
  if (!stats) return;

  if (info && info.gpu_detected) {
    appendLogEntry('ok', 'GPU_NVDEC', `Hardware Engine Active: ${info.gpu_model} | Latency: ${stats.avg_decode_latency_ms.toFixed(2)}ms`);
  }
  appendLogEntry('info', 'POSIX_SHM', `Shared memory ring /dev/shm: ${stats.posix_shm_occupancy.toFixed(1)}% (${stats.shm_lock_free_status})`);
  appendLogEntry('info', 'INGEST', `Bitrate Peak: ${stats.peak_bandwidth_mbps.toFixed(1)} Mbps | ${stats.total_ingest_fps.toFixed(0)} FPS (${stats.active_streams_count} active pipelines)`);
  if (stats.health_score < 99.0) {
    appendLogEntry('warn', 'HEALTH', `Cluster SLA degraded: ${stats.health_score.toFixed(2)}%`);
  }
}

export function initTelemetryLogs() {
  const terminal = document.getElementById('telemetryLogTerminal');
  if (!terminal) return;

  if (!logIntervalId) {
    appendLogEntry('ok', 'SYS_INIT', 'HydraStream Control Plane connected to Go backend.');
    pollLiveSystemLogs();
    logIntervalId = setInterval(pollLiveSystemLogs, 3000);
  }
}

export function toggleLogStream() {
  isLogPaused = !isLogPaused;
  const btn = document.getElementById('btnToggleLogStream');
  if (btn) btn.innerText = isLogPaused ? '[ RESUME STREAM ]' : '[ PAUSE STREAM ]';
}

export function clearLogTerminal() {
  const terminal = document.getElementById('telemetryLogTerminal');
  if (terminal) terminal.innerHTML = '';
}

export function filterLogs(level) {
  const entries = document.querySelectorAll('.log-entry');
  entries.forEach(e => {
    e.style.display = (level === 'ALL' || e.dataset.level.toUpperCase() === level) ? 'flex' : 'none';
  });
}
