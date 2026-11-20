# Plano: veredito, laudo e histórico (Parallax parte 3/3)

Demandas #31 (planejar) e #32 (implementar). Este arquivo é a spec e o plano
ao mesmo tempo. Decisão registrada no ai-memory:
`decisions/parallax-veredito-laudo-historico-demanda-31.md`.

## Decisões

1. O contrato (`docs/contrato.md`, `pkg/veredito`) não muda.
   - aprovado sai com 0; reprovado = `regressao` (1); inconclusivo = `revisar` (2); erro = 4.
   - Comparação incompleta (corpo acima de `--max-body`) já é achado `diferenca`/`branch`, logo dá `revisar`.
   - O código 3 deixa de ser usado pelo `check`; a constante continua no pacote.
2. `parallax replay` não muda o comportamento. Ganha só o campo aditivo `estatisticas.incompletas`.
3. `parallax check` aceita as flags do replay (menos `--out`) e mais:
   `--json` (imprime o veredito.json no lugar do laudo), `--report ARQ`
   (grava o laudo em arquivo; não pode ser a captura) e `--runs-dir`
   (padrão `.parallax/runs`).
4. Sem trocas comparadas (`Total == 0`): sai com 4, "nenhuma troca comparada", nada gravado.
5. **Falha ao gravar o histórico sai com 4** (escolha do usuário na demanda #32,
   opção A): o laudo continua aparecendo e o erro vai para o stderr.
6. Histórico em `<runs-dir>/<id>/` com `veredito.json`, `laudo.md` e `execucao.json`;
   pasta 0700, arquivos 0600, gravação atômica (temp na mesma pasta + rename);
   o id é validado (ULID de 26 caracteres) antes de ler.
7. `.parallax/` já está no `.gitignore`.

## Interfaces (`internal/julgamento`)

```go
var ErrSemEvidencia = errors.New("nenhuma troca comparada")
type Execucao struct { ID; Data (UTC); Captura (abs); Base; Candidata; Resultado; Resumo; Estatisticas repeticao.Estatisticas }
func Julgar(r repeticao.Resultado) (veredito.Veredito, error)
func Motivo(v veredito.Veredito) string
func EscreverLaudo(w io.Writer, e Execucao, v veredito.Veredito) error
var ErrNaoEncontrada = errors.New("execução não encontrada")
type Historico struct{ Dir string }
func (h Historico) Salvar(e Execucao, v veredito.Veredito, laudo []byte) error
func (h Historico) Listar() ([]Execucao, []error) // Data desc, depois ID desc
func (h Historico) Ler(id string) (Execucao, veredito.Veredito, []byte, error)
```

Laudo (Markdown): `# Laudo Parallax <id>` (data, captura, base, candidata),
`## Veredito`, `## Estatísticas`, `## Regressões de latência` (tabela p95
antes/depois/variação/origem), `## Diferenças por endpoint` (um `###` por
endpoint, até 10 itens de diff, curl), `## Comparações incompletas`. Seção
vazia mostra "Nenhuma.".

CLI: `check` → `Repetir` → `Julgar` → `EscreverLaudo` → salva histórico →
imprime laudo (ou JSON) → `--report` → sai com `CodigoSaida`.
`history [--runs-dir D]` lista; `history show <id> [--json] [--runs-dir D]`.

## Tarefas (TDD, um commit por tarefa)

0. Commitar este plano.
1. Contador `Incompletas` em `repeticao.Estatisticas` e `MarcadorIncompleta` exportado.
2. `Julgar` e `Motivo`.
3. `EscreverLaudo`.
4. `Historico` (Salvar/Listar/Ler).
5. Extrair `cmd/parallax/flags_repeticao.go` (replay_test continua verde).
6. `cmd/parallax/check.go`.
7. `cmd/parallax/history.go` e `main.go` (uso, `TestNaoImplementado` → check sem flags sai 4).
8. README.

## Verificação

```powershell
$env:GOCACHE = Join-Path $env:TEMP 'parallax-task32-go-build'
$env:GOMODCACHE = Join-Path $env:TEMP 'parallax-task32-go-mod'
go build ./...; go vet ./...; go test -timeout 120s ./...; go test -timeout 120s -count=10 ./internal/... ./cmd/...
```
