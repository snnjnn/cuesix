# Plan: Implement compiler module

This plan outlines the phases and tasks required to implement the compiler module as defined in `spec.md`.

---

## Phase 1: Core Compiler Implementation

This phase focuses on building the core functionality of the compiler: discovering files and unifying them into a `cue.Value`.

- [x] **Task:** Define the `Source` struct and the custom `CompilerError` type in a new `internal/compiler/compiler.go` file. [b18ccc9]
- [x] **Task:** Implement the file discovery logic. [3c0782a]
    - [x] **Sub-task:** Write failing tests in `internal/compiler/compiler_test.go` that cover various scenarios for including and excluding files based on glob patterns.
    - [x] **Sub-task:** Implement the file discovery logic within the main compile function to make the tests pass.
-   [ ] **Task:** Implement the CUE unification logic.
    -   [ ] **Sub-task:** Write failing tests that provide multiple CUE file contents and verify they are unified correctly into a single `cue.Value`. Include a test case for `@embed` directive support.
    -   [ ] **Sub-task:** Implement the CUE unification logic to make the tests pass.
-   [ ] **Task:** Conductor - User Manual Verification 'Core Compiler Implementation' (Protocol in workflow.md)

---

## Phase 2: YAML Dumping Utility

This phase focuses on creating the utility to convert a `cue.Value` into a deterministic YAML byte slice.

-   [ ] **Task:** Implement the `DumpYAML` function.
    -   [ ] **Sub-task:** Write failing tests in `internal/compiler/compiler_test.go` to verify that a given `cue.Value` is converted to a YAML `[]byte` slice with sorted keys and consistent formatting.
    -   [ ] **Sub--task:** Implement the `DumpYAML` function to make the tests pass.
-   [ ] **Task:** Conductor - User Manual Verification 'YAML Dumping Utility' (Protocol in workflow.md)
