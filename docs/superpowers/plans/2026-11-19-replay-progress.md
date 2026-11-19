# Progresso — plano 2026-11-19-replay.md

- Design #24 aprovado, corpo excessivo decidido pelo usuário: comparação incompleta.
- Worktree limpo no início (3441e0a); reservas #25 sem conflito.
- Memória consultada: decisões replay #24 e relógio Windows conferidas no código.
- Teste inicial falha antes de compilar: acesso negado ao cache global Go.
- Adaptação: commits proibidos pelo ambiente, alterações ficam para Solaris.
- README e docs/captura.md pertencem à etapa documentador #28.
- Baseline passou com GOCACHE temporário fora do repositório: todos os 4 pacotes.

## Conferência de interfaces

| Tarefas | Interface / consistência | Resultado |
|---|---|---|
| 1 | Config e captura inteiramente validadas | consistente |
| 2 | cliente limitado/contexto/trace | consistente; max+1 detecta excesso |
| 3 | índices originais e pool | consistente |
| 4 | resposta do cliente e Achado existente | consistente |
| 5 | medidas platform e Config | consistente |
| 6 | carregar → executar → comparar | consistente |
| 7 | ConfigPadrao, Validar, Repetir | assinatura acordada antes da CLI |
| 8 | validação e revisão | documentação de uso fica com #28 |
| 1–6 | Config Base/Candidata string, limites explícitos | consistente |
| 2–4 | comparação de Location | cliente não seguirá redirects para comparar a resposta original |
| 4–6 | cap de 50 diffs | classificação de contrato considera também mudanças além do cap |

Tarefas 1–6 delegadas a core, sem subdelegação. Todos os Go usam cache TEMP.

- CLI: RED observado por repetir ausente; implementadas todas as flags, saída
  stdout/arquivo, cancelamento e proteção contra sobrescrita da captura.
- Primeiro teste integrado encontrou WSAECONNREFUSED localizado; corrigido
  reconhecimento pelo código WinSock 10061. Registrado em memória durável.
- Corrigida a montagem de URLs com caminho decodificado da captura e prefixo.
- Suíte integrada passou; build e vet Windows/Linux passaram com cache TEMP.
- Revisão independente em andamento (pacote contém código novo e diff tracked,
  pois o git é somente leitura). README/documentação de uso permanecem para #28.
- Revisão final encontrou 3 P2 confirmados: HEAD/204/304 com gzip sem corpo;
  teto absoluto deve preceder regra relativa para origem; porta 99999 deve ser
  rejeitada antes de tráfego. Uma única onda de correções delegada ao core,
  com testes de regressão primeiro e re-review focada posterior.
- Onda concluída: os três P2 resolvidos, RED/GREEN registrado no core-report.
- Re-review focada: todos ADDRESSED, conformidade e qualidade aprovadas, sem
  regressão concreta identificada na onda. Nenhuma pendência de implementação.
- Verificação final independente: go build ./..., go vet ./...,
  go test -timeout 120s ./... (5 pacotes), GOOS=linux go vet ./...: todos exit 0.
  gofmt -l sem saída, git diff --check sem erros. Caches somente TEMP.
- Branch solaris/task-24 mantida, sem commits/push/merge/operações git mutantes.
  Solaris fará o commit. Nenhum servidor persistente foi iniciado.

## Entrada para o testador #26

API pública em internal/repeticao, CLI direta repetir(...) testável sem processo.
Execute os quatro comandos de verificação acima com GOCACHE e GOMODCACHE em
TEMP (ver gotcha da memória); testes somente locais. A regressão Windows usa
WSAECONNREFUSED 10061. Casos essenciais estão nos testes de replay, motor,
revisão e CLI. Ver core-report e review-report para evidências e correções.

README e docs/captura.md ficam para o documentador #28. A saída é resultado
de replay, sem veredito geral, laudo, histórico, YAML ou campos ignorados.
