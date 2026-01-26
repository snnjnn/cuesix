import { ENDPOINTS, MODES, SAMPLE_SOURCE } from './constants.js';
import { parseEnvironmentVars, parseYAML } from './parsers.js';

async function fetchJsonOrThrow(url, options = {}) {
  const response = await fetch(url, options);

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`HTTP ${response.status}: ${errorText || response.statusText}`);
  }

  return response.json();
}

function normalizeErrors(errors) {
  if (!Array.isArray(errors) || errors.length === 0) {
    return [];
  }

  return errors.map((error) => {
    const location = error.instanceLocation || error.dataPath || '(root)';
    const message = error.message || 'validation failed';
    return { location, message };
  });
}

export class ValidationService {
  constructor({
    endpoints = ENDPOINTS,
    modes = MODES,
    sampleSourceValue = SAMPLE_SOURCE.value
  } = {}) {
    this.endpoints = endpoints;
    this.modes = modes;
    this.sampleSourceValue = sampleSourceValue;
  }

  async validate({ mode, yamlText, envText, currentPath, onState }) {
    const yamlValue = yamlText || '';
    const envValue = envText || '';

    const emit = (state) => {
      onState?.(state);
      return state;
    };

    if (!yamlValue.trim()) {
      if (mode === this.modes.playground) {
        return emit({
          statusType: 'warning',
          statusText: 'Enter YAML to validate',
          errors: []
        });
      }

      return emit({
        statusType: 'ok',
        statusText: 'Ready',
        errors: []
      });
    }

    const yamlResult = parseYAML(yamlValue);
    if (!yamlResult.success) {
      const error = yamlResult.error;
      const line = error.mark?.line + 1;
      const column = error.mark?.column + 1;
      const message = line ? `Line ${line}, Column ${column}: ${error.message}` : error.message;

      return emit({
        statusType: 'error',
        statusText: 'YAML syntax error',
        errors: [{ location: '(root)', message }]
      });
    }

    const envResult = parseEnvironmentVars(envValue);
    if (envResult.errors.length > 0) {
      return emit({
        statusType: 'error',
        statusText: 'Environment variable errors',
        errors: envResult.errors.map((message) => ({ location: '(root)', message }))
      });
    }

    const canValidateByPath =
      mode === this.modes.browse && currentPath && currentPath !== this.sampleSourceValue;

    emit({
      statusType: 'warning',
      statusText: 'Validating…',
      errors: []
    });

    try {
      let result;
      if (canValidateByPath) {
        result = await fetchJsonOrThrow(
          `${this.endpoints.validate}/${currentPath}?${envResult.variables}`,
          {
            method: 'GET'
          }
        );
      } else {
        result = await fetchJsonOrThrow(`${this.endpoints.validate}?${envResult.variables}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(yamlResult.data || {})
        });
      }

      if (result.valid) {
        return emit({
          statusType: 'ok',
          statusText: 'Valid',
          errors: []
        });
      }

      const errors = normalizeErrors(result.errors);
      return emit({
        statusType: 'error',
        statusText: `Found ${errors.length} validation issue(s).`,
        errors
      });
    } catch (error) {
      return emit({
        statusType: 'error',
        statusText: error.message,
        errors: [{ location: '(root)', message: error.message }]
      });
    }
  }
}
