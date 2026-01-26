import Alpine from 'alpinejs';

import { MODES, SAMPLE_SOURCE } from './constants.js';
import { YamlEditorManager } from './editor.js';
import { SourceStore } from './sources.js';
import { ValidationService } from './validation.js';

function statusClass(type) {
  return (
    {
      ok: 'is-success',
      warning: 'is-warning',
      error: 'is-danger'
    }[type] || 'is-warning'
  );
}

export function registerAlpineSchemaApp() {
  window.Alpine = Alpine;

  Alpine.data('schemaApp', () => ({
    mode: MODES.browse,
    activeTab: 'yaml',

    statusType: 'warning',
    statusText: 'Loading…',
    errors: [],

    sources: [],
    selectedSource: '',

    currentPath: '',
    currentYAML: '',
    playgroundYAML: SAMPLE_SOURCE.content,
    yamlText: '',
    envText: '',

    _sourceStore: new SourceStore(),
    _validator: new ValidationService(),
    _yamlEditor: null,

    get statusTagClass() {
      return statusClass(this.statusType);
    },

    init() {
      const params = new URLSearchParams(window.location.search);
      const initialMode =
        params.get('mode') === MODES.playground ? MODES.playground : MODES.browse;

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
        this._yamlEditor?.mount({
          parent: this.$refs?.yamlEditor,
          doc: this.yamlText || '',
          editable: this.mode === MODES.playground,
          placeholderText: this.yamlPlaceholderText()
        });
      });

      this.$watch('mode', () => {
        this._yamlEditor?.syncConfig({
          editable: this.mode === MODES.playground,
          placeholderText: this.yamlPlaceholderText()
        })
      });
      this.$watch('yamlText', (value) => {
        this._yamlEditor?.setDoc(value);
      })

      // Initial sources load
      this.refreshSources();
    },

    setMode(mode, { pushHistory = true } = {}) {
      this.mode = mode;
      this.activeTab = 'yaml';
      this.errors = [];

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

    yamlPlaceholderText() {
      return this.mode === MODES.playground ? 'Edit YAML here…' : 'Select a source file to view YAML.';
    },

    async refreshSources() {
      this.setStatus('warning', 'Loading sources…');

      try {
        let sources = await this._sourceStore.listSources();
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
        this.selectedSource = '';
        this.setStatus('warning', 'Cannot load source files');
      }
    },

    async loadSelectedSource() {
      if (!this.selectedSource) {
        return;
      }

      try {
        const sourceContent = await this._sourceStore.getSourceContent(this.selectedSource);
        this.currentPath = this.selectedSource;
        this.currentYAML = sourceContent;
        this.yamlText = sourceContent;

        this.setStatus(
          'ok',
          this.selectedSource === SAMPLE_SOURCE.value ? 'Sample loaded' : 'Source loaded'
        );
      } catch (error) {
        this.setStatus('warning', 'Failed to load source');
      }
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
