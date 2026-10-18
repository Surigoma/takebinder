# Releasing

[日本語](releasing.ja.md)

Release Please derives versions and `CHANGELOG.md` entries from Conventional Commits on `main`.

Full CI runs on pull requests and again when release packages are built, without repeating ordinary CI on pushes to `main`. Branch protection requires PR checks against an up-to-date base before merging. The Release workflow still runs Release Please on each push to `main` to maintain the release PR; its build, verification, and asset upload jobs run only when a release is created.

1. Give every pull request a Conventional Commit title such as `feat: add capture filter`, `fix(ui): keep settings visible`, or `docs: clarify installation`. A breaking change uses `feat!:` or `BREAKING CHANGE:` in the squash commit body.
2. Merge with squash. GitHub uses the pull request title as the commit subject, so `main` retains one Conventional Commit per pull request.
3. Release Please opens or updates a release pull request containing the next version, `package.json`, and `CHANGELOG.md`. With the built-in `GITHUB_TOKEN`, approve that pull request's CI run in GitHub when prompted.
4. Review and merge the release pull request. Release Please creates the version tag and GitHub Release, then the same workflow rebuilds and smoke-tests Windows, macOS, and Linux packages and attaches them with `SHA256SUMS.txt`.

Each portable package includes `README.md` and `README.ja.md`. Both files must retain the GoPro trademark attribution and clearly state that the application is an independent, unofficial fan-made project.

Do not edit generated changelog sections, version tags, or release versions manually. Use `Release-As: x.y.z` in a Conventional Commit body when a specific next version is required.

The portable applications are not currently code-signed or notarized. Add signing only after the required platform certificates and protected release secrets are available.
