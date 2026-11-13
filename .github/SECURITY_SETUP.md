# Security alerts and checks

- **Dependabot**: alerts and grouped security fix PRs for vulnerable Go and npm
  dependencies. Routine Go/npm version update PRs are disabled. Weekly grouped
  GitHub Actions updates target `dev` to keep CI maintained. Nothing is merged
  automatically.
- **CodeQL**: Go and JavaScript source analysis with `security-extended` queries on
  PRs into `dev` and `stable`, pushes to those branches, and weekly on the default
  branch. Vendored JavaScript in `static/vendor` is excluded from source scanning
  and covered by dependency checks. Findings appear under **Security > Code scanning**.
- **Security workflow**: daily and on PRs/pushes. Reachable Go vulnerabilities and
  npm advisories of low severity or higher fail their respective checks. npm
  checks include development dependencies because they supply shipped frontend
  assets. Findings appear in Actions logs.

## Activate on GitHub

1. Merge these files through `dev` into the default branch, currently `stable`.
   Dependabot configuration and scheduled workflows must exist on that branch.
2. In repository **Settings > Advanced Security** (also called **Code security**
   or **Code security and analysis** in some layouts), enable the dependency
   graph, Dependabot alerts, and Dependabot security updates. Adding the YAML
   configuration alone does not enable these repository settings.
3. Use the committed CodeQL workflow as advanced setup; avoid also enabling CodeQL
   default setup. Code scanning is available for this public repository; making
   it private may require GitHub Code Security licensing.
4. Run **Actions > Security > Run workflow** and **Actions > CodeQL > Run
   workflow** after merging. Verify analysis uploads and inspect the Security tab.
   Existing vulnerabilities can make the Security workflow fail immediately.
5. In personal GitHub **Settings > Notifications**, enable desired Dependabot
   email/web notifications and failed Actions notifications. Repository files
   cannot change anyone's personal notification preferences.
6. Also enable secret scanning and push protection in repository security settings
   to detect supported leaked credentials and block new leaks.

Branch protection is unchanged. Maintainers can require the new checks after
initial findings are fixed.

## Branches and fixes

Dependabot security PRs target the default branch, `stable`. Go/npm entries have no
`target-branch` because that option prevents their security-update configuration
from applying. CI also runs on PRs into `stable` to test these fixes. Bring merged
security fixes back into `dev` using the usual branch synchronization process.

Frontend updates may need regenerated committed assets. If the frontend-assets CI
job reports a diff after a Dependabot update, check out its PR branch and commit
the output of:

```sh
npm ci --ignore-scripts --no-audit --no-fund
npm run build:assets
```

## Local checks and coverage

```sh
GOTOOLCHAIN=auto go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
CGO_ENABLED=0 govulncheck ./...
npm audit --package-lock-only --include=dev --audit-level=low
```

CI scans on Linux with the latest Go 1.25 patch, matching the Docker build's minor
version. Only scanner installation enables `GOTOOLCHAIN=auto`, allowing Go to
download a newer toolchain for the scanner itself. The scan still uses the
application's Go 1.25 toolchain. Update CI and Docker together when upgrading it;
Dependabot cannot fix standard library vulnerabilities by updating Go modules.

These checks do not scan Alpine packages in the container image. Container image
scanning is a separate check. Automated scanning also does not replace security
review.

References: [Dependabot configuration](https://docs.github.com/en/code-security/concepts/supply-chain-security/about-the-dependabot-yml-file),
[security updates](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/secure-your-dependencies/configure-security-updates),
[CodeQL](https://github.com/github/codeql-action),
[govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck).
