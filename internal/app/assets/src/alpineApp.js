import Alpine from 'alpinejs';
import DataTable from 'datatables.net-bm';

import { MODES, SAMPLE_SOURCE } from './constants.js';
import { YamlEditorManager } from './editor.js';
import { SourceStore } from './sources.js';
import { ValidationService } from './validation.js';

const INDEX_KINDS = [
  'routes',
  'services',
  'upstreams',
  'ssls',
  'global_rules',
  'consumer_groups',
  'plugin_configs',
  'stream_routes',
  'protos',
  'consumers',
  'plugin_metadata'
];

function statusClass(type) {
  return (
    {
      ok: 'is-success',
      warning: 'is-warning',
      error: 'is-danger'
    }[type] || 'is-warning'
  );
}

function escapeHtml(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function sourceFilename(path) {
  const sourcePath = String(path ?? '');
  const normalizedPath = sourcePath.replace(/\/+$/g, '');
  const lastSlash = normalizedPath.lastIndexOf('/');
  if (lastSlash < 0) {
    return sourcePath;
  }
  const filename = normalizedPath.slice(lastSlash + 1);
  if (!filename) {
    return sourcePath;
  }
  return filename;
}

function formatSourceLabel(path, virtualGateway, selectedVirtualGateway) {
  const sourcePath = sourceFilename(path);
  const gateway = String(virtualGateway ?? '').trim();
  const selected = String(selectedVirtualGateway ?? '').trim();
  if (!gateway || gateway === selected) {
    return sourcePath;
  }
  return `(${gateway}) ${sourcePath}`;
}

export function registerAlpineSchemaApp() {
  window.Alpine = Alpine;

  Alpine.data('schemaApp', () => ({
    mode: MODES.index,
    activeTab: 'yaml',

    statusType: 'warning',
    statusText: 'Loading…',
    errors: [],
    indexError: '',

    sources: [],
    sourceVirtualGateways: {},
    selectedSource: '',
    indexRows: [],
    virtualGatewayReadiness: {},
    selectedVirtualGateway: '',
    selectedIndexKind: '',
    indexKinds: INDEX_KINDS,
    indexEditorModalOpen: false,
    indexSelectionType: '',
    activeTagFilters: [],
    indexSearchQuery: '',

    currentPath: '',
    currentDocType: 'source',
    currentYAML: '',
    playgroundYAML: SAMPLE_SOURCE.content,
    yamlText: '',
    envText: '',

    _sourceStore: new SourceStore(),
    _validator: new ValidationService(),
    _yamlEditor: null,
    _yamlEditorMountParent: null,
    _indexDataTable: null,
    _indexTableClickHandler: null,
    _indexTagFilterFn: null,
    _indexSearchSyncHandler: null,
    _indexAbortController: null,

    get statusTagClass() {
      return statusClass(this.statusType);
    },
    get selectedVirtualGatewayReady() {
      if (!this.selectedVirtualGateway) {
        return null;
      }
      return this.virtualGatewayReadiness[this.selectedVirtualGateway] === true;
    },
    get virtualGatewayNames() {
      return Object.keys(this.virtualGatewayReadiness).sort((a, b) => a.localeCompare(b));
    },
    get selectedVirtualGatewayIndicatorButtonClass() {
      return 'is-static';
    },
    get selectedVirtualGatewayIndicatorIcon() {
      if (!this.selectedVirtualGateway) {
        return 'fa-circle-question';
      }
      return this.selectedVirtualGatewayReady ? 'fa-circle-check' : 'fa-circle-xmark';
    },
    get selectedVirtualGatewayIndicatorIconClass() {
      if (!this.selectedVirtualGateway) {
        return 'has-text-grey-light';
      }
      return this.selectedVirtualGatewayReady ? 'has-text-success' : 'has-text-danger';
    },

    init() {
      const params = new URLSearchParams(window.location.search);
      const modeParam = params.get('mode');
      const initialMode = Object.values(MODES).includes(modeParam) ? modeParam : MODES.index;

      this.setMode(initialMode, { pushHistory: false });

      this._yamlEditor = new YamlEditorManager({
        onChange: (value) => {
          if (this.mode === MODES.playground) {
            this.playgroundYAML = value;
          }
        },
        onValidate: () => {
          void this.validate();
        }
      });

      this.$nextTick(() => {
        this.ensureYamlEditorMounted();
      });

      this.$watch('mode', () => {
        this._yamlEditor?.syncConfig({
          editable: this.mode === MODES.playground,
          placeholderText: this.yamlPlaceholderText()
        });
        this.$nextTick(() => {
          this.ensureYamlEditorMounted();
        });
        if (this.mode === MODES.index) {
          this.refreshIndex();
        } else {
          this.destroyIndexDataTable();
        }
      });
      this.$watch('yamlText', (value) => {
        this._yamlEditor?.setDoc(value);
      });

      // Initial sources load
      this.refreshSources();
      if (initialMode === MODES.index) {
        this.refreshIndex({ refreshGateways: true });
      }
    },

    setMode(mode, { pushHistory = true } = {}) {
      this.mode = mode;
      this.activeTab = 'yaml';
      this.errors = [];
      if (mode !== MODES.index) {
        this.closeIndexEditorModal();
      }

      if (mode === MODES.playground) {
        this.yamlText = this.playgroundYAML || SAMPLE_SOURCE.content;
      } else {
        this.yamlText = this.currentYAML || '';
      }

      if (pushHistory) {
        const url = new URL(window.location.href);
        url.searchParams.set('mode', mode);
        window.history.replaceState({}, '', url.toString());
      }
    },

    setStatus(type, text) {
      this.statusType = type;
      this.statusText = text;
    },

    setTab(tab) {
      this.activeTab = tab;
    },

    openIndexEditorModal() {
      this.indexEditorModalOpen = true;
    },

    closeIndexEditorModal() {
      this.indexEditorModalOpen = false;
    },

    async openIndexEditorModalWithLoadedContent() {
      await new Promise((resolve) => this.$nextTick(resolve));
      this.openIndexEditorModal();
    },

    yamlPlaceholderText() {
      if (this.mode === MODES.playground) {
        return 'Edit YAML here…';
      }
      if (this.mode === MODES.index) {
        return 'Click any resource ID in the index table to view YAML.';
      }
      return 'Select a source file to view YAML.';
    },

    yamlEditorParent() {
      return this.mode === MODES.index ? this.$refs?.yamlEditorIndex : this.$refs?.yamlEditorStandard;
    },

    ensureYamlEditorMounted() {
      const parent = this.yamlEditorParent();
      if (!parent) {
        return;
      }
      if (this._yamlEditorMountParent === parent) {
        return;
      }

      this._yamlEditor?.destroy();
      this._yamlEditorMountParent = parent;
      this._yamlEditor?.mount({
        parent,
        doc: this.yamlText || '',
        editable: this.mode === MODES.playground,
        placeholderText: this.yamlPlaceholderText()
      });
    },

    async refreshSources() {
      this.setStatus('warning', 'Loading sources…');

      try {
        const sourceMap = await this._sourceStore.listSourceMap();
        this.sourceVirtualGateways = sourceMap;
        let sources = Object.keys(sourceMap).sort((a, b) => a.localeCompare(b));
        if (!sources.length) {
          sources = [SAMPLE_SOURCE.value];
        }

        this.sources = sources;

        if (this.selectedSource && !sources.includes(this.selectedSource)) {
          this.selectedSource = '';
        }

        this.setStatus('ok', 'Ready');
      } catch (error) {
        this.sources = [];
        this.sourceVirtualGateways = {};
        this.selectedSource = '';
        this.setStatus('warning', 'Cannot load source files');
      }
    },

    preferredVirtualGateway(gateways) {
      if (!Array.isArray(gateways) || gateways.length === 0) {
        return '';
      }
      if (this.selectedVirtualGateway && gateways.includes(this.selectedVirtualGateway)) {
        return this.selectedVirtualGateway;
      }
      if (gateways.includes('default')) {
        return 'default';
      }
      return gateways[0];
    },

    async refreshVirtualGateways(signal) {
      const gatewayReadiness = await this._sourceStore.listVirtualGateways(signal);
      this.virtualGatewayReadiness = gatewayReadiness;
      this.selectedVirtualGateway = this.preferredVirtualGateway(this.virtualGatewayNames);
      return this.selectedVirtualGateway;
    },

    async refreshIndex({ refreshGateways = false } = {}) {
      this.indexError = '';

      this._indexAbortController?.abort();
      const controller = new AbortController();
      this._indexAbortController = controller;
      const { signal } = controller;

      try {
        let selectedGateway = this.selectedVirtualGateway;
        if (refreshGateways) {
          selectedGateway = await this.refreshVirtualGateways(signal);
        } else if (!selectedGateway && this.virtualGatewayNames.length === 0) {
          // First index open when gateways were never loaded in this session.
          selectedGateway = await this.refreshVirtualGateways(signal);
        }
        if (!selectedGateway) {
          this.indexRows = [];
          this.syncIndexKinds();
          this.$nextTick(() => this.renderIndexDataTable());
          return;
        }

        const indexMap = await this._sourceStore.getIndex(selectedGateway, signal);
        this.indexRows = this.flattenIndexRows(indexMap);
        this.syncIndexKinds();
        this.$nextTick(() => this.renderIndexDataTable());
      } catch (error) {
        if (error?.name === 'AbortError') {
          return;
        }
        this.indexRows = [];
        this.indexError = error instanceof Error ? error.message : String(error);
        this.$nextTick(() => this.renderIndexDataTable());
      } finally {
        if (this._indexAbortController === controller) {
          this._indexAbortController = null;
        }
      }
    },

    async reloadIndex() {
      await this.refreshIndex({ refreshGateways: true });
    },

    flattenIndexRows(indexMap) {
      if (!indexMap || typeof indexMap !== 'object') {
        return [];
      }

      return Object.entries(indexMap)
        .flatMap(([kind, idMap]) => {
          if (!idMap || typeof idMap !== 'object') {
            return [];
          }
          return Object.entries(idMap).map(([id, descriptor]) => ({
            kind,
            id,
            details:
              descriptor && typeof descriptor === 'object' && descriptor.tags && typeof descriptor.tags === 'object'
                ? descriptor.tags
                : {},
            sources:
              descriptor && typeof descriptor === 'object' && Array.isArray(descriptor.paths)
                ? descriptor.paths
                : []
          }));
        })
        .sort((a, b) => a.kind.localeCompare(b.kind) || a.id.localeCompare(b.id));
    },

    syncIndexKinds() {
      const dataKinds = this.indexRows.map((row) => row.kind).filter(Boolean);
      this.indexKinds = Array.from(new Set([...INDEX_KINDS, ...dataKinds])).sort((a, b) =>
        a.localeCompare(b)
      );
    },

    destroyIndexDataTable() {
      if (!this._indexDataTable) {
        return;
      }
      if (this._indexSearchSyncHandler) {
        this._indexDataTable.off('search', this._indexSearchSyncHandler);
      }
      this._indexSearchSyncHandler = null;
      if (this._indexTagFilterFn && DataTable.ext?.search) {
        const idx = DataTable.ext.search.indexOf(this._indexTagFilterFn);
        if (idx !== -1) {
          DataTable.ext.search.splice(idx, 1);
        }
      }
      this._indexTagFilterFn = null;
      this._indexDataTable.destroy();
      this._indexDataTable = null;
      const tableEl = this.$refs?.indexTable;
      if (tableEl && this._indexTableClickHandler) {
        tableEl.removeEventListener('click', this._indexTableClickHandler);
      }
      tableEl?.classList.remove('index-table-hidden', 'index-table-fade-in');
      this._indexTableClickHandler = null;
      this.indexSearchQuery = '';
    },

    hideIndexTable() {
      const tableEl = this.$refs?.indexTable;
      if (!tableEl) {
        return;
      }
      // Reset previous animation before hiding so the next fade-in starts from a clean state.
      tableEl.classList.remove('index-table-fade-in');
      tableEl.classList.add('index-table-hidden');
    },

    fadeInIndexTable() {
      const tableEl = this.$refs?.indexTable;
      if (!tableEl) {
        return;
      }
      tableEl.classList.remove('index-table-hidden');
      tableEl.classList.remove('index-table-fade-in');
      void tableEl.offsetWidth;
      tableEl.classList.add('index-table-fade-in');
    },

    initIndexDataTable() {
      const tableEl = this.$refs?.indexTable;
      if (!tableEl) {
        return;
      }

      this._indexDataTable = new DataTable(tableEl, {
        order: [[0, 'asc'], [1, 'asc']],
        orderCellsTop: true,
        autoWidth: false,
        data: [],
        columns: [
          {
            data: 'kind',
            render: (value) => escapeHtml(value || '-')
          },
          {
            data: 'id',
            render: (value, type, row) => {
              if (!value) {
                return '-';
              }
              if (type !== 'display') {
                return value;
              }
              const safeId = escapeHtml(value);
              const encodedKind = encodeURIComponent(row?.kind || '');
              const encodedId = encodeURIComponent(value);
              return `<a href="#" data-config-kind="${encodedKind}" data-config-id="${encodedId}">${safeId}</a>`;
            }
          },
          {
            data: 'details',
            orderable: false,
            render: (details) => {
              if (!details || typeof details !== 'object') {
                return '<span>-</span>';
              }
              const entries = Object.entries(details);
              if (entries.length === 0) {
                return '<span>-</span>';
              }
              return entries
                .sort(([a], [b]) => a.localeCompare(b))
                .flatMap(([key, values]) => {
                  if (!Array.isArray(values) || values.length === 0) {
                    const encodedKey = encodeURIComponent(key);
                    return [
                      `<a href="#" class="tag is-medium mr-1 mb-1" data-tag-key="${encodedKey}">${escapeHtml(key)}</a>`
                    ];
                  }
                  return values.map(
                    (value) => {
                      const encodedKey = encodeURIComponent(key);
                      const encodedValue = encodeURIComponent(String(value));
                      return `<a href="#" class="tag is-medium mr-1 mb-1" data-tag-key="${encodedKey}" data-tag-value="${encodedValue}">${escapeHtml(key)}=${escapeHtml(value)}</a>`;
                    }
                  );
                })
                .join('');
            }
          },
          {
            data: 'sources',
            orderable: false,
            render: (sources, type) => {
              if (!Array.isArray(sources) || sources.length === 0) {
                return '<span>-</span>';
              }
              if (type !== 'display') {
                return sources
                  .map((sourcePath) => formatSourceLabel(sourcePath, this.sourceVirtualGateways[sourcePath], this.selectedVirtualGateway))
                  .join('\n');
              }
              return sources
                .map((sourcePath) => {
                  const safePath = escapeHtml(sourcePath);
                  const label = escapeHtml(formatSourceLabel(sourcePath, this.sourceVirtualGateways[sourcePath], this.selectedVirtualGateway));
                  const encodedPath = encodeURIComponent(sourcePath);
                  return `<a href="#" data-source-path="${encodedPath}" title="${safePath}">${label}</a>`;
                })
                .join('<br>');
            }
          }
        ]
      });
      tableEl.classList.add('is-striped');
      this._indexTagFilterFn = (settings, _searchData, dataIndex) => {
        if (settings.nTable !== tableEl || this.activeTagFilters.length === 0) {
          return true;
        }
        const row = this._indexDataTable?.row(dataIndex).data();
        return this.rowMatchesTagFilters(row);
      };
      if (DataTable.ext?.search && this._indexTagFilterFn) {
        DataTable.ext.search.push(this._indexTagFilterFn);
      }
      this._indexSearchSyncHandler = () => {
        this.indexSearchQuery = this._indexDataTable?.search() || '';
      };
      this._indexDataTable.on('search', this._indexSearchSyncHandler);
      this.applyIndexKindFilter();

      this._indexTableClickHandler = (event) => {
        const target = event.target instanceof Element ? event.target : null;
        if (!target) {
          return;
        }

        const tagLink = target.closest('a[data-tag-key]');
        if (tagLink) {
          event.preventDefault();
          const encodedKey = tagLink.getAttribute('data-tag-key');
          if (!encodedKey) {
            return;
          }
          const encodedValue = tagLink.getAttribute('data-tag-value');
          this.addTagFilter(
            decodeURIComponent(encodedKey),
            encodedValue ? decodeURIComponent(encodedValue) : ''
          );
          return;
        }

        const configLink = target.closest('a[data-config-kind][data-config-id]');
        if (configLink) {
          event.preventDefault();
          const encodedKind = configLink.getAttribute('data-config-kind');
          const encodedId = configLink.getAttribute('data-config-id');
          if (!encodedKind || !encodedId) {
            return;
          }
          void this.openIndexConfigByKindAndId(
            decodeURIComponent(encodedKind),
            decodeURIComponent(encodedId)
          );
          return;
        }

        const sourceLink = target.closest('a[data-source-path]');
        if (!sourceLink) {
          return;
        }
        event.preventDefault();
        const encodedPath = sourceLink.getAttribute('data-source-path');
        if (!encodedPath) {
          return;
        }
        void this.openIndexSourcePath(decodeURIComponent(encodedPath));
      };
      tableEl.addEventListener('click', this._indexTableClickHandler);
    },

    rowMatchesTagFilters(row) {
      if (!row || !row.details || typeof row.details !== 'object') {
        return false;
      }
      return this.activeTagFilters.every((filter) => {
        const values = row.details[filter.key];
        if (!Array.isArray(values)) {
          return false;
        }
        if (filter.value === '') {
          return true;
        }
        return values.some((value) => String(value) === filter.value);
      });
    },

    addTagFilter(key, value = '') {
      if (!key) {
        return;
      }
      const normalized = { key, value: value ?? '' };
      const exists = this.activeTagFilters.some(
        (filter) => filter.key === normalized.key && filter.value === normalized.value
      );
      if (exists) {
        return;
      }
      this.activeTagFilters = [...this.activeTagFilters, normalized];
      this._indexDataTable?.draw();
    },

    removeTagFilter(index) {
      if (index < 0 || index >= this.activeTagFilters.length) {
        return;
      }
      const next = [...this.activeTagFilters];
      next.splice(index, 1);
      this.activeTagFilters = next;
      this._indexDataTable?.draw();
    },

    clearTagFilters() {
      if (this.activeTagFilters.length === 0) {
        return;
      }
      this.activeTagFilters = [];
      this._indexDataTable?.draw();
    },

    clearAllIndexFilters() {
      this.selectedIndexKind = '';
      this.activeTagFilters = [];
      this.indexSearchQuery = '';
      if (!this._indexDataTable) {
        return;
      }
      this._indexDataTable.search('');
      this._indexDataTable.column(0).search('');
      this._indexDataTable.draw();
    },

    hasActiveIndexFilters() {
      if (this.selectedIndexKind || this.activeTagFilters.length > 0 || this.indexSearchQuery) {
        return true;
      }
      return false;
    },

    applyIndexKindFilter() {
      if (!this._indexDataTable) {
        return;
      }
      const kind = this.selectedIndexKind || '';
      if (!kind) {
        this._indexDataTable.column(0).search('').draw();
        return;
      }
      const escaped = kind.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      this._indexDataTable.column(0).search(`^${escaped}$`, true, false).draw();
    },

    renderIndexDataTable() {
      if (this.mode !== MODES.index) {
        return;
      }

      if (!this._indexDataTable) {
        this.initIndexDataTable();
      }
      if (!this._indexDataTable) {
        return;
      }

      this.hideIndexTable();
      this._indexDataTable.clear();
      this._indexDataTable.rows.add(this.indexRows);
      this._indexDataTable.draw();
      this.applyIndexKindFilter();
      requestAnimationFrame(() => {
        this.fadeInIndexTable();
      });
    },

    async loadSelectedSource() {
      if (!this.selectedSource) {
        return;
      }

      await this.loadSourcePath(this.selectedSource);
    },

    async loadSourcePath(path) {
      if (!path) {
        return false;
      }

      try {
        const sourceContent = await this._sourceStore.getSourceContent(path);
        this.currentPath = path;
        this.currentDocType = 'source';
        this.currentYAML = sourceContent;
        this.yamlText = sourceContent;
        this.selectedSource = path;

        this.setStatus(
          'ok',
          path === SAMPLE_SOURCE.value ? 'Sample loaded' : 'Source loaded'
        );
        return true;
      } catch (error) {
        this.setStatus('warning', 'Failed to load source');
        return false;
      }
    },

    async openIndexSourcePath(path) {
      this.indexSelectionType = 'source';
      const loaded = await this.loadSourcePath(path);
      if (!loaded) {
        return;
      }
      await this.openIndexEditorModalWithLoadedContent();
    },

    async loadConfigByKindAndId(kind, id) {
      if (!this.selectedVirtualGateway || !kind || !id) {
        return false;
      }

      this.indexError = '';

      try {
        const configContent = await this._sourceStore.getConfigContent(this.selectedVirtualGateway, kind, id);
        const resourcePath = `${this.selectedVirtualGateway}/${kind}/${id}`;
        this.currentPath = resourcePath;
        this.currentDocType = 'config';
        this.currentYAML = configContent;
        this.yamlText = configContent;
        return true;
      } catch (error) {
        this.indexError = error instanceof Error ? error.message : String(error);
        return false;
      }
    },

    async openIndexConfigByKindAndId(kind, id) {
      this.indexSelectionType = 'config';
      const loaded = await this.loadConfigByKindAndId(kind, id);
      if (!loaded) {
        return;
      }
      await this.openIndexEditorModalWithLoadedContent();
    },

    resetSample() {
      this.playgroundYAML = SAMPLE_SOURCE.content;
      this.yamlText = this.playgroundYAML;
      this.setTab('yaml');
    },

    revalidate() {
      this.validate();
    },

    async validate() {
      if (this.mode === MODES.index) {
        return;
      }

      await this._validator.validate({
        mode: this.mode,
        yamlText: this.mode === MODES.browse ? this.currentYAML : this.playgroundYAML,
        envText: this.envText,
        currentPath: this.currentPath,
        onState: (state) => this.applyValidationState(state)
      });
    },

    applyValidationState(state) {
      if (!state) {
        return;
      }

      this.statusType = state.statusType;
      this.statusText = state.statusText;
      this.errors = state.errors || [];
    }
  }));

  Alpine.start();
}
