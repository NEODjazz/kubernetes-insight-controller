# Releases

The project uses semantic versioning. Release tags must use the `vMAJOR.MINOR.PATCH`
format, for example `v0.1.1`.

## Version sources

- `VERSION` stores the next source-tree version without the leading `v`.
- Release tags are the source of truth for published artifacts.
- The manager binary embeds version metadata at build time and prints it with
  `--version`.

## Local checks

Run the same commands used by GitHub Actions:

```bash
make fmt-check
make vet
make test
make build
```

## Cutting a release

1. Update `VERSION`.
2. Update `CHANGELOG.md`.
3. Make sure `config/manager/deployment.yaml` points to the release image tag.
4. Run local checks.
5. Commit the changes.
6. Create and push a tag:

```bash
git tag v0.1.1
git push origin v0.1.1
```

The release workflow publishes:

- GitHub release notes with checksummed manager binaries.
- A multi-architecture container image at `ghcr.io/<owner>/<repo>`.
- A single install manifest named `k8s-insight-controller.yaml`.
