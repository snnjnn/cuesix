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
  async listSources() {
    const response = await fetchOrThrow(ENDPOINTS.sources, {}, true);
    if (response === null) {
      return [SAMPLE_SOURCE.value];
    }
    const sources = await response.json();
    if (!Array.isArray(sources) || sources.length === 0) {
      return [];
    }
    return sources;
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
}
