/* ==========================================================================
   HYDRASTREAM MAIN ENTRYPOINT ROUTER
   ========================================================================== */

import { switchView, filterTab } from './components/navigation.js';
import { renderStreamTable, changePage } from './components/table.js';
import { openStreamDrawer, closeDrawer, saveConsumerSettings } from './components/drawer.js';
import { openHelpModal, closeHelpModal } from './components/help.js';
import { onSearchInput, onSortChange } from './views/stream-matrix.js';
import { initControlPanel } from './views/control-panel.js';

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

document.addEventListener('DOMContentLoaded', () => {
  renderStreamTable();
  initControlPanel();
  const hash = window.location.hash.replace('#', '');
  if (hash) switchView(hash);
});
