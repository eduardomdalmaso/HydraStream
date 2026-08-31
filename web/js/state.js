/* ==========================================================================
   HYDRASTREAM STATE STORE MODULE (CLEAN LIVE REACTIVE STATE)
   ========================================================================== */

export const state = {
  activeView: 'control-panel',
  currentTabFilter: 'all',
  searchQuery: '',
  sortBy: 'resource_desc',
  currentPage: 1,
  pageSize: 10,
  activeDrawerStreamId: null,
  rawStreams: []
};

export function getFilteredStreams() {
  let streams = state.rawStreams || [];

  if (state.currentTabFilter !== 'all') {
    streams = streams.filter(s => {
      const isRTSP = s.source_url && s.source_url.startsWith('rtsp://');
      const isMP4 = s.source_url && (s.source_url.startsWith('file://') || s.source_url.endsWith('.mp4'));
      if (state.currentTabFilter === 'rtsp') return isRTSP;
      if (state.currentTabFilter === 'mp4') return isMP4;
      return true;
    });
  }

  if (state.searchQuery) {
    const q = state.searchQuery.toLowerCase();
    streams = streams.filter(s => 
      (s.stream_id && s.stream_id.toLowerCase().includes(q)) || 
      (s.source_url && s.source_url.toLowerCase().includes(q)) || 
      (s.tenant_id && s.tenant_id.toLowerCase().includes(q))
    );
  }

  return streams;
}
