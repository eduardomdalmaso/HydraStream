/* ==========================================================================
   HYDRASTREAM STREAM MATRIX VIEW MODULE
   ========================================================================== */

import { state } from '../state.js';
import { renderStreamTable } from '../components/table.js';

export function onSearchInput(val) {
  state.searchQuery = val;
  state.currentPage = 1;
  renderStreamTable();
}

export function onSortChange(val) {
  state.sortBy = val;
  state.currentPage = 1;
  renderStreamTable();
}
