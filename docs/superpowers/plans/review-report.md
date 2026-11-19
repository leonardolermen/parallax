# Revisão independente — replay #25

Escopo: design e plano aprovados de 2026-11-19, pacote de revisão baseado em 3441e0a e fonte atual em `internal/repeticao` e `cmd/parallax/replay*.go`. Sem alterações de implementação ou git. Veredito final do produto, laudo, histórico, YAML e documentação de uso #28 ficam fora do escopo.

## Achados

### P2 — Não tentar descompactar respostas que não podem ter corpo

Local: `internal/repeticao/service_cliente.go:103–104`.

O cliente chama `gzip.NewReader` sempre que recebe `Content-Encoding: gzip`. Uma resposta HEAD pode anunciar o encoding da representação GET e não transportar corpo; 204 e 304 também não transportam corpo. Nesses casos válidos, `gzip.NewReader` devolve EOF e o código marca a resposta como `Incompleta` e `Erro: falha`. Até duas respostas iguais passam a produzir `diferenca`, e o par é excluído dos percentis.

Reprodução: servidor httptest retorna `Content-Encoding: gzip` e status 200 sem corpo para uma troca HEAD; executar contra o mesmo servidor nos dois lados. Esperado: troca idêntica, sem erro e elegível para latência. Atual: um achado `diferenca`, ambos os lados com `falha`/comparação incompleta. Repetir com GET e status 204 ou 304 exercita a mesma condição. HEAD confirmado também pelo coordenador; caminho causal verificado no código.

Correção: respeitar método/status que proíbem corpo antes de inicializar o leitor gzip. Cobrir HEAD, 204 e 304.

### P2 — Aplicar a origem preexistente quando a base já excede o teto

Local: `internal/repeticao/service_latencia.go:29–34`.

A regra relativa retorna `branch` antes de consultar o teto absoluto. Quando ambos os critérios disparam e a base já excedia o teto, a origem diverge do design aprovado: “Teto já excedido pela base tem origem pre_existente”. A decisão importa para consumidores que distinguem regressões da branch de problemas preexistentes.

Reprodução determinística: `c := ConfigPadrao(); c.LimiteAbsolutoMs = 50; Regressao(60, 100, c)`. Atual: `(true, branch)`; esperado pela especificação: `(true, pre_existente)`. O coordenador confirmou que não existe outra decisão aprovada dando precedência ao relativo.

Correção: resolver o caso de teto excedido nos dois lados antes do retorno relativo; testar os dois critérios simultâneos, além dos limites já cobertos.

### P2 — Rejeitar portas fora do intervalo antes de enviar tráfego

Local: `internal/repeticao/service_config.go:32–38`.

`url.Parse` e `http.NewRequest` aceitam uma porta numérica fora do intervalo TCP. Com uma base válida e candidata `http://127.0.0.1:99999`, a configuração passa, a base recebe tráfego e só depois o transporte rejeita a candidata. `Repetir` ainda devolve sucesso com achado `diferenca`, em vez de erro de configuração. Isso quebra a garantia de validar toda a configuração antes de qualquer tráfego.

Reprodução confirmada: teste temporário com contador atômico em httptest como base, candidata acima e captura de um GET `/`. `go test -timeout 120s ./...` mostrou `err=<nil>`, `calls=1` e candidata `Status:0`, `Erro:falha`. O teste esperava erro e zero chamadas; falhou exatamente nessa expectativa. Todos os demais testes passaram nessa execução. Caches ficaram em TEMP; o arquivo temporário `internal/repeticao/review_probe_test.go` foi removido após a reprodução.

Correção: validar explicitamente o intervalo da porta informada antes de carregar/executar; cobrir a candidata inválida com base local instrumentada e zero tráfego.

## Pareceres separados

- **Conformidade com a especificação: requer alterações.** Os três casos acima afetam comparação correta, origem da latência ou validação anterior ao tráfego.
- **Qualidade: requer alterações antes da aprovação.** A separação entre carregamento, cliente, pool, comparação e CLI é clara; não foram identificados outros bloqueadores concretos na revisão. Corrigir os três casos e adicionar regressões locais é suficiente para nova avaliação focada.

O ajuste concorrente que remove headers hop-by-hop antes e depois dos overrides foi conferido na fonte atual e não permanece como achado. Não foram reexecutados build/vet Windows/Linux já reportados pelo implementador.

## Re-review focada — onda única de correções

Escopo desta rodada: fonte congelada de `fix-review-package.txt` e registro RED/GREEN final de `core-report.md`. Inspeção somente leitura da implementação; suíte idêntica não reexecutada nesta revisão.

| Achado original | Parecer | Evidência |
| --- | --- | --- |
| P2 HEAD/204/304 gzip sem corpo | **ADDRESSED** | `enviar` agora exclui HEAD, 204 e 304 da inicialização gzip. A leitura vazia continua completa, sem erro. `TestGzipSemCorpoPermitido` cobre os três casos tanto no cliente quanto no replay, incluindo identidade e elegibilidade para percentis. |
| P2 origem do teto quando ambos os critérios disparam | **ADDRESSED** | `Regressao` resolve primeiro o teto absoluto e devolve `pre_existente` quando ambos o excedem. `TestRegressaoPrioridadeTeto` cobre o caso 60/100/teto50, critérios isolados, simultâneos, fronteiras exatas e teto desativado. |
| P2 porta inválida aceita com tráfego | **ADDRESSED** | `Config.Validar` valida a porta explícita com `strconv.Atoi` e intervalo 1–65535, além de rejeitar porta explícita vazia. `TestPortasInvalidasZeroTrafego` cobre ambos os destinos e contador zero; também preserva exemplos válidos HTTP/HTTPS, IPv6 e porta 65535. |

Nenhuma regressão concreta identificada nas alterações desta onda. O teste integrado adicional confirma agrupamento de rotas, ordem determinística, origem preexistente e percentis positivos com teto reduzido, sem sleeps. O registro final informa `go test -timeout 120s ./...` verde nos cinco pacotes após todas as alterações; esta revisão conferiu o conteúdo dos testes e não repetiu a execução.

- **Conformidade com a especificação: aprovada após as correções**, no escopo revisado; os três bloqueadores originais foram resolvidos.
- **Qualidade: aprovada**, no escopo desta revisão focada; nenhum achado pendente ou novo bloqueador identificado na onda.
