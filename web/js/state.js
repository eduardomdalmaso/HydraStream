/* ==========================================================================
   HYDRASTREAM STATE STORE MODULE
   ========================================================================== */

export const state = {
  activeView: 'control-panel',
  currentTabFilter: 'all',
  searchQuery: '',
  sortBy: 'resource_desc',
  currentPage: 1,
  pageSize: 10,
  activeDrawerStreamId: null,
  rawStreams: [],
  sampleStreams: [
    {
      tenant_id: 'tenant_company_alpha',
      stream_id: 'cam_entrance_01',
      source_url: 'rtsp://mediamtx:8554/tenant_company_alpha/cam_entrance_01',
      type: 'rtsp',
      status: 'ONLINE',
      resolution: '1920x1080',
      codec: 'h264',
      ingest_fps: 30.0,
      network_kbps: 4850.5,
      latency: '1.42 ms',
      resource_score: 82.4,
      transport: 'CUDA IPC (NVDEC)',
      consumers: [
        { analytic_type: 'lpr_ocr', target_fps: 2.0, output_format: 'shm_numpy' },
        { analytic_type: 'object_tracker', target_fps: 15.0, output_format: 'cuda_ipc_tensor' }
      ]
    },
    {
      tenant_id: 'tenant_company_alpha',
      stream_id: 'cam_parking_02',
      source_url: 'rtsp://mediamtx:8554/tenant_company_alpha/cam_parking_02',
      type: 'rtsp',
      status: 'ONLINE',
      resolution: '1920x1080',
      codec: 'h264',
      ingest_fps: 30.0,
      network_kbps: 5120.0,
      latency: '1.85 ms',
      resource_score: 88.1,
      transport: 'CUDA IPC (NVDEC)',
      consumers: [
        { analytic_type: 'license_plate', target_fps: 5.0, output_format: 'cuda_ipc' }
      ]
    },
    {
      tenant_id: 'tenant_company_beta',
      stream_id: 'file_test_video_01',
      source_url: 'file:///var/videos/benchmark_4k.mp4',
      type: 'mp4',
      status: 'ONLINE',
      resolution: '3840x2160',
      codec: 'h265',
      ingest_fps: 60.0,
      network_kbps: 12400.0,
      latency: '0.95 ms',
      resource_score: 94.2,
      transport: 'POSIX SHM',
      consumers: [
        { analytic_type: 'yolo_v8_detection', target_fps: 30.0, output_format: 'shm_numpy' }
      ]
    }
  ]
};

export function getFilteredStreams() {
  let streams = (state.rawStreams && state.rawStreams.length > 0) ? state.rawStreams : state.sampleStreams;

  if (state.currentTabFilter !== 'all') {
    streams = streams.filter(s => s.type === state.currentTabFilter);
  }

  if (state.searchQuery) {
    const q = state.searchQuery.toLowerCase();
    streams = streams.filter(s => 
      s.stream_id.toLowerCase().includes(q) || 
      s.source_url.toLowerCase().includes(q) || 
      s.tenant_id.toLowerCase().includes(q)
    );
  }

  return streams;
}
