workspace "Kubernetes Insight Controller" "Архитектура приложения и потоки данных" {

    model {
        sre = person "SRE / Platform Engineer" "Настраивает анализ кластера и изучает рекомендации."

        insightSystem = softwareSystem "Kubernetes Insight Controller" "Периодически собирает состояние Kubernetes-кластера и метрики, получает рекомендации от внешней LLM и хранит историю анализа." {
            manager = container "Manager Pod" "Запускает controller-runtime manager, reconciler, Web UI, health probes и endpoint метрик." "Go, controller-runtime" {
                reconciler = component "InsightReport Reconciler" "Следит за InsightReport, управляет периодическим запуском анализа, статусом и retention." "Go"
                collector = component "Cluster Snapshot Collector" "Собирает сведения о nodes, workloads, pods, services, warning events и метриках Prometheus." "Go"
                llmClient = component "LLM Client" "Формирует prompt со снимком кластера и вызывает Azure OpenAI или Ollama API." "Go, HTTP"
                webUI = component "Web UI / Read-only API" "Отдает HTML UI и REST endpoints для отчетов и истории рекомендаций." "Go, HTTP"
                managerRuntime = component "Controller Runtime" "Предоставляет Kubernetes client, watch/reconcile loop, leader election, health и metrics endpoints." "controller-runtime"

                reconciler -> collector "Запрашивает сбор снимка" "Внутренний вызов"
                reconciler -> llmClient "Передает JSON-снимок и prompts для анализа" "Внутренний вызов"
                reconciler -> managerRuntime "Читает InsightReport, обновляет status, создает и удаляет snapshots" "Kubernetes client"
                collector -> managerRuntime "Читает ресурсы кластера" "Kubernetes client"
                webUI -> managerRuntime "Читает InsightReport и InsightReportSnapshot" "Kubernetes client"
                webUI -> sre "Возвращает HTML/JSON с рекомендациями" "HTTP"
            }

            webService = container "Kubernetes Service" "Открывает Web UI и endpoint метрик внутри кластера." "Kubernetes Service"
            apiKeySecret = container "LLM API Key Secrets" "Хранят опциональные API keys, которые передаются Manager Pod через переменные окружения." "Kubernetes Secret" "Secret"

            webService -> manager "Маршрутизирует Web UI и metrics трафик" "HTTP :8090 / :8080"
            webService -> webUI "Передает запрос Web UI" "HTTP :8090"
            apiKeySecret -> manager "Инжектирует AZURE_OPENAI_API_KEY и OLLAMA_API_KEY" "Pod environment"
        }

        kubernetes = softwareSystem "Kubernetes Control Plane" "API server и etcd целевого Kubernetes-кластера." "Kubernetes" {
            clusterState = container "Cluster Resources" "Nodes, Deployments, StatefulSets, Pods, Services и warning Events." "Kubernetes API"
            insightCRDs = container "Insight CRDs" "InsightReport содержит конфигурацию и текущий статус; InsightReportSnapshot хранит историю рекомендаций." "Kubernetes API / etcd" "Database"
            leaderLease = container "Leader Election Lease" "Координирует единственный активный manager при включенном leader election." "Kubernetes API"
        }

        prometheus = softwareSystem "Prometheus" "Предоставляет текущие CPU, memory, restart и unavailable replica метрики." "Prometheus"
        azureOpenAI = softwareSystem "External LLM" "Azure OpenAI или Ollama анализирует JSON-снимок кластера и возвращает текстовые рекомендации." "Azure OpenAI / Ollama"
        operator = person "Kubernetes Operator / GitOps" "Создает и изменяет InsightReport и Secret, разворачивает приложение."

        operator -> insightCRDs "Создает и изменяет InsightReport" "kubectl / GitOps"
        operator -> apiKeySecret "Создает API key Secret" "kubectl / GitOps"
        sre -> webService "Просматривает историю рекомендаций" "HTTP :8090"
        sre -> insightCRDs "Читает текущий статус и рекомендации" "kubectl"

        managerRuntime -> clusterState "List: nodes, deployments, statefulsets, pods, services и warning events" "Kubernetes API"
        managerRuntime -> insightCRDs "Watch/List/Get InsightReport; Update status; Create/List/Delete InsightReportSnapshot" "Kubernetes API"
        managerRuntime -> leaderLease "Получает и обновляет lease" "Kubernetes API"
        collector -> prometheus "Выполняет PromQL instant queries" "HTTP GET /api/v1/query"
        llmClient -> azureOpenAI "Передает system prompt, user prompt и JSON-снимок кластера; получает рекомендации" "HTTP(S), Azure OpenAI / Ollama API"
        azureOpenAI -> llmClient "Возвращает текстовые рекомендации" "HTTPS response"

        deploymentEnvironment "Production" {
            deploymentNode "Kubernetes Cluster" "Целевой кластер" "Kubernetes" {
                deploymentNode "k8s-insight-system namespace" "Namespace приложения" "Kubernetes Namespace" {
                    deploymentNode "k8s-insight-controller Pod" "Один экземпляр manager" "Kubernetes Pod" {
                        containerInstance manager
                    }
                    containerInstance webService
                    containerInstance apiKeySecret
                }
                deploymentNode "Control Plane" "API server и etcd" "Kubernetes" {
                    containerInstance clusterState
                    containerInstance insightCRDs
                    containerInstance leaderLease
                }
                deploymentNode "monitoring namespace" "Опциональный Prometheus" "Kubernetes Namespace" {
                    softwareSystemInstance prometheus
                }
            }
            deploymentNode "Microsoft Azure" "Внешний Azure tenant/subscription" "Azure" {
                softwareSystemInstance azureOpenAI
            }
        }
    }

    views {
        systemContext insightSystem "SystemContext" "Системный контекст" {
            include *
            autoLayout lr
        }

        container insightSystem "Containers" "Контейнеры приложения и внешние потоки данных" {
            include *
            include kubernetes
            include prometheus
            include azureOpenAI
            include sre
            include operator
            autoLayout lr
        }

        component manager "ManagerComponents" "Компоненты Manager Pod" {
            include *
            include clusterState
            include insightCRDs
            include leaderLease
            include prometheus
            include azureOpenAI
            include webService
            include sre
            autoLayout lr
        }

        dynamic manager "AnalysisFlow" "Поток данных периодического анализа" {
            operator -> insightCRDs "1. Создает или изменяет InsightReport"
            managerRuntime -> insightCRDs "2. Получает событие InsightReport или запускается после requeue interval"
            reconciler -> collector "3. Запускает сбор состояния кластера"
            collector -> managerRuntime "4. Запрашивает Kubernetes-ресурсы"
            managerRuntime -> clusterState "5. Читает разрешенные RBAC ресурсы"
            collector -> prometheus "6. Выполняет настроенные PromQL-запросы"
            reconciler -> llmClient "7. Передает собранный JSON-снимок"
            llmClient -> azureOpenAI "8. Отправляет prompts и JSON-снимок"
            azureOpenAI -> llmClient "9. Возвращает рекомендации"
            reconciler -> managerRuntime "10. Передает результат для сохранения"
            managerRuntime -> insightCRDs "11. Создает InsightReportSnapshot и обновляет InsightReport.status"
            autoLayout lr
        }

        dynamic manager "HistoryFlow" "Поток просмотра истории рекомендаций" {
            sre -> webService "1. Открывает Web UI"
            webService -> webUI "2. Передает HTTP-запрос"
            webUI -> managerRuntime "3. Запрашивает отчеты и snapshots"
            managerRuntime -> insightCRDs "4. Читает InsightReport и InsightReportSnapshot"
            webUI -> sre "5. Возвращает HTML/JSON с рекомендациями"
            autoLayout lr
        }

        deployment insightSystem "Production" "KubernetesDeployment" {
            include *
            autoLayout lr
        }

        styles {
            element "Person" {
                shape Person
                background #08427b
                color #ffffff
            }
            element "Software System" {
                background #1168bd
                color #ffffff
            }
            element "Container" {
                background #438dd5
                color #ffffff
            }
            element "Component" {
                background #85bbf0
                color #111111
            }
            element "Database" {
                shape Cylinder
                background #2e7d32
                color #ffffff
            }
            element "Secret" {
                shape Hexagon
                background #7b1fa2
                color #ffffff
            }
            relationship "Relationship" {
                color #707070
                routing Orthogonal
            }
        }
    }
}
