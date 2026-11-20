# Plano: testes de veredito, laudo e histórico (#33)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** demonstrar os caminhos de erro e a preservação dos resultados entre check, arquivos e history.

**Architecture:** testes novos junto à CLI e ao pacote julgamento, reutilizando os helpers existentes. Produção só muda quando um teste demonstra defeito no contrato aprovado. Execução por subagentes conforme escolha técnica já determinada pelo usuário; revisão independente ao terminar.

**Tech Stack:** Go 1.22+, biblioteca padrão, testing e httptest local.

**Spec:** `docs/superpowers/specs/2026-11-19-testes-veredito-laudo-historico-design.md` — desenho aprovado pelo usuário na retomada desta execução. Plano aprovado pelo usuário: “Aprovo o plano; execute”.

## Restrições globais

- Preservar pkg/veredito, códigos 0/1/2/4 de check e 0/4 de replay, trabalhadores padrão 4 e regra de latência (>50% e >=20 ms, relativa antes do teto).
- Todo teste roda com `go test -timeout 120s ./...`; filtros e repetição são acrescentados a esse comando.
- GOCACHE e GOMODCACHE em TEMP, nunca no repositório; somente servidores httptest locais e clientes HTTP com timeout.
- Reservar os arquivos no Solaris antes de editar; preferir arquivos novos, pois #32 reservou cmd/parallax/ e internal/julgamento/.
- Não alterar testes existentes para ocultar falhas; sem dependências, CI, push, merge ou Git de escrita. O Solaris fará o commit.

## Foco da revisão

1. Falha de stdout ou report deve retornar 4 sem perder a execução já persistida (tarefa 1).
2. Captura inválida após linha válida deve abortar antes de qualquer tráfego (tarefa 1).
3. Cancelamento durante requisição deve terminar com erro, sem histórico ou laudo de sucesso (tarefa 1).
4. Arquivo ausente ou JSON inválido deve produzir erro de leitura; listagem segue a política de execucao.json como marcador (tarefa 2).
5. JSON, laudo e metadados devem preservar a mesma execução entre check e history, inclusive revisar por corpo incompleto (tarefa 3).

## Tarefa 1: erros e cancelamento da CLI

**Files:** criar `cmd/parallax/check_erros_test.go` e `cmd/parallax/history_erros_test.go`.

**Interfaces:** consumir `checar(context.Context, []string, io.Writer, io.Writer) int`, `historico([]string, io.Writer, io.Writer) int`, `capturaCheck`, `servidorCheck` e `execucoesGravadas`. Produzir testes sem alterar essas assinaturas.

- [x] Criar escritor que retorna erro sentinela; `TestCheckErroSaida` cobre Markdown/JSON, código 4, diagnóstico e histórico legível mesmo após falha; com report válido, conferir que ele foi gravado.
- [x] `TestCheckErroReport` usa diretório no lugar do arquivo, evitando depender de permissões Unix; conferir 4, stdout válido e histórico preservado.
- [x] `TestHistoryErroSaida` cobre listagem, show Markdown e show JSON com escritor falho e diagnóstico não vazio.
- [x] `TestCheckEntradaInvalidaSemTrafego` cobre arquivo vazio e linha inválida após linha válida; contador atômico no handler comprova zero envios, stdout vazio, código 4 e zero execuções.
- [x] `TestCheckCancelamento` cobre contexto previamente cancelado e cancelamento após handler sinalizar recebimento. Usar canais e prazo explícito (até 5s), liberar handlers no cleanup, conferir 4 e ausência de histórico/laudo. Configurar --timeout explicitamente.
- [x] Rodar `go test -timeout 120s ./... -run 'Test(CheckErro|HistoryErro|CheckEntrada|CheckCancelamento)'`; testes de cobertura podem passar imediatamente. Se falharem, aplicar systematic-debugging e corrigir somente o defeito comprovado, após nova reserva dos arquivos de produção.

## Tarefa 2: leitura degradada e erro no gerador do laudo

**Files:** criar `internal/julgamento/repository_historico_erros_test.go` e `internal/julgamento/service_laudo_erros_test.go`.

**Interfaces:** consumir `Historico.Salvar`, `Historico.Ler`, `Historico.Listar`, `EscreverLaudo(io.Writer, Execucao, veredito.Veredito) error` e helpers `salvarTeste`, `resultadoTeste`, `execucaoTeste`.

- [x] `TestHistoricoArquivosAusentes` remove cada um dos três arquivos de uma execução válida e exige errors.Is(err, ErrNaoEncontrada) de Ler.
- [x] `TestHistoricoJSONCorrompido` troca execucao.json e veredito.json, separadamente, por JSON inválido; exigir erro de leitura identificando o id, sem classificá-lo como inexistente.
- [x] Em cada cenário, manter segunda execução íntegra. Conferir Listar: metadados ausentes ignorados, metadados corrompidos geram aviso, veredito/laudo ausentes não retiram entrada cujos metadados continuam legíveis. Não ampliar validação fora do escopo.
- [x] `TestLaudoErroEscrita` injeta erro no primeiro Write e após um prefixo aceito; errors.Is deve recuperar o erro original e o gerador deve terminar.
- [x] Rodar `go test -timeout 120s ./... -run 'Test(HistoricoArquivos|HistoricoJSON|LaudoErro)'`; diagnosticar falhas antes de qualquer correção.

## Tarefa 3: integração e verificação final

**Files:** criar `cmd/parallax/check_history_integracao_test.go`; atualizar estado desta spec/plano e registrar evidências para o revisor no plano.

**Interfaces:** consumir `rodar(args ...string) (int, string, string)`, `julgamento.Execucao` e `veredito.Veredito.Validar`.

- [x] `TestCheckHistoryRoundTrip` executa check --json --report em tabela: aprovado (0), candidata 500 (1), resposta truncada via --max-body (2). Evitar depender de latência real para definir o resultado, configurando limites altos neste teste.
- [x] Para cada execução, validar contrato e resultado esperado; comparar integralmente stdout JSON com veredito.json e history show --json. Comparar bytes de report com laudo.md e history show.
- [x] Validar id, data UTC não vazia dentro do intervalo da chamada, caminho absoluto da captura, URLs, resumo, total e incompletas no execucao.json. Conferir que a listagem contém o id e o resultado. Comparar estatísticas comuns entre metadados e veredito; incompletas só existe nos metadados do replay.
- [x] Rodar `go test -timeout 120s ./...` e depois `go test -timeout 120s ./... -run 'Test(CheckErro|HistoryErro|CheckEntrada|CheckCancelamento|HistoricoArquivos|HistoricoJSON|LaudoErro|CheckHistoryRoundTrip)' -count=10`, sempre com caches em TEMP.
- [x] Rodar `go build ./...` e `go vet ./...` com os mesmos caches. Conferir git diff --check e git status; não produzir binário no repositório.
- [x] Usar requesting-code-review e verification-before-completion; corrigir achados demonstrados e repetir apenas verificações afetadas. Registrar evidências, lacunas e reservas sobrepostas para a etapa #34.

## Limitações conhecidas e entrega

- Memória consultada: decisions/parallax-replay-demanda-24.md, gotchas/go-cache-global-acesso-negado.md e gotchas/go-race-sem-cgo-windows.md. O check placeholder mencionado na decisão antiga já foi substituído pela parte 3 no código atual.
- Race detector não disponível nesta máquina Windows sem cgo; permissões 0700/0600 não verificáveis aqui. Manter os testes existentes sem remover ou pular novos testes para esconder falhas.
- Sem execução do binário/curl bloqueados neste ambiente; a integração usa as funções reais da CLI. Disco cheio não será induzido; falhas de escrita usam escritor com erro e caminho que é diretório.
- Registrar em ai-memory apenas novas descobertas duráveis; o Solaris captura automaticamente o relato da demanda.
- Estado da execução anterior: árvore limpa, último commit a27656d, sem alteração de código/testes. Esta retomada iniciou em ebefa89 e executou o plano aprovado.

## Execução da etapa de testes

Base desta retomada: ebefa89; árvore limpa. Suíte de base: seis pacotes passaram com caches em TEMP.

| Verificação prévia | Resultado |
|---|---|
| Tarefa 1, consistência interna | Arquivos novos; helpers existentes; erros observáveis e cancelamento sincronizado. |
| Tarefa 2, consistência interna | Arquivos novos; leitura degradada sem ampliar validação do formato. |
| Tarefa 3, consistência interna | Integração real da CLI; comparação dentro da mesma execução evita variação de id/latência. |
| Tarefas 1 e 2 | Sem arquivos compartilhados; ambas preservam a política de persistência. |
| Tarefas 1 e 3 | Helpers da CLI compartilhados apenas para leitura; arquivos novos separados. |
| Tarefas 2 e 3 | Interfaces Salvar/Ler e formato persistido compatíveis; nenhuma alteração de produção prevista. |

Decisão de execução: subagentes com contexto restrito e revisão independente. Cobertura de comportamento existente pode passar de imediato, conforme plano aprovado; qualquer correção de produção exige teste que reproduza o defeito antes. Não reabrir brainstorming/design/plano já aprovados.

Reservas #33 efetuadas. #32 ainda reserva cmd/parallax/ e internal/julgamento/; serão criados testes novos, sem reorganizar arquivos existentes. Git de escrita reservado ao Solaris.
Tarefa 1 implementada em dois arquivos novos; filtro de erros/cancelamento passou. Revisão independente aprovou conformidade e qualidade, sugerindo aguardar a goroutine no cleanup do teste de cancelamento. Ajuste aplicado com canal de término e prazo independente de 5s; filtro TestCheckCancelamento passou. Build e vet passaram com caches em TEMP.

Tarefa 2 implementada em dois arquivos novos; filtro de histórico/laudo passou. Revisão independente aprovou conformidade e qualidade sem achados. Revisão do ajuste de cleanup da tarefa 1 também aprovada.

## Entrega para a revisão #34

Concluído: nove funções de teste em cinco arquivos novos, cobrindo 20 cenários. Nenhum arquivo de produção nem teste existente foi alterado. Contrato, códigos de saída, limite de trabalhadores e regra de latência foram preservados. Não foram encontrados defeitos de produção nos cenários exercitados.

- `cmd/parallax/check_erros_test.go`: erros de stdout Markdown/JSON, falha de report, captura vazia/inválida sem tráfego e cancelamento prévio/durante requisição.
- `cmd/parallax/history_erros_test.go`: erros de saída na listagem e no show Markdown/JSON.
- `internal/julgamento/repository_historico_erros_test.go`: arquivos ausentes/corrompidos, classificação dos erros, política de listagem e preservação de outra execução íntegra.
- `internal/julgamento/service_laudo_erros_test.go`: propagação da causa na primeira escrita e após prefixo aceito.
- `cmd/parallax/check_history_integracao_test.go`: aprovado/regressao/revisar; igualdade integral de JSON e Markdown entre CLI, arquivos e history; metadados e contagens conferidos.

Verificação com GOCACHE=`%TEMP%/parallax-task33-go-build` e GOMODCACHE=`%TEMP%/parallax-task33-go-mod`:

| Comando | Resultado |
|---|---|
| `go test -timeout 120s ./... -count=1` | Passou nos seis pacotes, sem cache de resultados. |
| `go test -timeout 120s ./... -run <filtro da tarefa 3> -count=10` | Passou; CLI 5.055s, julgamento 2.028s. |
| `go test -timeout 120s ./... -run TestCheckHistoryRoundTrip -count=10` | Passou após remover a dependência do espaçamento visual na listagem. |
| `go test -timeout 120s ./...` | Verificação final passou nos seis pacotes após os ajustes da revisão. |
| `go build ./...` e `go vet ./...` | Verificação final passou, sem saída de erro e sem binário no repositório. |
| `gofmt -l` nos cinco arquivos novos | Sem saída. |
| `git diff --check` | Sem erro de whitespace. |

Revisões independentes aprovaram conformidade e qualidade das tarefas 1 e 2, da tarefa 3 e da entrega final. Dois achados baixos resolvidos: cleanup do cancelamento aguarda a goroutine com prazo independente antes de fechar o servidor/remover temporários; a listagem não exige dois espaços literais antes do resumo. Ambos tiveram re-revisão aprovada. Não há achados pendentes.

Lacunas: detector de corrida indisponível no Windows sem cgo (limitação previamente registrada); permissões Unix 0700/0600 não verificáveis nesta máquina; binário/curl não executados, conforme bloqueios conhecidos, com integração pelas funções reais da CLI. Não foi induzido disco cheio: erros de escrita são testados com writer falho e diretório no destino. Testes existentes foram mantidos, sem novos skips. CI e ajuste com tráfego real continuam fora do escopo.

ai-memory respondeu normalmente; nenhuma nova armadilha de ambiente ou decisão durável foi descoberta, portanto não foi criada nota transitória na wiki. O plano guarda evidências para o revisor e o Solaris captura as notas da demanda.

Sobreposição de reservas com #32: apenas arquivos novos dentro de cmd/parallax/ e internal/julgamento/, reduzindo o risco de conflito. Documentos desta etapa atualizados com aprovações e evidências. Branch solaris/task-31 preservada; nenhum push, merge, troca de branch ou comando Git de escrita. Alterações ficam para o commit automático do Solaris, conforme a instrução específica deste ambiente.