# Specification: Compiler Module

## 1. Overview

This document outlines the design for the `compiler` module. The module has two primary responsibilities:
1.  To discover and unify configuration files into a single `cue.Value`.
2.  To provide a separate utility function to deterministically dump a `cue.Value` to a UTF-8 encoded YAML byte slice.

## 2. Functional Requirements

### 2.1. Main Compilation Function

-   **Input Configuration:** The main compilation function will accept a variadic list of `Source` structs. Each struct will contain:
    -   `FS fs.FS`: The filesystem to be read.
    -   `Include []string`: A list of glob patterns for file inclusion.
    -   `Exclude []string`: A list of glob patterns for file exclusion.
-   **Core Logic:**
    1.  **File Discovery:** It must walk the provided `fs.FS` filesystems and identify all files matching the include/exclude patterns.
    2.  **Content Unification:** It must unify the content of all matched files into a single CUE value using the CUE SDK.
    3.  **@embed Directive Support:** It must ensure that the CUE `@embed` directive is correctly handled, providing any necessary plumbing required by the CUE SDK.
-   **Output:** The function will return a `cue.Value` object on success, or an `error` if any part of the process fails.

### 2.2. YAML Dumping Function

-   A separate, public function (e.g., `DumpYAML`) will be provided.
-   **Input:** This function will accept a single `cue.Value` object.
-   **Output:** It will return a `[]byte` slice containing the YAML representation of the value, or an `error`.
-   **Deterministic Output:** The YAML output must be deterministic. Keys must be sorted, and spacing/indentation must be consistent across multiple runs to ensure repeatable results. The output must be UTF-8 encoded.

### 2.3. Custom Error Type

-   A custom error type (e.g., `CompilerError`) shall be defined for logical errors originating from within the module (e.g., "no files found"), to distinguish them from underlying library errors.

## 3. Acceptance Criteria

-   The main compilation function successfully unifies CUE/YAML files into a single `cue.Value`.
-   The compilation function correctly processes `@embed` directives.
-   The `DumpYAML` function successfully converts a `cue.Value` to a YAML `[]byte` slice.
-   The output of `DumpYAML` is deterministic and UTF-8 encoded.
-   The module returns custom errors for logical failures and underlying library errors for I/O or CUE processing failures.

## 4. Out of Scope

-   Watching for file changes (hot-reloading) is not part of this module's responsibility.
