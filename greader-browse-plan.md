# Google Reader-Compatible Browse Mode

## Summary
Implement a new optional remote source mode that logs into a Google Reader-compatible server using:
- base API URL
- login
- password

The first target is FreshRSS, but the config and client should be generic enough for other Google Reader-compatible servers that honor the same login and read-only endpoints.

Estimated effort:
- Thin proof of concept: `2-3` days
- Solid v1 integrated into Tide's current UI: `4-7` days

This is much easier than full sync because v1 can stay read-only and skip mutation tokens, unread-state writes, subscription editing, and bidirectional sync.

## Key Changes
- Add a new source config alongside the current local/direct RSS flow.
  - Proposed shape:
    - `source.type = "local" | "greader"`
    - `source.greader_url`
    - `source.greader_login`
    - `source.greader_password`
- Add a small Google Reader-compatible client package responsible for:
  - `ClientLogin` authentication
  - storing/reusing the auth token in memory
  - calling read-only endpoints for:
    - subscription list
    - unread counts
    - stream/article contents
- Keep FreshRSS-first assumptions for v1:
  - require the user to provide the full API URL such as `https://host/api/greader.php`
  - use the documented `Authorization: GoogleLogin auth=...` flow
  - expect FreshRSS-compatible JSON shapes first, while keeping request construction generic
- Integrate the remote source into the existing Tide model without replacing the local DB architecture all at once:
  - feed pane shows remote subscriptions instead of local DB feeds when `source.type = "greader"`
  - article list loads from stream contents API instead of local article rows
  - article content can initially use the summary/content/link fields returned by the API; only fetch the article URL if the API payload is insufficient
- Keep v1 read-only:
  - no mark-read writebacks
  - no starring
  - no subscribe/unsubscribe
  - no token endpoint for modifying requests

## Implementation Changes
- Networking layer:
  - add a dedicated Google Reader API client with login, auth-header injection, and JSON decoding
  - isolate endpoint paths so alternate compatible servers can be tried later without touching UI code
- Data mapping:
  - define internal lightweight remote models for subscription, article summary, and article detail
  - adapt UI-facing data at the boundary instead of forcing the current SQLite schema to absorb remote IDs immediately
- UI/config:
  - add settings fields for source type, API URL, login, and password
  - show clear auth/network failures in the existing status/error surfaces
  - when `greader` is active, the feed/article panes browse the remote source rather than the local DB-backed source
- Compatibility posture:
  - build for FreshRSS first
  - keep naming generic (`greader`, not `freshrss`) so other servers may work if they support the same endpoints and auth flow
  - do not promise universal compatibility in v1

## Test Plan
- Unit tests for the Google Reader client:
  - successful `ClientLogin`
  - auth failure handling
  - subscription list parsing
  - unread count parsing
  - article/stream content parsing
- Integration-style tests with mocked HTTP responses:
  - feed pane population from remote subscriptions
  - article list population from remote stream contents
  - auth/network errors surface cleanly without crashing the UI
- Manual scenarios:
  - FreshRSS login with valid credentials
  - invalid password
  - unreachable API URL
  - browse subscriptions and open article content from a real FreshRSS instance

## Assumptions
- V1 is browse-only, read-only.
- The user supplies the full Google Reader-compatible endpoint URL, not just the site root.
- FreshRSS is the reference server for correctness; other compatible servers are best-effort unless explicitly tested later.
- Existing direct RSS/local DB behavior remains available as a separate mode rather than being removed.
