# Архитектурная схема

Файл [`workspace.dsl`](workspace.dsl) содержит C4-модель приложения в формате
Structurizr DSL:

- System Context — пользователи, приложение, Kubernetes, Prometheus и внешняя LLM (Azure OpenAI или Ollama).
- Containers — Manager Pod, Kubernetes Service и Secret.
- Components — reconciler, collector, LLM client, Web UI и controller-runtime.
- Dynamic views — поток периодического анализа и поток просмотра истории.
- Deployment — размещение компонентов в Kubernetes и Azure.

## Запуск Structurizr Local

Из корня репозитория:

```bash
docker run --rm -it \
  -p 8080:8080 \
  -v "$PWD/docs/architecture:/usr/local/structurizr" \
  structurizr/structurizr local
```

После запуска открыть `http://localhost:8080`.

## Основной поток данных

1. Operator или GitOps создает `InsightReport`.
2. Reconciler собирает разрешенные RBAC данные Kubernetes и опциональные метрики Prometheus.
3. Collector формирует JSON-снимок кластера.
4. LLM Client добавляет снимок в prompt и передает его во внешний Azure OpenAI или Ollama API.
5. Полученные рекомендации сохраняются в `InsightReport.status` и новый `InsightReportSnapshot`.
6. SRE читает текущий результат через `kubectl` или историю через Web UI.

Kubernetes Secrets не входят в собираемый снимок. Единственный Secret приложения
содержит опциональные Azure OpenAI или Ollama API keys и инжектируется в Manager Pod через переменные окружения.
