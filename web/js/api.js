/* ==========================================================================
   HYDRASTREAM API CLIENT MODULE
   ========================================================================== */

export async function fetchStreamsAPI(search = '', tenant = '', sortBy = '', page = 1, limit = 10) {
  try {
    const params = new URLSearchParams({ search, tenant, sort_by: sortBy, page, limit });
    const res = await fetch(`/api/v1/streams?${params.toString()}`);
    if (!res.ok) throw new Error(`HTTP error ${res.status}`);
    return await res.json();
  } catch (err) {
    console.warn('[HydraStream API] Live server unavailable, using local state:', err);
    return null;
  }
}

export async function fetchSystemInfoAPI() {
  try {
    const res = await fetch('/api/v1/info');
    if (!res.ok) throw new Error(`HTTP error ${res.status}`);
    return await res.json();
  } catch (err) {
    return null;
  }
}

export async function updateConsumerFPSAPI(streamId, analyticType, targetFPS, format) {
  try {
    const res = await fetch(`/api/v1/streams/${streamId}/consumers/${analyticType}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ target_fps: targetFPS, output_format: format })
    });
    return res.ok;
  } catch (err) {
    return false;
  }
}
