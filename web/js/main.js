/* ==========================================================================
   HYDRASTREAM MAIN ENTRYPOINT ROUTER
   ========================================================================== */

import { switchView, filterTab } from './components/navigation.js?v=1.0.2';
import { renderStreamTable, changePage } from './components/table.js?v=1.0.2';
import { openStreamDrawer, closeDrawer, saveConsumerSettings } from './components/drawer.js?v=1.0.2';
import { openHelpModal, closeHelpModal } from './components/help.js?v=1.0.2';
import { onSearchInput, onSortChange } from './views/stream-matrix.js?v=1.0.2';
import { initControlPanel } from './views/control-panel.js?v=1.0.2';
import { initClusterTopology } from './views/cluster-topology.js?v=1.0.2';
import { initTelemetryLogs, toggleLogStream, clearLogTerminal, filterLogs } from './views/telemetry-logs.js?v=1.0.2';
import { 
  triggerChaosPacketDrop, 
  triggerChaosDisconnect, 
  triggerChaosGPUStall, 
  triggerChaosSHMOverflow, 
  resetChaosLab, 
  injectPacketLoss 
} from './views/chaos-lab.js?v=1.0.2';

window.switchView = switchView;
window.filterTab = filterTab;
window.changePage = changePage;
window.openStreamDrawer = openStreamDrawer;
window.closeDrawer = closeDrawer;
window.saveConsumerSettings = saveConsumerSettings;
window.openHelpModal = openHelpModal;
window.closeHelpModal = closeHelpModal;
window.onSearchInput = onSearchInput;
window.onSortChange = onSortChange;

// Chaos Lab Globals
window.triggerChaosPacketDrop = triggerChaosPacketDrop;
window.triggerChaosDisconnect = triggerChaosDisconnect;
window.triggerChaosGPUStall = triggerChaosGPUStall;
window.triggerChaosSHMOverflow = triggerChaosSHMOverflow;
window.resetChaosLab = resetChaosLab;
window.injectPacketLoss = injectPacketLoss;

// Telemetry Logs Globals
window.toggleLogStream = toggleLogStream;
window.clearLogTerminal = clearLogTerminal;
window.filterLogs = filterLogs;

window.addEventListener('viewChanged', (e) => {
  const v = e.detail.viewName;
  if (v === 'control-panel') initControlPanel();
  if (v === 'stream-matrix') renderStreamTable();
  if (v === 'cluster-topology') initClusterTopology();
  if (v === 'telemetry-logs') initTelemetryLogs();
});

document.addEventListener('DOMContentLoaded', () => {
  renderStreamTable();
  initControlPanel();
  initClusterTopology();
  initTelemetryLogs();
  const hash = window.location.hash.replace('#', '') || 'control-panel';
  switchView(hash);
});
