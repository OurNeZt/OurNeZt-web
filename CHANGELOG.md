# Changelog
All notable changes to this project will be documented in this file.
 
The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

---

## [v1.1.3] - 2026-06-15

### Added
- (fill)

### Changed
- (fill)

### Fixed
- (fill)

### Removed
- (fill)

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
