# Wabi-sabi visual refresh

The first Phase 3 design pass uses warm paper, olive navigation, distinct financial colors, generous spacing, lightly varied corners, and serif headings. Financial values and controls remain precise. This implements the shared visual foundation for Web issue #94; guest access, account lifecycle, and the full dashboard chart redesigns remain separate work.

## Theme and components

- `static/css/wabi-sabi.css` is loaded after the existing component/layout stylesheet. It owns the visual tokens and shared component treatment; `app.css` retains existing interaction and page-specific behavior.
- The explicit `ournezt` theme keeps the current palette consistent across system light/dark preferences. Fonts use local system sans-serif and serif stacks; no external fonts, scripts, or image services are required.
- `--oz-paper`, `--oz-surface`, `--oz-ink`, `--oz-line`, `--oz-olive`, and `--oz-clay` are the primary tokens. Existing coral/leaf/sky aliases remain available for older components.
- `oz-page-heading`, `oz-eyebrow`, `oz-section-intro`, `oz-metrics`, and `oz-metric` provide the shared page and overview hierarchy.
- The workspace, navigation and footer use the available window width with shared gutters of 16–48 px. Data-heavy pages are not capped at a narrow reading width. The sign-in composition has a separate 80 rem limit, and long prose and individual forms retain their own readable widths.
- Body and input text use a 16 px baseline; supporting copy, controls and table text have a larger readable scale. Navigation uses 16 px semibold labels with small line icons inside a soft segmented capsule; the current page has a raised paper-colored segment and olive text. A compact wordmark, circular account control and rounded mobile menu keep the header light. The menu collapses at 1000 px to keep larger labels from crowding the header.
- Navigation uses real request paths for active state, including nested people/housing routes. Desktop and mobile use the same navigation partial and preserve account/admin/password-change restrictions.
- The small house still life and paper-grain SVG are decorative. Inputs, forms, tables, comparison cards and data labels remain the functional focus.
- Planning pages use short functional titles with small icons instead of introductory slogans. Metric cards use restrained blue, coral, jade and lilac tints. Charts use distinct blue income, coral costs, jade surplus, amber upfront payments and violet projected/later amounts; legends, dashed projection lines and signed values preserve meaning beyond color alone. The housing comparison uses horizontal grouped bars with the existing monthly cost/surplus data.

## Behavior retained and improved

The existing authenticated routes, form field names, CSRF fields, CSP nonces, Core calculations, role checks and financial datasets remain in place. The dashboard now groups the current monthly figures and labels their units; its chart types and calculation semantics still need the dedicated dashboard issues.

The first-visit tour prompt no longer interrupts planning. The footer's Quick tour action starts the existing tour explicitly and keeps the current household query parameters. An explicit request can start a tour even if automatic tips were previously dismissed. Required password-change guidance is preserved.

The household form uses visible input labels and consistent household wording. An existing dependency-failure regression check was updated to recognize the new empty-state message; it still verifies that service errors are not presented as an empty household list.

## Accessibility and review

The theme includes a skip link, active-page attributes, visible keyboard focus, reduced-motion handling, wrapping labels, and mobile navigation. Representative token checks: secondary text on paper 4.63:1, primary button text 6.68:1, input borders 3.21:1, and placeholder text 4.76:1. These checks do not replace a complete accessibility audit.

Visual QA uses the actual templates and fictional sample data without a production database. Desktop (1440 px), tablet (820 px), phone (390 px) and small-phone (320 px) review covers sign-in, populated/empty overview, housing list/comparison/forms, people/forms, households, account and FAQ. Keep generated screenshots and fixture servers out of the shipped application.

Before completing all of #94, review this direction with users and apply the agreed guest/onboarding contracts when those APIs exist. Dashboard comprehension testing and the three chart replacements belong to their linked issues.
