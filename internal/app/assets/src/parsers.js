import jsyaml from 'js-yaml';

export function parseYAML(yamlText) {
  try {
    const data = jsyaml.load(yamlText, { schema: jsyaml.DEFAULT_SCHEMA });
    return { success: true, data };
  } catch (error) {
    return { success: false, error };
  }
}

export function parseEnvironmentVars(envText) {
  const variables = new URLSearchParams();
  const errors = [];

  if (!envText.trim()) {
    return { variables, errors };
  }

  const lines = envText.split(/\r?\n/);

  lines.forEach((line, index) => {
    const trimmed = line.trim();

    if (!trimmed || trimmed.startsWith('#')) {
      return;
    }

    const equalIndex = line.indexOf('=');
    if (equalIndex === -1) {
      errors.push(`Line ${index + 1}: missing '=' separator`);
      return;
    }

    const key = line.substring(0, equalIndex).trim();
    if (!key) {
      errors.push(`Line ${index + 1}: empty variable name`);
      return;
    }

    const value = line.substring(equalIndex + 1);
    variables.append(key, value);
  });

  return { variables, errors };
}
