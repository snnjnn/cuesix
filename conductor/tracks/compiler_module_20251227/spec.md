# Specification: Compiler Module

## 1. Overview

This document outlines the design for the `compiler` module. The module has two primary responsibilities:
1.  To discover and merge configuration files into a single APISIX configuration representation using custom merge rules.
2.  To provide a separate utility function to deterministically dump the merged configuration to a UTF-8 encoded YAML byte slice.

## 2. Functional Requirements

### 2.1. Main Compilation Function

-   **Input Configuration:** The main compilation function will accept a variadic list of `Source` structs. Each struct will contain:
    -   `FS fs.FS`: The filesystem to be read.
    -   `Include []string`: A list of glob patterns for file inclusion.
    -   `Exclude []string`: A list of glob patterns for file exclusion.
-   **Core Logic:**
    1.  **File Discovery:** It must walk the provided `fs.FS` filesystems and identify all files matching the include/exclude patterns.
    2.  **YAML Parsing:** It must parse the content of all matched files as YAML.
    3.  **APISIX-Aware Merge:** It must merge the parsed configurations using resource-specific rules.
        - List resources with required keys are merged by key.
        - List resources with optional ids do not generate ids; elements without ids are carried through as independent entries.
        - Repeated keys trigger a deep merge: scalars must match or be present on only one side; maps merge recursively; lists merge only when a rule exists.
-   **Output:** The function will return a merged configuration representation (e.g., `map[string]any`) on success, or an `error` if any part of the process fails.

### 2.2. YAML Dumping Function

-   A separate, public function (e.g., `DumpYAML`) will be provided.
-   **Input:** This function will accept a single merged configuration representation (e.g., `map[string]any`).
-   **Output:** It will return a `[]byte` slice containing the YAML representation of the value, or an `error`.
-   **Deterministic Output:** The YAML output must be deterministic. Keys must be sorted, and spacing/indentation must be consistent across multiple runs to ensure repeatable results. The output must be UTF-8 encoded.

### 2.3. Custom Error Type

-   A custom error type (e.g., `CompilerError`) shall be defined for logical errors originating from within the module (e.g., "no files found"), to distinguish them from underlying library errors.

## 3. Acceptance Criteria

-   The main compilation function successfully merges YAML files into a single configuration representation.
-   The compilation function applies APISIX-aware list merge rules consistently.
-   The `DumpYAML` function successfully converts the merged configuration to a YAML `[]byte` slice.
-   The output of `DumpYAML` is deterministic and UTF-8 encoded.
-   The module returns custom errors for logical failures and underlying library errors for I/O or YAML parsing/merging failures.

## 5. Merge Rules Reference

The standalone YAML lists and their merge keys:

- routes: id (optional)
- services: id (optional)
- upstreams: id (optional)
- ssls: id (optional)
- global_rules: id (required)
- consumer_groups: id (required)
- plugin_configs: id (required)
- stream_routes: id (optional)
- protos: id (optional)
- consumers: username (required)
- consumers.credentials: credential_id (required)
- plugin_metadata: plugin_name (required)

Example merge rule configuration:

```
path: /consumers
kind: list
id_attr: username
id_optional: false
allow_merge_same_id: true
children:
  /credentials:
    kind: list
    id_attr: credential_id
    id_optional: false
    allow_merge_same_id: false
```

## 4. Out of Scope

-   Watching for file changes (hot-reloading) is not part of this module's responsibility.
