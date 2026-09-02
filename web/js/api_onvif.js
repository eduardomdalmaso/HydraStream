/* ==========================================================================
   HYDRASTREAM ONVIF CAMERA API CLIENT
   ========================================================================== */

export async function discoverONVIFAPI() {
  try {
    const res = await fetch('/api/v1/onvif/discover');
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    return null;
  }
}

export async function probeONVIFAPI(ip, port, username, password) {
  const res = await fetch('/api/v1/onvif/probe', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ip_address: ip, port: parseInt(port) || 80, username, password })
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Connection failed' }));
    throw new Error(err.error || 'Failed to probe ONVIF camera');
  }
  return await res.json();
}

export async function importONVIFStreamAPI(data) {
  const res = await fetch('/api/v1/onvif/import', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Import failed' }));
    throw new Error(err.error || 'Failed to import stream');
  }
  return await res.json();
}
