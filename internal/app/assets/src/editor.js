import { EditorState, Compartment, Transaction, Prec } from '@codemirror/state';
import { EditorView, keymap, placeholder as codeMirrorPlaceholder } from '@codemirror/view';
import { defaultHighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { yaml } from '@codemirror/lang-yaml';
import { oneDark } from '@codemirror/theme-one-dark';
import { basicSetup } from 'codemirror';

function createBaseTheme() {
  return EditorView.theme({
    '&': {
      border: '1px solid var(--bulma-border)',
      //borderRadius: 'var(--bulma-radius)',
      fontFamily: 'var(--bulma-family-code)',
      backgroundColor: 'var(--bulma-background)',
      fontSize: '1rem'
    },
    '.cm-scroller': {
      overflow: 'auto'
    },
    '.cm-content': {
      padding: '0.75em',
      minHeight: '28em'
    },
    '&.cm-focused': {
      outline: 'none',
      borderColor: 'var(--bulma-link)',
      boxShadow: '0 0 0 0.125em color-mix(in srgb, var(--bulma-link) 25%, transparent)'
    }
  });
}

function isDarkMode(mediaQueryList) {
  return mediaQueryList?.matches ?? false;
}

export class YamlEditorManager {
  constructor({ onChange, onValidate } = {}) {
    this.onChange = onChange;
    this.onValidate = onValidate;
    this.view = null;
    this._darkQuery = null;
    this._config = {
      editable: false,
      placeholderText: ''
    };
    this._themeCompartment = new Compartment();
    this._readOnlyCompartment = new Compartment();
    this._placeholderCompartment = new Compartment();
  }

  mount({ parent, doc = '', editable = false, placeholderText = '' } = {}) {
    if (this.view || !parent) {
      return;
    }

    this._darkQuery = window.matchMedia?.('(prefers-color-scheme: dark)') ?? null;
    this._config = {
      editable: Boolean(editable),
      placeholderText: placeholderText ?? ''
    };

    const state = EditorState.create({
      doc,
      extensions: [
        basicSetup,
        yaml(),
        createBaseTheme(),
        Prec.high(
          keymap.of([
            {
              key: 'Ctrl-Enter',
              run: () => {
                this.onValidate?.();
                return true;
              }
            },
            {
              key: 'Mod-Enter',
              run: () => {
                this.onValidate?.();
                return true;
              }
            }
          ])
        ),
        this._themeCompartment.of(this._resolveTheme()),
        EditorView.editable.of(true),
        this._readOnlyCompartment.of(EditorState.readOnly.of(!this._config.editable)),
        this._placeholderCompartment.of(codeMirrorPlaceholder(this._config.placeholderText)),
        EditorView.updateListener.of((update) => {
          if (!update.docChanged) {
            return;
          }
          this.onChange?.(update.state.doc.toString());
        })
      ]
    });

    this.view = new EditorView({
      state,
      parent
    });

    const handleDarkChange = () => this._applyConfig();

    if (this._darkQuery?.addEventListener) {
      this._darkQuery.addEventListener('change', handleDarkChange);
    } else if (this._darkQuery?.addListener) {
      this._darkQuery.addListener(handleDarkChange);
    }
  }

  requestMeasure() {
    this.view?.requestMeasure();
  }

  destroy() {
    if (!this.view) {
      return;
    }
    this.view.destroy();
    this.view = null;
  }

  setDoc(value, { addToHistory = false } = {}) {
    const view = this.view;
    if (!view) {
      return;
    }

    const nextValue = value ?? '';
    const currentValue = view.state.doc.toString();
    if (nextValue === currentValue) {
      return;
    }

    view.dispatch({
      changes: { from: 0, to: currentValue.length, insert: nextValue },
      annotations: Transaction.addToHistory.of(Boolean(addToHistory))
    });
  }

  syncConfig({ editable, placeholderText } = {}) {
    if (editable !== undefined) {
      this._config.editable = Boolean(editable);
    }

    if (placeholderText !== undefined) {
      this._config.placeholderText = placeholderText ?? '';
    }

    this._applyConfig();
  }

  _applyConfig() {
    const view = this.view;
    if (!view) {
      return;
    }

    view.dispatch({
      effects: [
        this._themeCompartment.reconfigure(this._resolveTheme()),
        this._readOnlyCompartment.reconfigure(EditorState.readOnly.of(!this._config.editable)),
        this._placeholderCompartment.reconfigure(codeMirrorPlaceholder(this._config.placeholderText))
      ]
    });
  }

  _resolveTheme() {
    if (isDarkMode(this._darkQuery)) {
      return oneDark;
    }

    return syntaxHighlighting(defaultHighlightStyle, { fallback: true });
  }
}
