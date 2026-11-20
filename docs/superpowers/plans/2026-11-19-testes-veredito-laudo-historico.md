# Plano: testes de veredito, laudo e histórico (#33)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** demonstrar os caminhos de erro e a preservação dos resultados entre check, arquivos e history.

**Architecture:** testes novos junto à CLI e ao pacote julgamento, reutilizando os helpers existentes. Produção só muda quando um teste demonstra defeito no contrato aprovado. Execução por subagentes conforme escolha técnica já determinada pelo usuário; revisão independente ao terminar.

**Tech Stack:** Go 1.22+, biblioteca padrão, testing e httptest local.

**Spec:** `docs/superpowers/specs/2026-11-19-testes-veredito-laudo-historico-design.md` — desenho aprovado pelo usuário na retomada desta execução. Plano aguardando revisão pelo Solaris.

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

- [ ] Criar escritor que retorna erro sentinela; `TestCheckErroSaida` cobre Markdown/JSON, código 4, diagnóstico e histórico legível mesmo após falha; com report válido, conferir que ele foi gravado.
- [ ] `TestCheckErroReport` usa diretório no lugar do arquivo, evitando depender de permissões Unix; conferir 4, stdout válido e histórico preservado.
- [ ] `TestHistoryErroSaida` cobre listagem, show Markdown e show JSON com escritor falho e diagnóstico não vazio.
- [ ] `TestCheckEntradaInvalidaSemTrafego` cobre arquivo vazio e linha inválida após linha válida; contador atômico no handler comprova zero envios, stdout vazio, código 4 e zero execuções.
- [ ] `TestCheckCancelamento` cobre contexto previamente cancelado e cancelamento após handler sinalizar recebimento. Usar canais e prazo explícito (até 5s), liberar handlers no cleanup, conferir 4 e ausência de histórico/laudo. Configurar --timeout explicitamente.
- [ ] Rodar `go test -timeout 120s ./... -run 'Test(CheckErro|HistoryErro|CheckEntrada|CheckCancelamento)'`; testes de cobertura podem passar imediatamente. Se falharem, aplicar systematic-debugging e corrigir somente o defeito comprovado, após nova reserva dos arquivos de produção.

## Tarefa 2: leitura degradada e erro no gerador do laudo

**Files:** criar `internal/julgamento/repository_historico_erros_test.go` e `internal/julgamento/service_laudo_erros_test.go`.

**Interfaces:** consumir `Historico.Salvar`, `Historico.Ler`, `Historico.Listar`, `EscreverLaudo(io.Writer, Execucao, veredito.Veredito) error` e helpers `salvarTeste`, `resultadoTeste`, `execucaoTeste`.

- [ ] `TestHistoricoArquivosAusentes` remove cada um dos três arquivos de uma execução válida e exige errors.Is(err, ErrNaoEncontrada) de Ler.
- [ ] `TestHistoricoJSONCorrompido` troca execucao.json e veredito.json, separadamente, por JSON inválido; exigir erro de leitura identificando o id, sem classificá-lo como inexistente.
- [ ] Em cada cenário, manter segunda execução íntegra. Conferir Listar: metadados ausentes ignorados, metadados corrompidos geram aviso, veredito/laudo ausentes não retiram entrada cujos metadados continuam legíveis. Não ampliar validação fora do escopo.
- [ ] `TestLaudoErroEscrita` injeta erro no primeiro Write e após um prefixo aceito; errors.Is deve recuperar o erro original e o gerador deve terminar.
- [ ] Rodar `go test -timeout 120s ./... -run 'Test(HistoricoArquivos|HistoricoJSON|LaudoErro)'`; diagnosticar falhas antes de qualquer correção.

## Tarefa 3: integração e verificação final

**Files:** criar `cmd/parallax/check_history_integracao_test.go`; atualizar estado desta spec/plano e registrar evidências para o revisor no plano.

**Interfaces:** consumir `rodar(args ...string) (int, string, string)`, `julgamento.Execucao` e `veredito.Veredito.Validar`.

- [ ] `TestCheckHistoryRoundTrip` executa check --json --report em tabela: aprovado (0), candidata 500 (1), resposta truncada via --max-body (2). Evitar depender de latência real para definir o resultado, configurando limites altos neste teste.
- [ ] Para cada execução, validar contrato e resultado esperado; comparar integralmente stdout JSON com veredito.json e history show --json. Comparar bytes de report com laudo.md e history show.
- [ ] Validar id, data UTC não vazia dentro do intervalo da chamada, caminho absoluto da captura, URLs, resumo, total e incompletas no execucao.json. Conferir que a listagem contém o id e o resultado. Comparar estatísticas comuns entre metadados e veredito; incompletas só existe nos metadados do replay.
- [ ] Rodar `go test -timeout 120s ./...` e depois `go test -timeout 120s ./... -run 'Test(CheckErro|HistoryErro|CheckEntrada|CheckCancelamento|HistoricoArquivos|HistoricoJSON|LaudoErro|CheckHistoryRoundTrip)' -count=10`, sempre com caches em TEMP.
- [ ] Rodar `go build ./...` e `go vet ./...` com os mesmos caches. Conferir git diff --check e git status; não produzir binário no repositório.
- [ ] Usar requesting-code-review e verification-before-completion; corrigir achados demonstrados e repetir apenas verificações afetadas. Registrar evidências, lacunas e reservas sobrepostas para a etapa #34.

## Limitações conhecidas e entrega

- Memória consultada: decisions/parallax-replay-demanda-24.md, gotchas/go-cache-global-acesso-negado.md e gotchas/go-race-sem-cgo-windows.md. O check placeholder mencionado na decisão antiga já foi substituído pela parte 3 no código atual.
- Race detector não disponível nesta máquina Windows sem cgo; permissões 0700/0600 não verificáveis aqui. Manter os testes existentes sem remover ou pular novos testes para esconder falhas.
- Sem execução do binário/curl bloqueados neste ambiente; a integração usa as funções reais da CLI. Disco cheio não será induzido; falhas de escrita usam escritor com erro e caminho que é diretório.
- Registrar em ai-memory apenas novas descobertas duráveis; o Solaris captura automaticamente o relato da demanda.
- Estado inicial: árvore limpa, último commit a27656d. Código e testes ainda não alterados nesta retomada.
