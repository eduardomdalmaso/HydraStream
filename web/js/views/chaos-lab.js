/* ==========================================================================
   HYDRASTREAM CHAOS LAB STUDIO VIEW MODULE (REAL LIVE BACKEND INJECTION)
   ========================================================================== */

import { injectChaosAPI, resetChaosAPI } from '../api.js';

export function logChaos(msg, type = 'info') {
  const consoleEl = document.getElementById('chaosConsoleOutput');
  if (!consoleEl) return;
  const time = new Date().toLocaleTimeString();
  const entry = document.createElement('div');
  entry.style.color = type === 'warn' ? 'var(--cb-yellow)' : type === 'crit' ? 'var(--cb-magenta)' : type === 'ok' ? 'var(--cb-green)' : '#cbd5e1';
  entry.innerHTML = `<span style="color: var(--cb-text-muted);">[${time}]</span> ${msg}`;
  consoleEl.appendChild(entry);
  consoleEl.scrollTop = consoleEl.scrollHeight;
}

export function injectPacketLoss(val) {
  const label = document.getElementById('lossValLabel');
  if (label) label.innerText = `${val}%`;
}

export async function triggerChaosPacketDrop() {
  const slider = document.getElementById('packetLossSlider');
  const val = parseFloat(slider ? slider.value : '25');
  logChaos(`⚡ [INJECT:API] Dispatching ${val}% packet drop to Go Control Plane...`, 'crit');
  const res = await injectChaosAPI('packet_drop', val);
  if (res) {
    logChaos(`🛡️ [BACKEND RESPONSE] ${res.message} (Recovery Δt: ${res.recovery_ms.toFixed(1)}ms)`, 'ok');
  } else {
    logChaos(`❌ [FAILED] Backend unreachable for chaos injection.`, 'warn');
  }
}

export async function triggerChaosDisconnect() {
  logChaos(`⚠️ [INJECT:API] Requesting active socket severing on cam_entrance_01...`, 'warn');
  const res = await injectChaosAPI('disconnect', 100);
  if (res) {
    logChaos(`🔄 [RECONNECT:OK] ${res.message}`, 'ok');
  } else {
    logChaos(`❌ [FAILED] Backend unreachable for chaos injection.`, 'warn');
  }
}

export async function triggerChaosGPUStall() {
  logChaos(`🔥 [INJECT:API] Injecting artificial GPU pipeline stall (+20ms Δt)...`, 'crit');
  const res = await injectChaosAPI('gpu_stall', 20);
  if (res) {
    logChaos(`⚡ [FAILOVER:OK] ${res.message} (Recovered in ${res.recovery_ms.toFixed(1)}ms)`, 'ok');
  } else {
    logChaos(`❌ [FAILED] Backend unreachable for chaos injection.`, 'warn');
  }
}

export async function triggerChaosSHMOverflow() {
  logChaos(`💥 [INJECT:API] Saturating POSIX /dev/shm ring buffer to 95% capacity...`, 'crit');
  const res = await injectChaosAPI('shm_overflow', 95);
  if (res) {
    logChaos(`🧹 [RING_EVICTION:OK] ${res.message} (Completed in ${res.recovery_ms.toFixed(1)}ms)`, 'ok');
  } else {
    logChaos(`❌ [FAILED] Backend unreachable for chaos injection.`, 'warn');
  }
}

export async function resetChaosLab() {
  const consoleEl = document.getElementById('chaosConsoleOutput');
  if (consoleEl) consoleEl.innerHTML = '';
  const res = await resetChaosAPI();
  logChaos(`✅ Chaos Lab Studio reset. ${res ? res.message : 'All circuits disarmed.'}`, 'ok');
}
