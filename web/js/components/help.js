/* ==========================================================================
   CYBERPUNK HELP MODAL MODULE
   ========================================================================== */

export function openHelpModal() {
  const modal = document.getElementById('helpModalOverlay');
  if (modal) modal.style.display = 'flex';
}

export function closeHelpModal() {
  const modal = document.getElementById('helpModalOverlay');
  if (modal) modal.style.display = 'none';
}
