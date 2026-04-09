import { ENDPOINTS, SAMPLE_SOURCE } from './constants.js';

async function fetchOrThrow(url, options = {}, accept404 = false) {
  const response = await fetch(url, options);

  if (!response.ok && (response.status !== 404 || !accept404)) {
    const errorText = await response.text();
    throw new Error(`HTTP ${response.status}: ${errorText || response.statusText}`);
  }
  if (response.status === 404) {
    return null;
  }
  return response;
}

export class SourceStore {
  async listVirtualGateways(signal) {
    const response = await fetchOrThrow(ENDPOINTS.gateways, { signal }, true);
    if (response === null) {
      return {};
    }
    const gateways = await response.json();
    if (!gateways || typeof gateways !== 'object') {
      return {};
    }
    return gateways;
  }

  async getIndex(virtualGateway, signal) {
    const normalizedVirtualGateway = String(virtualGateway || '').replace(/^\/+|\/+$/g, '');
    const indexUrl = `${ENDPOINTS.virtualgw}/${encodeURIComponent(normalizedVirtualGateway)}`;
    const response = await fetchOrThrow(indexUrl, { signal });
    return response.json();
  }

  async listSourceMap() {
    const response = await fetchOrThrow(ENDPOINTS.sources, {}, true);
    if (response === null) {
      return {};
    }
    const sources = await response.json();
    if (sources && typeof sources === 'object') {
      return sources;
    }
    return {};
  }

  async getSourceContent(selectedPath) {
    if (selectedPath === SAMPLE_SOURCE.value) {
      return SAMPLE_SOURCE.content;
    }

    const normalizedPath = selectedPath.replace(/^\//, '');
    const sourceUrl = `../sources/${encodeURI(normalizedPath)}`;
    const response = await fetchOrThrow(sourceUrl);
    return response.text();
  }

  async getConfigContent(virtualGateway, kind, id) {
    const normalizedVirtualGateway = String(virtualGateway || '').replace(/^\/+|\/+$/g, '');
    const normalizedKind = String(kind || '').replace(/^\/+|\/+$/g, '');
    const normalizedId = String(id || '').replace(/^\/+|\/+$/g, '');
    const configUrl = `${ENDPOINTS.virtualgw}/${encodeURIComponent(normalizedVirtualGateway)}/${encodeURIComponent(normalizedKind)}/${encodeURIComponent(normalizedId)}`;
    const response = await fetchOrThrow(configUrl);
    return response.text();
  }
}
