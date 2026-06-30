# Kubernetes Insight Controller

Kubernetes controller that periodically collects cluster state, workload health, warning events, and selected Prometheus metrics, then asks Azure OpenAI or Ollama for prioritized optimization recommendations.

![Test](https://github.com/NEODjazz/kubernetes-insight-controller/actions/workflows/test.yml/badge.svg)

Select the external LLM per `InsightReport` with `spec.llmProvider`. Supported values are `azureOpenAI` (the default for backward compatibility) and `ollama`.

## What it analyzes

- Nodes: readiness, kubelet version, allocatable CPU and memory.
- Workloads: Deployment and StatefulSet desired/available/unavailable replicas.
- Pods: phase, readiness, restart count, waiting reasons, node placement.
- Services: service type inventory.
- Events: optional Kubernetes warning events.
- Prometheus: CPU, memory, restarts, and unavailable deployment replica queries.

## Azure OpenAI configuration

The controller uses the Azure OpenAI Chat Completions REST API:

`POST {endpoint}/openai/deployments/{deployment}/chat/completions?api-version={apiVersion}`

The deployment name is configured in `spec.azureDeployment`. This should be the deployment name from Azure AI Foundry, not necessarily the raw model name. Store the API key in a Kubernetes Secret and inject it into the controller Pod as `AZURE_OPENAI_API_KEY`.

The system and user prompts are configured per report through `spec.systemPrompt` and `spec.userPrompt`. Use `{{snapshot}}` in the user prompt to control where the collected cluster JSON is inserted. If the placeholder is omitted, the controller appends the snapshot to the user prompt. Empty prompt fields use the built-in defaults.

```yaml
spec:
  systemPrompt: You are a senior Kubernetes SRE. Return recommendations in Russian.
  userPrompt: |-
    Analyze the following cluster snapshot:

    {{snapshot}}
```

```bash
kubectl create namespace k8s-insight-system
kubectl -n k8s-insight-system create secret generic azure-openai \
  --from-literal=api-key="$AZURE_OPENAI_API_KEY"
```

Microsoft recommends token based authentication for production Azure AI workloads. This starter keeps API key support because it is simple to run anywhere; a production variant should use workload identity and Entra ID tokens.

## Ollama configuration

The controller calls the Ollama chat API at `POST {ollamaEndpoint}/api/chat`. Configure an Ollama instance reachable from the controller Pod and make sure the selected model is already available there.

```yaml
spec:
  llmProvider: ollama
  ollamaEndpoint: http://ollama.ollama.svc.cluster.local:11434
  ollamaModel: qwen3:8b
```

Ollama normally requires no API key. For an external service protected by bearer authentication, create the optional `ollama` Secret used by the deployment:

```bash
kubectl -n k8s-insight-system create secret generic ollama \
  --from-literal=api-key="$OLLAMA_API_KEY"
```

See `config/samples/insightreport-ollama.yaml` for a complete report example.

The controller RBAC intentionally does not include `get`, `list`, or `watch` access to Kubernetes Secrets. Kubernetes Secret list responses include the `data` field, so even list-only access is not safe for an analyzer that sends cluster context to an external LLM. LLM credentials are injected into the controller Pod through environment variables.

## Local build

```bash
make test
make build
./bin/k8s-insight-controller --version
```

Without `make`, use the Go toolchain directly:

```bash
go test ./...
go build ./cmd/manager
```

## Deploy

The default manifest uses the published GitHub Container Registry image:
`ghcr.io/neodjazz/kubernetes-insight-controller:0.1.0`.

To move to another release, update the tag in `config/manager/deployment.yaml`,
then install the controller:

```bash
kubectl apply -k config
```

Then apply credentials and one report sample:

```bash
kubectl apply -f config/samples/azure-openai-secret.yaml
kubectl apply -f config/samples/insightreport.yaml
```

To use Ollama instead, apply `config/samples/insightreport-ollama.yaml`. The Azure API key Secret is optional when all reports use Ollama.

Read the result:

```bash
kubectl -n k8s-insight-system get insightreport cluster-health -o yaml
```

The generated recommendations are written to `.status.recommendations`.

Each successful analysis also creates an `InsightReportSnapshot` owned by the source `InsightReport`. The snapshot stores the analysis timestamp, duration, and recommendations. `spec.retentionDays` controls how many days snapshots are retained; the default is 30 days.

```bash
kubectl -n k8s-insight-system get insightreportsnapshots
```

## Web UI

The manager exposes a read-only web UI that lists all `InsightReportSnapshot` resources. Snapshots can be filtered by source `InsightReport`, snapshot name, and recommendation text. Recommendations returned by the model are rendered as sanitized Markdown.

When Traefik or another Kubernetes Ingress controller is available, the standard
install includes the Web UI Ingress. Open `http://k8s-insight.localhost`.

For direct local access without Ingress, use port-forward:

```bash
kubectl -n k8s-insight-system port-forward svc/k8s-insight-controller 8090:8090
```

Open `http://127.0.0.1:8090`.

## Architecture

The C4 architecture model, deployment view, and data-flow diagrams are defined in
Structurizr DSL at [`docs/architecture/workspace.dsl`](docs/architecture/workspace.dsl).
See [`docs/architecture/README.md`](docs/architecture/README.md) for rendering
instructions and a summary of the data flows.

## Releases

The project uses semantic versioning. Published releases are created from tags
like `v0.1.0`; GitHub Actions publishes checksummed binaries and a GHCR image.
See [`docs/releases/README.md`](docs/releases/README.md).

## License

Licensed under the Apache License, Version 2.0. See [`LICENSE`](LICENSE).
