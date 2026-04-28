# Org-Level `.env` Secrets CLI Plan

## Goal

Build a professional-grade CLI and TUI tool to manage organization-level `.env` secrets with encrypted storage, biometric-gated sensitive actions, Git-friendly backups, and multi-organization support.

## Proposed Tech Stack

- Go
- Cobra for CLI command structure
- Bubble Tea for TUI application flow
- Bubbles for reusable terminal UI components
- Lip Gloss for styling and layout

## Product Scope

The tool should support:

- Organization setup by providing:
  - a path where repositories live
  - an output path for encrypted secret storage
- Encryption of secrets in the destination location
- Use of Mac fingerprint authentication for sensitive actions such as:
  - restore
  - show secrets
  - backup
- Comparison of stored secrets with current `.env` contents
- Display of `.env` and backup timestamps to inform further actions
- Management of multiple organizations
- A TUI with a help menu for shortcuts and actions
- Per-organization master keys
- A Git-friendly store layout with per-file encrypted files to support incremental updates and backups

## Initial Product Plan

### 1. Product Definition

Define the exact operator workflows:

- bootstrap an organization
- scan repositories
- import `.env` files
- compare stored secrets against current repo files
- redot vault
- reveal secrets
- back up secrets
- rotate keys

Define the trust model:

- which actions require biometric approval
- whether short-lived unlocked sessions are allowed
- what plaintext is allowed in memory
- what metadata is safe to persist unencrypted

Define v1 platform support so biometric integration is implemented intentionally.

### 2. Core Architecture

Build the tool as a standalone Go module with clear package boundaries:

- `cmd/`
  - Cobra commands and CLI entrypoints
- `internal/tui/`
  - Bubble Tea screens, keymaps, actions, and navigation
- `internal/store/`
  - encrypted on-disk store layout and metadata handling
- `internal/crypto/`
  - envelope encryption, master key handling, per-file key handling
- `internal/orgs/`
  - organization definitions, repo discovery, and config management
- `internal/diff/`
  - `.env` parsing, normalization, and comparison
- `internal/biometric/`
  - macOS Touch ID / secure action authorization

Use a per-organization master key with per-file data keys so encrypted files can be updated independently and committed incrementally.

### 3. Storage and Security Design

Maintain local config that tracks:

- organizations
- repo root path
- backup output path
- authentication policy
- active organization

Store encrypted secrets per repository and per env file, along with metadata such as:

- source path
- last imported timestamp
- last backup timestamp
- content fingerprint
- key version

Design the store so each encrypted file changes independently and remains Git-friendly.

### 4. UX Design

Provide both CLI and TUI workflows.

CLI should cover:

- automation
- scripting
- CI-safe read-only operations where appropriate

TUI should cover:

- organization picker
- repository list
- env file list
- status badges for drift, missing backups, and stale secrets
- timestamp columns
- compare view
- guarded actions for reveal, restore, and backup
- help panel with shortcuts

Lip Gloss should provide a polished but restrained visual design suitable for day-to-day terminal usage.

### 5. Implementation Phases

#### Phase 1: Foundation

- scaffold the Go project
- define config models
- implement organization setup
- implement encrypted store read/write

#### Phase 2: Secret Workflows

- implement repository scanning
- import `.env` files
- build the comparison engine
- add timestamps and backup flow

#### Phase 3: TUI

- build primary screens
- add shortcuts and contextual help
- add filtering and action confirmations

#### Phase 4: Security and Advanced Operations

- add macOS biometric authorization
- add key rotation
- add export/import behavior where needed
- harden sensitive action handling

#### Phase 5: Quality and Release

- add unit and integration tests
- add fixtures for `.env` parsing and diff validation
- add release packaging
- write operator documentation

## Open Questions

These need to be clarified before implementation starts:

1. Is v1 macOS-only, or should Linux also be supported without biometric features?
Cross-platform support ask for master key if biometric gating isn't available. 

2. Should the tool manage only `.env`, or also variants like `.env.local`, `.env.production`, and `.env.staging`?
Also include those is present. Consult .gitgnore for extra patterns 

3. What defines an organization:
   - one top-level repo root
   - multiple repo roots
   - or an arbitrary named repo collection?
Just a path where an organization repos live. 

4. What should be the primary key strategy:
   - tool-managed master key in macOS Keychain / secure storage
   - passphrase-derived key
   - or both?
Encrypted master key in keychain 

5. Should Touch ID gate every sensitive action, only reveal/restore/export actions, or unlock a short-lived session?
Unlock a session for 15 minutes after biometric approval, then require re-authentication for further sensitive actions.

6. Should the encrypted destination store be:
   - one Git repo per organization
   - one shared repo with subfolders per organization
   - or any filesystem path with a Git-friendly layout?
filesystem path with Git-friendly layout, so users can choose to put it in a repo or not.

7. Should compare view include:
   - key presence only
   - key and value changes
   - masked secret values by default?
Include diff for all .env contents. 

8. Should backups be:
   - point-in-time encrypted snapshots
   - latest-only state plus Git history
   - or both?
Only backups if .env contents have changed since last backup, to avoid redundant commits.

9. Is team-sharing part of v1, or is this a local operator tool with Git as transport?
Not for v1, but design should allow for future team-sharing features using Git as transport.

10. Should restore write directly into repo `.env` files or stage to a preview/temp 
location first?
Directly into repo .env files


## Recommended Next Step

The open questions above have now been answered. The v1 product direction is:

- local operator tool first, with future team sharing possible through Git-backed store transport
- filesystem-backed encrypted store path, optionally placed inside a Git repository
- one configured organization maps to one repo root path and one encrypted store root path
- `.env` plus present `.env.*` variants are included, with `.gitignore` consulted for extra env-like files
- master keys are stored through the platform keychain/keyring backend
- sensitive actions unlock an organization-scoped 15 minute session
- compare output includes full env value differences, so compare is treated as a reveal-style sensitive action
- restore writes directly into the target repository env file
- backup snapshots are created only when encrypted content has changed since the previous backup

## Implementation Progress

### Completed

- Go module scaffolded with Cobra, Bubble Tea, Bubbles-compatible model patterns, and Lip Gloss styling.
- Organization setup implemented with repo root, encrypted store root, active organization, auth policy, and key backend metadata.
- Repository scanning implemented for nested Git repositories and env file discovery.
- Encrypted store read/write implemented with per-file data keys wrapped by a per-organization master key.
- Git-friendly per-record encrypted JSON layout implemented under `repos/`, with incremental backup snapshots under `backups/`.
- CLI workflows implemented for:
  - `org add`
  - `org list`
  - `org use`
  - `repo scan`
  - `repo status`
  - `repo import`
  - `repo compare`
  - `repo backup`
  - `repo restore`
  - `store put`
  - `store get`
- TUI implemented with env-file rows, drift and backup badges, timestamps, filtering, help text, and guarded import/backup/restore actions.
- Sensitive-action authorization session implemented for reveal/compare, restore, and backup flows.
- macOS Touch ID authorization implemented for supported cgo builds, with keyring/passphrase fallback only when biometric auth is unavailable.
- Cross-platform passphrase fallback implemented for cases where the OS keyring is unavailable, including `DOT_VAULT_MASTER_PASSPHRASE` for non-interactive use.
- First-run setup asks for a master passphrase and keeps TUI actions non-blocking when passphrase auth is required.
- Active organization switching implemented for multi-organization workflows.
- First-run TUI setup implemented for creating the initial organization without using `org add`.
- Unit tests added for env parsing/diffing, repo scanning, org setup, encrypted store round trips, backup de-duplication, safe restore paths, TUI actions, and auth session TTL behavior.

### Remaining Phase 4/5 Work

- Add key rotation commands and migration tests.
- Add release packaging and operator documentation.
- Add integration tests that exercise CLI commands against fixture repositories and a temporary encrypted store.
