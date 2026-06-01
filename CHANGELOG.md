# Changelog
All notable changes to this project will be documented in this file.
 
The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

---

## [v1.1.1] - 2026-06-01

### Added
- (fill)

### Changed
- (fill)

### Fixed
- (fill)

### Removed
- (fill)

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
