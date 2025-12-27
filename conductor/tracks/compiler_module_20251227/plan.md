# Plan: Implement compiler module

This plan outlines the phases and tasks required to implement the compiler module as defined in `spec.md`.

---

## Phase 1: Core Compiler Implementation

This phase focuses on building the core functionality of the compiler: discovering files and merging them into a single APISIX configuration representation.

- [x] **Task:** Define the `Source` struct and the custom `CompilerError` type in a new `internal/compiler/compiler.go` file. [b18ccc9]
- [x] **Task:** Implement the file discovery logic. [3c0782a]
    - [x] **Sub-task:** Write failing tests in `internal/compiler/compiler_test.go` that cover various scenarios for including and excluding files based on glob patterns.
    - [x] **Sub-task:** Implement the file discovery logic within the main compile function to make the tests pass.
- [ ] **Task:** Implement the APISIX-aware merge logic.
    - [ ] **Sub-task:** Write failing tests that provide multiple YAML file contents and verify they are merged correctly for list resources with required keys (e.g., consumers by username).
    - [ ] **Sub-task:** Write failing tests that verify list resources with optional ids carry id-less entries through without generating ids.
    - [ ] **Sub-task:** Write failing tests that verify repeated ids trigger deep merge and fail when scalars conflict or list rules are missing.
    - [ ] **Sub-task:** Implement the merge logic to make the tests pass.
-   [ ] **Task:** Conductor - User Manual Verification 'Core Compiler Implementation' (Protocol in workflow.md)

---

## Phase 2: YAML Dumping Utility

This phase focuses on creating the utility to convert the merged configuration into a deterministic YAML byte slice.

-   [ ] **Task:** Implement the `DumpYAML` function.
    -   [ ] **Sub-task:** Write failing tests in `internal/compiler/compiler_test.go` to verify that a merged configuration is converted to a YAML `[]byte` slice with sorted keys and consistent formatting.
    -   [ ] **Sub--task:** Implement the `DumpYAML` function to make the tests pass.
-   [ ] **Task:** Conductor - User Manual Verification 'YAML Dumping Utility' (Protocol in workflow.md)

## Completion Note

This track was revised. The original unification approach was replaced because APISIX configuration lists require custom merge rules that are incompatible with list-by-position semantics.
