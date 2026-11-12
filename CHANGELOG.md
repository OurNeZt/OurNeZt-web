# Changelog
All notable changes to this project will be documented in this file.
 
The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

---

## [v2.0.0] - 2026-11-13

### Added

- CSRF protection for state-changing requests, including login, logout, and authenticated forms, with a reload page for rejected submissions.
- Configurable HTTP connection timeouts and a global request body limit, including bounded multipart parsing.
- A shared Core RPC deadline budget and safe timeout/service-unavailable pages that preserve unsaved housing notes, checklist criteria, and evaluation answers.
- Inline whitespace feedback and clear Core password-policy messages for user creation and password changes.
- Self-hosted, pinned Tailwind CSS, DaisyUI, and Chart.js assets with committed licenses, checksums, and CI verification of generated files.
- Browser security headers covering pages, assets, redirects, and application errors, including nonce-based Content Security Policy, frame protection, MIME sniffing protection, referrer and permissions policies, and HSTS for direct HTTPS.
- Reviewer commands and regression coverage for request limits, RPC deadlines, session revocation, login responses, CSRF, password validation, transport security, and browser headers.

### Changed

- Production startup now requires a persistent `CSRF_AUTH_KEY` containing 32 random bytes encoded as 64 hexadecimal characters. Share the same secret across Web replicas.
- Production Core connections now require verified TLS unless `CORE_ALLOW_INSECURE=true` explicitly permits a trusted plaintext hop. Invalid TLS settings fail startup, certificate-verification bypass is forbidden in production, and failed TLS connections never downgrade to plaintext.
- Logout now revokes the session in Core before clearing the browser cookie. When revocation cannot be confirmed, the cookie is retained and the user can retry.
- Successful password changes clear the browser session and require a fresh login after Core revokes all sessions for the account.
- Every Core RPC uses `REQUEST_TIMEOUT`, defaulting to five seconds, with one budget shared by session validation and subsequent calls in each HTTP request.
- HTTP defaults now bound headers to 10 seconds, complete reads to 15 seconds, writes to 30 seconds, idle connections to 60 seconds, and request bodies to 1 MiB. Authentication forms retain the smaller 16 KiB limit.
- Scripts now require per-response CSP nonces; inline event handlers, inline style attributes, external scripts/styles, frames, and objects are blocked. HTML responses use `Cache-Control: no-store`.

### Fixed

- Prevented login responses from revealing whether an account exists, is disabled, or has an incorrect password.
- Preserved exact login and current-password values instead of silently trimming credentials; new passwords containing whitespace are rejected before submission to Core.
- Preserved session cookies and editable drafts during Core timeouts or outages, and prevented optional RPC failures from rendering incomplete data as a successful page.
- Made GitHub Actions cache export best-effort so cache reservation failures do not fail a release after its image is built and pushed. Build and image push failures still fail the workflow.

### Removed

- Runtime CDN dependencies for frontend scripts and styles.

### Upgrade Notes

- Deploy the matching Core 2.0 release before Web so server-side logout revocation and password-policy handling are available.
- Before upgrading production, supply `CSRF_AUTH_KEY`, retain HTTPS and secure cookies, and configure verified Web-to-Core TLS or the explicit trusted plaintext opt-ins on both services. Existing production configuration may otherwise fail startup.
- When TLS terminates at ingress, configure HSTS and HTTP-to-HTTPS redirects there. Preserve the application's CSP header so its nonce matches the rendered page.
- Core 2.0 list APIs return 50 records by default. This Web release does not follow `next_page_token`; affected lists, including housing comparisons, omit later pages. Update the client before deploying where collections can exceed 50 records.

## [v1.4.0] - 2026-10-09

### Added

- Family housing checklist management for creating, editing, reordering, weighting, and deleting criteria.
- Per-housing evaluation forms with answer states, ratings, and criterion-specific notes.
- Evaluation scores and completion progress in housing lists, detail pages, dashboard cards, and comparison views.
- Responsive comparison layouts with expandable evaluation criteria and notes.
- Shared housing notes editor with a character counter and a 10,000-character limit.
- Save confirmation, validation feedback, and recovery pages for housing notes.
- Configurable authentication throttling by browser IP and account, with concurrent request limits.
- HTTP 429 retry pages with `Retry-After` and `Cache-Control: no-store` headers.
- Explicit trusted proxy configuration through `WEB_TRUSTED_PROXIES`.
- `auth-rate-check` command for verifying account throttling, IP limits, and recovery.
- Regression tests for checklist forms, draft preservation, housing notes, and authentication throttling.

### Changed

- Generated gRPC bindings now support housing checklist evaluations and shared notes.
- Login, password changes, and admin user creation are throttled before downstream authentication work.
- Password-change routes share the same account rate-limit budget.
- Authentication forms are limited to 16 KiB, and authentication RPCs use request deadlines.
- Login failures display generic messages instead of raw Core authentication errors.
- Core retry delays are preserved when authentication requests are throttled.

### Fixed

- Preserved evaluation drafts when validation or saving fails.
- Preserved housing notes drafts when saving fails.
- Prevented untrusted forwarding headers from bypassing browser IP throttling.
- Kept shared housing notes saves separate from housing details, checklist ratings, and affordability calculations.

### Removed

- No removals in this release.

## [v1.3.0] - 2026-08-04

### Added
- Housing group management UI for creating, renaming, and deleting housing groups.
- Group assignment controls for moving housing options into and out of groups.
- Per-housing-option dashboard visibility controls in housing management and housing form flows.
- Group-level bulk show/hide actions for dashboard visibility.
- Grouped housing display with support for ungrouped housing options.
- Visible and hidden housing status summaries across housing management and comparison views.

### Changed
- Dashboard charts, timeline, and housing comparison now show only housing options marked as visible.
- Housing form now includes dashboard visibility and housing group selection.
- Housing management page now acts as the main UI for grouping and dashboard visibility control.

### Fixed
- Dashboard visibility checkbox persistence when saving housing options.
- Tooltip rendering and positioning for animated housing form fields.
- Move button behavior so it stays disabled until a different housing group is selected.

### Removed
- No removals in this release.

## [v1.2.0] - 2026-06-26

### Added
- Added FAQ page for housing eligibility and planning questions.
- Added pagination to the Admin Dashboard user table.
- Added required field indicators and validation to the Add Housing form.
- Added configurable maintenance notice banner for planned downtime messaging.
- Added dynamic maintenance notice support through environment variables or a JSON file.
- Added Helm ConfigMap wiring for updating maintenance notice copy without rebuilding the web image.

### Changed
- Updated loan tenure limits based on MAS rules.
- Improved Add Housing form guidance and validation feedback.
- Improved maintenance banner styling for desktop and mobile readability.

### Fixed
- Hid or disabled HDB-only fields for EC and private condo housing types.

### Removed
- No removals in this release

## [v1.1.4] - 2026-06-17

### Added
- Mobile-friendly card layout for Housing Comparison on narrow screens.

### Changed
- Updated Housing Comparison table styling for better readability on horizontal/tablet screens.
- Renamed the ambiguous “Upfront” housing comparison label to “Downpayment + Fees.”

### Fixed
- Fixed Housing Comparison columns wrapping vertically and becoming unreadable on mobile viewports.
- Fixed unclear upfront cost wording by clarifying that the value includes downpayment, legal fees, and buyer stamp duty.

### Removed
- No changes in this release.

## [v1.1.3] - 2026-06-15

### Added
- Added contextual help tooltips across key housing form fields, calculated values, and housing detail views
- Added centralized housing tooltip content for easier copy updates and maintenance
- Added official reference links in selected tooltips for buyer stamp duty, housing grants, and HDB interest rate
- Added initial payment visibility in housing detail, comparison, and timeline views

### Changed
- Updated housing loan tenure input from months to years in the Add/Edit Housing form
- Updated selected system-derived housing fields to use placeholder or read-only styling instead of appearing user-entered
- Updated `BTO + HDB loan` tenure handling to auto-assume 25 years
- Renamed the first housing payment checkpoint from `Initial Downpayment` to `Total Initial Payment`
- Updated the Payment vs Income Timeline chart labels, legends, and stage presentation for clearer payment journey tracking
- Refined tooltip styling and behavior to be more minimal, consistent, and mobile-friendly

### Fixed
- Fixed housing tooltip positioning, clipping, visibility, and interaction issues across desktop and mobile layouts
- Fixed tooltip link behavior so “See more” references are clickable and do not close incorrectly
- Fixed grant amount handling to better reflect system-derived calculation behavior in the housing form
- Fixed initial payment display logic so the due-now checkpoint aligns with the intended affordability flow
- Fixed total initial payment display to include buyer stamp duty while keeping later downpayment stages separate
- Fixed form submission issues for system-derived `BTO + HDB loan` tenure values
- Fixed placeholder/value behavior so empty or default housing fields no longer appear as manually entered values

### Removed
- Removed misleading prefilled-looking default values from selected housing inputs
- Removed inconsistent tooltip presentation across different housing sections

## [v1.1.2] - 2026-06-01

### Added
- Optional TLS startup support for web server via `WEB_TLS_CERT_FILE` and `WEB_TLS_KEY_FILE`.
- Optional TLS support for web-to-core gRPC via:
    - `CORE_GRPC_USE_TLS`
    - `CORE_GRPC_TLS_CA_FILE`
    - `CORE_GRPC_TLS_SERVER_NAME`
    - `CORE_GRPC_TLS_INSECURE_SKIP_VERIFY`
- Mobile/tablet tour improvements:
    - Auto-open hamburger menu for nav-link guide steps.
    - Better orientation/resize/scroll reflow behavior for guide positioning.

### Changed
- Core gRPC client initialization now accepts full app config to support secure transport options.
- Session cookie security default behavior tightened for production-oriented usage.
- Guide placement logic now prefers non-overlapping positions near highlighted targets.

### Fixed
- Tour popup layering/cutoff issues on small screens and tablets.
- Tour highlight failing on mobile nav items (hidden desktop link vs hamburger menu link mismatch).
- Payment-vs-income timeline selector update reliability on dashboard.
- Dashboard gross income display consistency with household summary updates.

### Removed
- Shadow/dim overlay spotlight approach for tour highlights (reverted to attached highlight style).


## [v1.1.1] - 2026-06-01

### Added
- Mobile/tablet tour behavior improvements for responsive layouts and orientation changes.
- Automatic mobile hamburger-menu handling during tour steps so nav-item highlights can appear correctly.

### Changed
- Family creation now uses controlled family-type selection (`single`, `couple`, `family`) instead of free-form input.
- Housing `unit type` options standardized to room/category-based values (removed `-bedder` style options).
- Dashboard timeline chart housing selector behavior updated to switch chart data reliably.

### Fixed
- Create Family no longer rejects uppercase/mixed-case input unexpectedly.
- Dashboard gross income now correctly includes all relevant users, fixing dependent card calculations.
- Mobile tips/guide layering and positioning issues:
    - tour no longer gets cut off
    - z-index/placement issues on phone/tablet and smaller landscape screens are fixed
    - highlight and guide alignment improved across scroll/orientation
- Payment vs Income Timeline now updates correctly when switching housing options.

### Removed
- Redundant/free-text family type entry path in create-family flow.
- `-bedder` style unit-type choices from housing form.

## [v1.1.0] - 2026-05-31

### Added
- Per-housing DIA income overrides so each housing option stores its own projected income inputs by person.
- New database migration to support housing-level DIA overrides (000004_housing_dia_overrides).
- Housing-aware dashboard scenario impact logic that computes projected take-home per housing option.

### Changed
- DIA flow now uses housing-specific projected incomes instead of globally overwriting shared person profile future income.
- Dashboard housing impact and payment timeline now evaluate affordability with per-option projected income when DIA data exists.
- Protobuf/contracts and server mapping updated to include DIA override payloads on housing options.
- Documentation updated to reflect the DIA behavior and latest profile/update flow expectations.

### Fixed
- Personal finance profile edit failure caused by backend SQL placeholder mismatch in UpdatePersonProfile.
- DIA sync/update failures related to profile update path and legacy linked-user compatibility handling.
- Form/input handling reliability for DIA income updates and profile updates.
- Lint issue (SA9003 empty branch) in housing projection logic.

### Removed
- Legacy behavior that tied DIA projected income updates directly to shared person profile updates during housing save.

## [v1.0.0] - 2026-05-29

### Added
- Role-based web experience with separate admin and user navigation paths.
- Admin dashboard and user management page (create/list/disable user workflows).
- User profile/account management (account details update and password change flow).
- Household planning modules for families, profiles (people), housing options, and housing comparison.
- Dashboard charts for income and housing impact/timeline.
- First-time onboarding flow with interactive tips/tour and per-user preference persistence.
- Footer and shared layout components aligned to Phase 1 branding.

### Changed
- Refactored web flows to align with latest OurNeZt-core contract and user detail updates.
- Improved housing forms and affordability flow (including DIA-related behavior and projected-income handling).
- Updated forms to use clearer input behavior (dropdowns, required fields, date inputs, money formatting patterns).
- Updated responsive behavior across key pages (navigation, forms, actions, and chart containers).
- Aligned CI/workflow and lint expectations for release readiness.

### Fixed
- Admin and user routing/visibility inconsistencies.
- First-login password-change prompt and onboarding behavior issues.
- Multiple mobile UI issues:
    - Hamburger menu layering/interaction issues.
    - Edit Housing page overflow/styling problems.
    - Table/action button wrapping issues on small screens.
    - Footer positioning inconsistencies on short pages.
- Lint findings and related code-quality issues from CI checks.

### Removed
- Redundant admin dashboard bootstrap guidance after login.
- Admin-facing access to non-admin planning sections in navbar/dashboard context.
