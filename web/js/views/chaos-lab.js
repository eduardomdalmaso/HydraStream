/* ==========================================================================
   HYDRASTREAM CHAOS LAB STUDIO VIEW MODULE
   ========================================================================== */

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

export function triggerChaosPacketDrop() {
  const slider = document.getElementById('packetLossSlider');
  const val = slider ? slider.value : '25';
  logChaos(`⚡ [INJECT] Simulating ${val}% packet drop on RTSP ingress...`, 'crit');
  setTimeout(() => {
    logChaos(`🛡️ [AUTOPILOT] Dynamic jitter buffer engaged. Recovered in 84ms (0 frame loss).`, 'ok');
  }, 900);
}

export function triggerChaosDisconnect() {
  logChaos(`⚠️ [INJECT] Severing RTSP TCP connection on cam_entrance_01...`, 'warn');
  setTimeout(() => {
    logChaos(`🔄 [RECONNECT] Auto-reconnect triggered. Handshake OK in 142ms.`, 'ok');
  }, 1100);
}

export function triggerChaosGPUStall() {
  logChaos(`🔥 [INJECT] Artificially throttling NVDEC GPU decode pipeline (+20ms)...`, 'crit');
  setTimeout(() => {
    logChaos(`⚡ [OFFLOAD] CPU POSIX SHM fallback engaged. Queue back to 1.4ms.`, 'ok');
  }, 1200);
}

export function triggerChaosSHMOverflow() {
  logChaos(`💥 [INJECT] Saturating /dev/shm atomic ring buffer to 95% capacity...`, 'crit');
  setTimeout(() => {
    logChaos(`🧹 [RING_BUFFER] Lock-free atomic eviction dropped oldest 3 unconsumed frames. Buffer stabilized at 18%.`, 'ok');
  }, 1000);
}

export function resetChaosLab() {
  const consoleEl = document.getElementById('chaosConsoleOutput');
  if (consoleEl) consoleEl.innerHTML = '';
  logChaos(`✅ Chaos Lab Studio reset. All injection circuits disarmed.`, 'ok');
}
