# Pulse — Plano Mestre de Execução Autônoma

> Missão do agente: implementar, executar, testar, medir, corrigir e auto-validar o projeto de ponta a ponta. Não encerrar porque apenas compila ou sobe. Só declarar concluído quando os critérios de aceite forem atendidos ou uma limitação física de infraestrutura estiver comprovada por métricas.

## 1. Objetivo

Construir o **Pulse**, plataforma distribuída e multi-tenant para ingestão, processamento, persistência, analytics e distribuição de eventos em alta volumetria.

Demonstrar: arquitetura de software; sistemas distribuídos; Go e concorrência; Kafka; particionamento; consumer groups; idempotência; consistência eventual; backpressure; retry/DLQ; circuit breaker; bulkhead; rate limiting distribuído; PostgreSQL; Redis; ClickHouse; Kubernetes; observabilidade; performance engineering; chaos engineering; CI/CD; capacity planning; ADRs.

### Metas

- Benchmark-alvo: **100.000 eventos/s**.
- p95 de ingestão: **<100 ms**.
- p99: **<250 ms**.
- Perda silenciosa de evento confirmado: **0**.
- Efeito duplicado do mesmo evento: **0**.
- SLO de disponibilidade: **99,95%**.

100k/s é meta real, não número a falsificar. Se o hardware não suportar, encontrar o throughput máximo sustentável, gargalos e capacity planning para 100k/s.

## 2. Regras do agente

1. Trabalhar autonomamente e não pedir confirmação para decisões reversíveis.
2. Criar ADR para decisões arquiteturais importantes.
3. Nunca esconder testes falhando ou fabricar benchmarks.
4. Medir antes de otimizar; registrar before/after.
5. Não adicionar tecnologia sem necessidade.
6. Corrigir automaticamente build, lint, testes, integração, containers e manifests.
7. Reexecutar testes afetados após correções.
8. Fazer validação limpa final.
9. Não deixar TODO/FIXME crítico.
10. Nunca commitar secrets.
11. README deve permitir reprodução completa.
12. Não reduzir critérios de aceite para “passar”.

## 3. Arquitetura

```text
Clients
  |
Gateway / LB (Auth + Rate Limit)
  |
Event Ingestion (Go)
  |
Kafka
  +------------+-------------+
  |            |             |
Persistence  Analytics     Webhook
Worker       Processor     Dispatcher
  |            |             |
PostgreSQL  ClickHouse    External APIs
  |
Redis

Query / Analytics API
```

Stack: Go, Kafka, PostgreSQL, Redis, ClickHouse, Docker Compose, Kubernetes, Helm, Terraform quando aplicável, OpenTelemetry, Prometheus, Grafana, Loki, Tempo/Jaeger, k6 e GitHub Actions. Trocas centrais exigem ADR.

## 4. Estrutura sugerida

```text
pulse/
├── cmd/{ingestion,persistence-worker,analytics-worker,webhook-worker,query-api}
├── internal/{auth,events,kafka,idempotency,ratelimit,webhook,telemetry,platform}
├── migrations/
├── deployments/{docker,helm,terraform}
├── observability/{prometheus,grafana,otel}
├── tests/{integration,e2e,load,chaos}
├── docs/{architecture,benchmarks,capacity-planning,runbooks,adr}
├── scripts/
├── .github/workflows/
├── docker-compose.yml
├── Makefile
├── .env.example
└── README.md
```

## 5. Domínio e API

Tenant: `id, name, api_key_hash, status, created_at, updated_at`. Nunca armazenar API key em texto puro.

Evento:

```json
{"eventId":"evt_8fa91","type":"payment.completed","timestamp":"2026-09-09T22:30:00Z","payload":{"paymentId":"123","amount":499.90}}
```

Derivar `tenantId` da credencial autenticada.

Webhook Subscription: `id, tenant_id, event_type, endpoint, secret, enabled, created_at, updated_at`.

Ingestão:

```http
POST /v1/events
Authorization: Bearer <api-key>
Content-Type: application/json
```

Resposta somente após ponto de durabilidade documentado:

```http
202 Accepted
```

```json
{"eventId":"evt_8fa91","status":"accepted"}
```

Criar API/CLI para tenants, rotação de API key, webhooks, DLQ e replay seguro. Criar analytics por período, tipo e tenant.

## 6. Pipeline de ingestão

```text
HTTP -> Authentication -> Rate Limit -> Validation
     -> Duplicate Handling -> Serialization -> Kafka -> ACK
```

Implementar payload limit, timeouts, pooling, graceful shutdown, batching/compressão quando benchmark justificar.

## 7. Kafka

Começar simples: `events.raw` e `events.dlq`, com consumer groups independentes `persistence-workers`, `analytics-workers`, `webhook-workers`.

V1: `partition_key = tenantId`, preservando ordering por tenant.

Criar benchmark de hot partition com tenants pequenos e um tenant dominante. Medir distribuição e lag. Se necessário evoluir para `tenantId + bucket`, criando ADR e documentando impacto em ordering.

Implementar offset management correto, rebalance, retry seguro, poison messages, graceful shutdown, tracing via headers, retenção e replay.

## 8. Semântica e idempotência

Adotar **at-least-once + idempotent consumers**. Chave lógica: `tenantId + eventId`.

Testar evento normal, duplicata, consumer morto durante processamento, redelivery, duplicatas simultâneas, múltiplos consumers e race conditions. Documentar por que não se promete exactly-once end-to-end.

## 9. Workers

Persistence Worker: batching, transações, idempotência, retry, poison-message handling, métricas, tracing e shutdown. Benchmark batch sizes `1,10,50,100,500,1000`.

Analytics: ClickHouse para OLAP se validado. Consultas: eventos/minuto, tenant, tipo, volume temporal, top tenants e falhas.

Webhook Dispatcher: timeout, concorrência limitada, exponential backoff + jitter, circuit breaker, bulkhead, DLQ, HMAC, idempotência, métricas e SSRF protection.

Headers: `X-Pulse-Signature`, `X-Pulse-Timestamp`, `X-Pulse-Event-Id`.

Retry inicial configurável: `1s,5s,30s,5m,30m`.

Teste bulkhead: A=20ms, B=100ms, C=timeout, D=500; C/D não podem bloquear A/B.

Circuit breaker: `CLOSED -> OPEN -> HALF_OPEN -> CLOSED`, com métricas.

## 10. Rate limiting e backpressure

Token Bucket por tenant, distribuído entre réplicas; avaliar Redis. Testar que múltiplos pods não multiplicam o limite.

Nenhuma fila em memória pode crescer ilimitadamente. Monitorar lag, queue depth, active workers, CPU, RAM, processamento/s e erros/s. Documentar pause/resume, shedding quando aplicável e recuperação de backlog.

## 11. Go e dados

Usar conscientemente goroutines, channels, worker pools, context/cancellation/timeouts, errgroup e sync primitives apenas quando necessárias.

Obrigatório:

```bash
go test -race ./...
go vet ./...
```

PostgreSQL: migrations, índices, constraints, unique idempotency, pool, timeouts, métricas e teste de saturação.

Redis: rate limit/dedup/cache/coordenação conforme ADR, TTL e comportamento fail-open/fail-closed documentado.

## 12. Lifecycle e observabilidade

Endpoints `/health/live` e `/health/ready`. Liveness não deve reiniciar pods só por dependência externa temporária.

SIGTERM: ficar não-ready, parar novo trabalho, drenar, parar consumers, commitar offsets apenas quando seguro, fechar conexões, flush telemetry e encerrar no grace period. Testar shutdown durante processamento.

Logs JSON: timestamp, level, service, tenant_id, event_id, trace_id, span_id, duration_ms, message, error.

Métricas mínimas: requests, latência, eventos recebidos/processados/falhos/duplicados, Kafka publish latency, processing latency, consumer lag, webhook delivery/retry/DLQ, rate-limit rejection, workers e queue depth.

Tracing: `HTTP -> Kafka Produce -> Kafka Consume -> DB/ClickHouse/Webhook`.

Provisionar dashboards Grafana: Overview, Kafka, Webhooks e Tenants.

## 13. Ambientes

Docker Compose deve subir Kafka, PostgreSQL, Redis, ClickHouse, Prometheus, Grafana, Loki, Tempo/Jaeger e Pulse.

Criar:

```text
make up
make down
make test
make integration
make e2e
make load-smoke
make validate
```

Helm: Deployment, Service, ConfigMap, Secrets templates, probes, requests/limits, HPA e PDB quando adequado. Requests/limits baseados em medições.

Testar HPA. Documentar que consumers Kafka não escalam utilmente além do número de partitions do consumer group.

## 14. Testes

Unitários: validation, auth, partition strategy, idempotency, rate limiting, retry/backoff/jitter, circuit breaker, HMAC, worker pools.

Integração:

```text
API -> Kafka
Kafka -> PostgreSQL
Kafka -> ClickHouse
Kafka -> Webhook mock
Redis -> Rate Limit
Redis/DB -> Idempotency
```

E2E:

```text
Create Tenant -> Create Webhook -> Send Event -> Kafka
-> Persistence + Analytics + Webhook -> Validate all effects
```

Concorrência: múltiplos producers/consumers, duplicatas, rebalance, shutdown e race detector.

## 15. Performance e capacity planning

Criar k6:

- Smoke: 10 req/s por 1 min.
- Baseline: 1k/s por 5 min.
- Progressão: 1k, 5k, 10k, 25k, 50k, 75k, 100k/s quando possível.
- Spike.
- Stress até saturação.
- Soak prolongado.

Registrar throughput real, p50/p95/p99, errors, CPU, RAM, GC, network, Kafka lag, DB pool e latências de Redis/ClickHouse.

Definir **maximum sustainable throughput** como carga sem crescimento contínuo do backlog e dentro dos limites de erro/latência.

Capacity planning deve incluir events/s por pod, CPU/evento, memória, bandwidth, tamanho médio de evento, partitions, consumers úteis, crescimento diário, retenção Kafka, storage e estimativa para 100k/s.

## 16. Chaos engineering

Automatizar cenários: matar ingestion pod, consumer, reiniciar Kafka, PostgreSQL lento/indisponível, Redis lento/indisponível, webhook timeout/500 e problemas de rede quando viável.

Para cada cenário: hipótese, esperado, observado, métricas, recovery time, perda/duplicação e correções.

Evento confirmado jamais pode desaparecer silenciosamente.

## 17. Segurança

API keys por hash, rotação, TLS em produção, HMAC, replay protection, input validation, payload limit, SSRF protection, queries parametrizadas, external secrets, least privilege, dependency scanning, containers non-root quando possível, imagens mínimas e nenhuma credencial em logs.

## 18. CI/CD

GitHub Actions: format check, lint, unit, race detector, integration, build, container build e security/dependency checks. Load/chaos completo pode ser manual. Não mascarar validações obrigatórias com `|| true`.

## 19. ADRs obrigatórios

Criar pelo menos:

```text
ADR-001-messaging-platform.md
ADR-002-delivery-semantics.md
ADR-003-partition-strategy.md
ADR-004-idempotency.md
ADR-005-operational-vs-analytical-storage.md
ADR-006-rate-limiting.md
ADR-007-webhook-resilience.md
ADR-008-observability.md
ADR-009-autoscaling.md
```

Formato:

```markdown
# ADR-NNN: Título
## Status
## Contexto
## Drivers
## Alternativas consideradas
## Decisão
## Consequências positivas
## Consequências negativas
## Evidências / benchmarks
```

## 20. Documentação obrigatória

Produzir README, architecture overview, diagramas, ADRs, API docs/OpenAPI, runbook, troubleshooting, SLI/SLO, benchmarks, capacity planning, chaos report e security considerations.

README final deve mostrar resultados reais, não metas, por exemplo: throughput máximo medido, p95/p99, cenário/hardware, delivery semantics, idempotency, horizontal scaling e chaos tests.

## 21. Fases de execução

### Fase 0 — Baseline
Inicializar repo, tooling, Makefile, Compose, CI básico e documentação inicial.

### Fase 1 — Ingestão mínima
Tenant/auth, API, Kafka producer, validação e testes.

### Fase 2 — Persistência
Consumer, PostgreSQL, migrations, idempotência e integração.

### Fase 3 — Observabilidade
OTel, Prometheus, logs, tracing e Grafana.

### Fase 4 — Analytics
ClickHouse, processor, query API e benchmarks.

### Fase 5 — Webhooks
Subscriptions, dispatcher, HMAC, retry, circuit breaker, bulkhead, DLQ e SSRF.

### Fase 6 — Performance
k6, profiling, batching, tuning e relatório before/after.

### Fase 7 — Kubernetes
Helm, probes, resources, HPA e scaling tests.

### Fase 8 — Chaos
Falhas controladas, recuperação, runbooks e correções.

### Fase 9 — Finalização
Security scan, testes limpos, documentação, capacity planning e relatório final.

Não pular diretamente para uma arquitetura complexa se a evolução puder ser demonstrada.

## 22. Critério global de conclusão

Só marcar **DONE** quando:

- build reproduzível passa;
- lint/vet passam;
- unit/integration/E2E passam;
- race detector passa;
- Compose sobe do zero;
- Kafka, PostgreSQL, Redis e ClickHouse estão integrados;
- idempotência comprovada;
- retry/DLQ comprovados;
- circuit breaker/bulkhead comprovados;
- rate limiting distribuído comprovado;
- observabilidade e dashboards funcionam;
- tracing atravessa componentes;
- graceful shutdown validado;
- load/stress/spike executados;
- throughput máximo sustentável medido;
- chaos tests executados;
- Kubernetes/Helm validado;
- HPA testado quando o ambiente permitir;
- ADRs/documentação concluídos;
- security checks relevantes tratados;
- nenhum TODO crítico permanece.

## 23. Auto-validação final obrigatória

Executar o equivalente a:

```bash
make clean
make up
make lint
make test
make race
make integration
make e2e
make load-smoke
make validate
```

Depois executar benchmark principal e chaos suite apropriados.

Gerar `docs/FINAL_VALIDATION_REPORT.md` contendo:

```text
Commit testado
Data
Ambiente/hardware
Versões
Build
Lint
Unit
Race
Integration
E2E
Security
Load
Stress
Spike
Soak
Chaos
Throughput máximo sustentável
p50/p95/p99
Error rate
Consumer lag
Problemas encontrados
Correções realizadas
Limitações
Critérios não atendidos
Conclusão
```

Se qualquer critério obrigatório não for atendido, **não declarar o projeto concluído**. Corrigir e repetir. Se a causa for limitação externa/hardware, fornecer evidência e capacity planning.

## 24. Definition of Done do agente

A resposta final do agente deve ser curta e factual. Informar:

1. que o projeto foi implementado;
2. quais validações passaram;
3. throughput/latências realmente medidos;
4. onde está `FINAL_VALIDATION_REPORT.md`;
5. limitações remanescentes, se existirem.

Não responder “pronto” antes de executar a validação final.

---

# Instrução final ao agente

Você recebeu autorização para executar este plano de forma autônoma. Analise o repositório e o ambiente disponíveis, crie um plano interno de trabalho e avance fase por fase. Tome decisões técnicas reversíveis sem solicitar aprovação. Para decisões arquiteturais relevantes, registre ADRs.

Implemente código real, infraestrutura real, testes reais e benchmarks reais. Execute tudo que puder localmente. Investigue falhas, corrija-as e repita os testes.

Seu objetivo não é produzir apenas código: é entregar uma **demonstração verificável de arquitetura para sistemas de alta escala**, com evidências quantitativas, resiliência, observabilidade, documentação e decisões arquiteturais justificadas.

**Não encerre a tarefa até atingir o Definition of Done ou documentar objetivamente uma limitação externa incontornável.**
