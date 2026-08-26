/* ==========================================================================
   HYDRASTREAM NAVIGATION & ROUTING MODULE
   ========================================================================== */

import { state } from '../state.js';

export function switchView(viewName) {
  state.activeView = viewName;
  
  document.querySelectorAll('.spa-view').forEach(el => {
    el.style.display = 'none';
  });

  const targetView = document.getElementById(`view-${viewName}`);
  if (targetView) {
    targetView.style.display = 'block';
  }

  document.querySelectorAll('.cyber-nav-item').forEach(el => {
    el.classList.remove('active');
  });

  const activeNav = document.querySelector(`.cyber-nav-item[onclick*="${viewName}"]`);
  if (activeNav) {
    activeNav.classList.add('active');
  }

  window.location.hash = viewName;
}

export function filterTab(type) {
  state.currentTabFilter = type;
  state.currentPage = 1;

  document.querySelectorAll('.cyber-tab-btn').forEach(btn => {
    btn.classList.remove('active');
  });

  const activeBtn = document.querySelector(`.cyber-tab-btn[onclick*="${type}"]`);
  if (activeBtn) {
    activeBtn.classList.add('active');
  }
}
