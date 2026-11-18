# Parallax 1/3 (base, contrato e gravação) — Plano de implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Criar o módulo Go do Parallax com o contrato do veredito (`pkg/veredito`), o motor de gravação (`pkg/motor`) e a CLI fina (`cmd/parallax`) com `record` funcionando.

**Architecture:** `pkg/veredito` é folha (só stdlib), importado pelo Solaris. `pkg/motor` é biblioteca: tipo `Troca`, JSONL em streaming, ULID e um `http.Handler` gravador que envolve o `ResponseWriter` de um `httputil.ReverseProxy`. `cmd/parallax` só lê flags, liga o `http.Server` e traduz erros em códigos de saída.

**Tech Stack:** Go (go.mod `go 1.22`; toolchain local 1.27.3), só biblioteca padrão: `net/http`, `net/http/httputil`, `net/http/httptest`, `encoding/json`, `bufio`, `crypto/rand`, `math/rand/v2`, `flag`, `os/signal`.

**Spec:** `docs/superpowers/specs/2026-11-18-parallax-base-design.md` (aprovada). Leia a spec inteira antes de qualquer tarefa: as tabelas de campos e os tipos de lá são normativos.

## Global Constraints

- Módulo `github.com/leonardolermen/parallax`; `go.mod` com `go 1.22`; zero dependências externas (`go.sum` não deve existir).
- `pkg/veredito` não importa nada de `pkg/motor` nem de `cmd/`.
- `pkg/motor` não usa `flag`, `os.Stdout`, `os.Stderr`, `fmt.Print*` nem `log`.
- Tags json exatamente como na spec (português, sem acento). Mudá-las quebra capturas.
- Máscara: valor `***`, cabeçalhos `Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, sem diferenciar maiúsculas, requisição e resposta. O cliente e o upstream nunca veem a máscara.
- Padrões: `--sample 1`, `--max 0` (sem limite), `--max-body 1048576` (1 MiB).
- Saídas: aprovado 0, regressão 1, revisar 2, não implementado 3, erro/uso 4.
- Mensagens da CLI, README, docs e commits em português.
- `.gitignore` inclui `.parallax/`. Nunca commitar binários, caches ou capturas geradas.
- Testes com `net/http/httptest`, ponta a ponta. `go build ./...`, `go vet ./...` e `go test ./...` passam no fim de cada tarefa. `-race` só se houver cgo (no Windows normalmente não há; não é critério).

## Review Focus

1. **`Max` sob concorrência:** 20 requisições paralelas com `Max: 3` geram exatamente 3 linhas, e todas as 20 recebem 200. (Tarefa 6)
2. **Cabeçalho sensível com vários valores:** dois `Set-Cookie` e `cookie` em minúsculas viram dois/um `***` na captura (a quantidade de valores é preservada), e o cliente recebe os dois `Set-Cookie` originais. (Tarefa 5)
3. **Upstream com caminho base:** `--upstream http://h/api` e requisição `/orders?x=1` chegam ao upstream como `/api/orders?x=1`; a captura guarda `caminho: "/orders"`, `query: "x=1"`. (Tarefa 5)
4. **Resposta sem `WriteHeader` explícito e sem corpo:** upstream que só faz `Write` grava status 200; um `204` e um `HEAD` gravam `corpo_resposta: ""` (campo presente). (Tarefa 5)
5. **Troca interrompida:** upstream que derruba a conexão no meio do corpo (via `http.Hijacker`) não gera linha e devolve a vaga do `Max`; a próxima requisição completa é gravada. (Tarefa 6)

---

## Estrutura de arquivos

```
go.mod
.gitignore                         (+ .parallax/)
README.md                          (reescrito na Tarefa 8)
docs/contrato.md
pkg/veredito/veredito.go           tipos, constantes, MarshalJSON
pkg/veredito/regras.go             CalcularResultado, CodigoSaida, Validar
pkg/veredito/veredito_test.go
pkg/veredito/regras_test.go
pkg/veredito/testdata/exemplo.json
pkg/motor/troca.go                 Troca
pkg/motor/jsonl.go                 Escritor, Leitor
pkg/motor/jsonl_test.go
pkg/motor/id.go                    NovoID, NovoIDEm
pkg/motor/id_test.go
pkg/motor/mascara.go               mascarar(http.Header) http.Header
pkg/motor/gravador.go              ConfigGravador, Gravador, NovoGravador
pkg/motor/resposta.go              escritor de resposta envolvido + tee do corpo
pkg/motor/gravador_test.go
cmd/parallax/main.go               main → run
cmd/parallax/record.go             subcomando record
cmd/parallax/main_test.go
```

---

### Task 1: Módulo e regras do veredito

**Files:**
- Create: `go.mod`, `pkg/veredito/veredito.go`, `pkg/veredito/regras.go`, `pkg/veredito/regras_test.go`
- Modify: `.gitignore` (acrescentar `.parallax/`)

**Interfaces:**
- Produces: tipos `Resultado`, `Tipo`, `Origem` (string nomeados) e constantes `Aprovado="aprovado"`, `Regressao="regressao"`, `Revisar="revisar"`, `ErroNovo="erro_novo"`, `ContratoQuebrado="contrato_quebrado"`, `Latencia="latencia"`, `Diferenca="diferenca"`, `Branch="branch"`, `PreExistente="pre_existente"`; `const Versao = 1`; `SaidaAprovado=0`, `SaidaRegressao=1`, `SaidaRevisar=2`, `SaidaNaoImplementado=3`, `SaidaErro=4`; structs `Veredito`, `Estatisticas`, `Latencias`, `Percentis`, `Achado`, `Lado`, `Mudanca` exatamente como na spec; `CalcularResultado(achados []Achado) Resultado`; `CodigoSaida(r Resultado) int`.

- [ ] **Step 1:** `go mod init github.com/leonardolermen/parallax`, ajuste a linha para `go 1.22`; acrescente `.parallax/` ao `.gitignore`.
- [ ] **Step 2: Teste de tabela que falha** — `TestCalcularResultado` em `regras_test.go`, casos (nome → achados → esperado):
  - vazio (`nil`) → `Aprovado`; slice vazio → `Aprovado`
  - `{ErroNovo, Branch}` → `Regressao`; `{ContratoQuebrado, Branch}` → `Regressao`; `{Latencia, Branch}` → `Regressao`
  - `{Diferenca, Branch}` → `Revisar`
  - `{ErroNovo, PreExistente}` → `Revisar`; `{Latencia, PreExistente}` → `Revisar`
  - `{Diferenca, Branch}, {ErroNovo, PreExistente}` → `Revisar`
  - `{Diferenca, Branch}, {ContratoQuebrado, Branch}` → `Regressao`

  `TestCodigoSaida`: `Aprovado→0`, `Regressao→1`, `Revisar→2`, `""→4`, `"outro"→4`; e confere que as constantes valem 0,1,2,3,4.
- [ ] **Step 3:** `go test ./pkg/veredito/` → FAIL (não compila).
- [ ] **Step 4:** Implemente tipos (doc comment em cada campo, em português) em `veredito.go` e as duas funções em `regras.go`.
- [ ] **Step 5:** `go test ./pkg/veredito/` → PASS; `go vet ./...` limpo.
- [ ] **Step 6:** Commit `feat(veredito): tipos do contrato e regra do resultado`.

### Task 2: Validar, exemplo e docs/contrato.md

**Files:**
- Modify: `pkg/veredito/veredito.go` (MarshalJSON), `pkg/veredito/regras.go` (Validar)
- Create: `pkg/veredito/veredito_test.go`, `pkg/veredito/testdata/exemplo.json`, `docs/contrato.md`

**Interfaces:**
- Consumes: Task 1.
- Produces: `func (v Veredito) MarshalJSON() ([]byte, error)` (emite `"achados":[]` quando nil); `func (v Veredito) Validar() error`.

- [ ] **Step 1: Testes que falham** em `veredito_test.go`:
  - `TestIdaEVolta`: monta um `Veredito` com 2 achados (um com `Diff` contendo `Antes` e `Depois`, outro com `Depois` ausente e `Local`), `json.Marshal` → `json.Unmarshal` → `reflect.DeepEqual` com o original.
  - `TestAchadosNuncaNull`: `Veredito{Versao:1, Resultado: Aprovado}` serializado contém `"achados":[]`.
  - `TestExemploCarregaEValida`: lê `testdata/exemplo.json`, `json.Unmarshal` com `DisallowUnknownFields` (via `json.Decoder`) sem erro, `Validar()` → nil, `Versao == Versao`, `len(Achados) >= 4` cobrindo os quatro tipos e as duas origens.
  - `TestDocIgualAoExemplo`: lê `../../docs/contrato.md`, extrai o primeiro bloco ` ```json ` depois do título `## Exemplo completo`, faz `Unmarshal` para `any` no bloco e no arquivo e compara com `reflect.DeepEqual` (imune a CRLF e indentação).
  - `TestValidar` (tabela): versão 2 → erro; resultado `"x"` → erro; achado com tipo `"x"` → erro; origem `"x"` → erro; resultado `Aprovado` com um achado `{ErroNovo, Branch}` → erro (não bate com a regra); exemplo válido → nil. Mensagens de erro em português citando o campo (ex.: `achados[0].tipo`).
- [ ] **Step 2:** `go test ./pkg/veredito/` → FAIL.
- [ ] **Step 3:** Implemente `MarshalJSON` (tipo alias local para evitar recursão; troca `nil` por `[]Achado{}`) e `Validar`.
- [ ] **Step 4:** Escreva `testdata/exemplo.json`: `resultado: "regressao"`, id ULID plausível (26 chars), 4 achados — `erro_novo/branch` (candidata `status 500`, `local: "internal/orders/handler.go:87"`), `contrato_quebrado/branch` com `diff` de campo removido (só `antes`) e tipo trocado, `latencia/branch`, `diferenca/pre_existente`; um lado com `status 0` e `erro: "timeout"`; `reproducao` como `curl -sS -X GET 'http://localhost:8080/orders/42?expand=items' -H 'Accept: application/json'`; traceIds de 32 hex, diferentes em cada lado.
- [ ] **Step 5:** Escreva `docs/contrato.md`: o que é o arquivo `veredito.json`; tabela de cada campo (nome json, tipo, significado) para `Veredito`, `Estatisticas`, `Latencias/Percentis`, `Achado`, `Lado`, `Mudanca`; valores de `tipo` e `origem` com o significado da demanda; a regra do resultado; tabela de códigos de saída (0–4); como importar (`import "github.com/leonardolermen/parallax/pkg/veredito"`); `## Exemplo completo` com o conteúdo de `exemplo.json`.
- [ ] **Step 6:** `go test ./pkg/veredito/` → PASS.
- [ ] **Step 7:** Commit `feat(veredito): validação, exemplo e documentação do contrato`.

### Task 3: Troca e JSONL

**Files:**
- Create: `pkg/motor/troca.go`, `pkg/motor/jsonl.go`, `pkg/motor/jsonl_test.go`

**Interfaces:**
- Produces:
  - `type Troca struct` com os campos e tags da tabela da spec (`Horario time.Time "horario"`, `Metodo`, `Caminho`, `Query`, `CabecalhosRequisicao http.Header "cabecalhos_requisicao"`, `CorpoRequisicao []byte "corpo_requisicao"`, `CorpoRequisicaoTruncado bool "corpo_requisicao_truncado,omitempty"`, `Status int`, `CabecalhosResposta http.Header`, `CorpoResposta []byte`, `CorpoRespostaTruncado bool ",omitempty"`, `LatenciaMs float64 "latencia_ms"`), doc comment por campo.
  - `NovoEscritor(w io.Writer) *Escritor`; `(*Escritor) Escrever(Troca) error`; `(*Escritor) Flush() error`; `(*Escritor) Fechar() error` (flush + marca fechado; não fecha `w`); `var ErrEscritorFechado = errors.New(...)` devolvido por `Escrever` depois de `Fechar`.
  - `NovoLeitor(r io.Reader) *Leitor`; `(*Leitor) Proxima() (Troca, error)` (`io.EOF` no fim).

- [ ] **Step 1: Testes que falham:**
  - `TestJSONLIdaEVolta`: escreve 3 trocas (uma com corpo binário `[]byte{0,255,10}`, uma com corpo vazio, uma truncada), `Flush`, lê de volta, `reflect.DeepEqual` campo a campo (compare `Horario` com `Equal`); depois `io.EOF`.
  - `TestJSONLCampos`: a linha crua contém `"horario":"` em RFC3339Nano, `"corpo_resposta":""` para corpo vazio (não omitido) e não contém `_truncado` quando falso; termina com `\n`.
  - `TestJSONLLinhaGrande`: troca com corpo de 1 MiB lida de volta igual (prova que não usa `bufio.Scanner`).
  - `TestJSONLLinhaInvalida`: entrada `linha válida\n\n{quebrado\n` → primeira `Proxima` ok, segunda devolve erro contendo `linha 3`; linhas em branco são puladas; última linha sem `\n` final também é lida.
  - `TestEscritorConcorrente`: 50 goroutines escrevendo, `Flush`, o leitor lê 50 trocas válidas.
  - `TestEscritorFechado`: `Fechar()` e depois `Escrever` → `errors.Is(err, ErrEscritorFechado)`.
- [ ] **Step 2:** `go test ./pkg/motor/` → FAIL.
- [ ] **Step 3:** Implemente. `Escrever`: `json.Marshal` fora do lock, depois `mutex` → checa fechado → `bufio.Writer.Write(linha+'\n')`. `Proxima`: `bufio.Reader.ReadBytes('\n')`, conta linhas, ignora `bytes.TrimSpace` vazio.
- [ ] **Step 4:** `go test ./pkg/motor/` → PASS.
- [ ] **Step 5:** Commit `feat(motor): tipo Troca e leitura/escrita JSONL em streaming`.

### Task 4: Id da execução (ULID)

**Files:**
- Create: `pkg/motor/id.go`, `pkg/motor/id_test.go`

**Interfaces:**
- Produces: `NovoID() string` (usa `time.Now()` e `crypto/rand`; em erro do `crypto/rand` dá `panic`, que não acontece na prática); `NovoIDEm(t time.Time, aleatorio io.Reader) (string, error)`.

- [ ] **Step 1: Testes que falham:**
  - `TestNovoIDFormato`: 26 caracteres, todos em `0123456789ABCDEFGHJKMNPQRSTVWXYZ`; dois `NovoID()` seguidos são diferentes.
  - `TestNovoIDEmConhecido`: `NovoIDEm(time.UnixMilli(1469918176385), bytes.NewReader(make([]byte,10)))` → prefixo de tempo `01ARYZ6S41` e sufixo `0000000000000000` (vetor da especificação ULID).
  - `TestNovoIDOrdenado`: ids de `t` e `t+1ms` (mesmo aleatório) comparam `<` como string.
  - `TestNovoIDEmErroAleatorio`: leitor que falha → erro.
- [ ] **Step 2:** FAIL. **Step 3:** Implemente: 16 bytes (6 de ms big-endian + 10 aleatórios via `io.ReadFull`) codificados em 26 símbolos base32 Crockford, 5 bits por vez a partir do bit mais alto (o primeiro símbolo usa só 3 bits). **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(motor): gerador de id da execução (ULID)`.

### Task 5: Gravador — caminho feliz, máscara, amostra e corpo

**Files:**
- Create: `pkg/motor/mascara.go`, `pkg/motor/resposta.go`, `pkg/motor/gravador.go`, `pkg/motor/gravador_test.go`

**Interfaces:**
- Consumes: `Troca`, `Escritor` (Task 3).
- Produces: `ConfigGravador` exatamente como na spec (`Upstream *url.URL`, `Saida *Escritor`, `Amostra float64`, `Max int`, `MaxCorpo int64`, `Sorteio func() float64`, `Transport http.RoundTripper`); `const MaxCorpoPadrao = 1 << 20`; `NovoGravador(cfg ConfigGravador) (*Gravador, error)`; `(*Gravador) ServeHTTP`; `(*Gravador) Gravadas() int` (linhas efetivamente escritas); `(*Gravador) Erro() error` (primeiro erro de escrita); `(*Gravador) Fechar() error` (chama `Saida.Fechar()`). Helper de teste reutilizado na Task 6: `montar(t, upstream http.Handler, cfg ConfigGravador) (cliente *httptest.Server, captura *bytes.Buffer, g *Gravador)` que preenche `Upstream` e `Saida` e lê as trocas com `lerTrocas(t, buf) []Troca` (após `g.Fechar()`).

- [ ] **Step 1: Testes que falham** (`gravador_test.go`):
  - `TestNovoGravadorValida` (tabela): sem `Upstream` → erro; sem `Saida` → erro; `Amostra` -0.1 e 1.1 → erro; `Max -1` → erro; `MaxCorpo -1` → erro; válido → ok. Observação: `Amostra` zero é válido (nada grava); a CLI é quem aplica o padrão 1.
  - `TestGravaGETePOST` (o teste de ponta a ponta do critério): upstream ecoa o corpo, devolve `Set-Cookie: s=1`, `Set-Cookie: t=2`, `X-Api-Key: segredo-resp`, status 201 para POST e 200 para GET. Cliente faz `GET /pedidos/42?expand=itens&x=1` com `Authorization: Bearer abc`, `cookie: a=b` (minúsculo via `req.Header["cookie"]`) e `X-API-KEY: k1`; e `POST /pedidos` com corpo JSON `{"total":10}`, `Content-Type`. Afirma: o upstream recebeu `Authorization` e `X-Api-Key` originais; o cliente recebeu os dois `Set-Cookie` originais e corpo ecoado; captura com 2 linhas; por troca: `Metodo`, `Caminho`, `Query == "expand=itens&x=1"`, `CorpoRequisicao`/`CorpoResposta` iguais aos enviados/recebidos, `Status` 200/201, `LatenciaMs > 0`, `Horario` não zero; `CabecalhosRequisicao.Values("Authorization") == ["***"]`, idem `Cookie`, `X-Api-Key`; `CabecalhosResposta.Values("Set-Cookie") == ["***","***"]`, `X-Api-Key == ["***"]`; `CabecalhosRequisicao.Get("Host")` é o host do servidor do gravador; nenhum `segredo`, `abc`, `k1`, `s=1` aparece no texto cru da captura. (Cobre o Review Focus 2.)
  - `TestUpstreamComCaminhoBase` (Review Focus 3).
  - `TestSemWriteHeaderESemCorpo` (Review Focus 4): upstream que só faz `w.Write([]byte("ok"))` → status 200; rota `204` e um `HEAD` → `CorpoResposta` vazio e a linha crua contém `"corpo_resposta":""`.
  - `TestAmostraZero`: `Amostra: 0`, 5 GETs, todos 200 com corpo certo, captura vazia, `Gravadas()==0`.
  - `TestAmostraSorteio`: `Amostra: 0.5`, `Sorteio` devolve em sequência `0.1, 0.9, 0.4, 0.6` → 2 linhas (1ª e 3ª requisição, conferindo pelo caminho).
  - `TestMax`: `Max: 2`, 5 GETs sequenciais, todos 200, 2 linhas (as duas primeiras), `Gravadas()==2`.
  - `TestMaxCorpo`: `MaxCorpo: 10`, POST com corpo de 25 bytes e resposta de 30 bytes: upstream recebe os 25, cliente recebe os 30 inteiros; captura tem os 10 primeiros de cada e `CorpoRequisicaoTruncado` e `CorpoRespostaTruncado` true. Corpo exatamente de 10 bytes → não truncado.
- [ ] **Step 2:** `go test ./pkg/motor/ -run 'Gravador|Grava|Upstream|SemWrite|Amostra|Max'` → FAIL.
- [ ] **Step 3:** Implemente:
  - `mascara.go`: `mascarar(h http.Header) http.Header` devolve um `Clone` em que toda chave cujo `strings.EqualFold` bate com um dos quatro nomes tem cada valor trocado por `***` (preserva a quantidade). Nunca altera o original.
  - `gravador.go`: `httputil.ReverseProxy` com `Rewrite: func(pr){ pr.SetURL(up); pr.SetXForwarded() }`, `Transport` da config, `ErrorHandler` que marca a falha num `*atomic.Bool` guardado no contexto da requisição e responde 502. `ServeHTTP` segue o fluxo 1–6 da spec: sorteio → reserva de vaga com `CompareAndSwap` em loop (sem `Max`, sempre reserva) → clona cabeçalhos da requisição e acrescenta `Host: r.Host` → troca `r.Body` pelo tee limitado (se `r.Body` não for nil/`http.NoBody`) → envolve `w` → `proxy.ServeHTTP` → monta `Troca` (`latencia_ms` = `time.Since(inicio)` em ms float, `Horario` UTC) → `Saida.Escrever`; em erro guarda o primeiro em `Erro()` e devolve a vaga; em sucesso incrementa `Gravadas`.
  - `resposta.go`: tee de requisição (`io.ReadCloser` que copia até `MaxCorpo` e marca truncado ao ver o byte seguinte) e escritor de resposta envolvido: `WriteHeader(code)` com `code < 200` só repassa (respostas 1xx não são o status final); o primeiro `WriteHeader` final ou `Write` registra status (200 se implícito) e captura `Clone` de `Header()`; `Write` repassa tudo e copia até o limite; `Flush()` usa `http.NewResponseController(inner).Flush()`; `Unwrap() http.ResponseWriter`.
- [ ] **Step 4:** `go test ./pkg/motor/` → PASS; `go vet ./...` limpo.
- [ ] **Step 5:** Commit `feat(motor): gravador como proxy reverso com máscara, amostra, limite e corpo máximo`.

### Task 6: Gravador — falhas, interrupções, streaming e concorrência

**Files:**
- Modify: `pkg/motor/gravador.go`, `pkg/motor/resposta.go`, `pkg/motor/gravador_test.go`

**Interfaces:**
- Consumes: Task 5 (incluindo o helper `montar`).

- [ ] **Step 1: Testes que falham:**
  - `TestUpstreamFora`: `Upstream` aponta para um `httptest.Server` já fechado → cliente recebe 502, captura vazia, `Gravadas()==0`.
  - `TestFalhaDevolveVaga`: `Max: 1` e um `Transport` falso que devolve erro na 1ª chamada e delega a `http.DefaultTransport` nas seguintes → 1ª requisição 502 sem linha; 2ª requisição 200 e gravada (a vaga voltou).
  - `TestTrocaInterrompida` (Review Focus 5): upstream escreve cabeçalhos + `Content-Length: 100` + 10 bytes, faz `Flush`, `Hijack` e fecha a conexão. `Max: 1`. Cliente recebe erro de leitura (ignore). Captura vazia; uma segunda requisição para rota normal é gravada.
  - `TestClienteDesconectou`: upstream bloqueia até um canal fechar; cliente usa contexto cancelado após o upstream receber a requisição; depois libera o upstream; captura vazia (espere o handler terminar com `httptest.Server.Close()` antes de ler).
  - `TestSSEFlush`: upstream envia `data: 1\n\n`, `Flush`, e espera um canal antes de enviar `data: 2\n\n`. O cliente lê a primeira mensagem **antes** de o teste liberar o canal (prova que o flush atravessa o gravador); depois termina; captura tem 1 troca com o corpo completo.
  - `TestConcorrencia`: 50 requisições paralelas, cada uma com caminho único → 50 linhas válidas, conjunto de caminhos igual ao enviado.
  - `TestMaxConcorrente` (Review Focus 1).
  - `TestFechar`: depois de `g.Fechar()`, uma requisição ainda é respondida 200 mas não é gravada, e `Erro()` continua nil (escrita recusada por fechamento não conta como erro).
- [ ] **Step 2:** FAIL nos que dependem de comportamento novo.
- [ ] **Step 3:** Implemente: `defer` com `recover` que, ao ver `http.ErrAbortHandler` (ou qualquer panic), devolve a vaga e repassa o `panic`; descarte quando a marca de falha do proxy está ligada ou `r.Context().Err() != nil`; `ErrEscritorFechado` devolve a vaga sem registrar em `Erro()`.
- [ ] **Step 4:** `go test ./pkg/motor/ -count=3` → PASS estável. Se houver cgo: `go test -race ./pkg/motor/`.
- [ ] **Step 5:** Commit `feat(motor): descarta trocas incompletas e preserva streaming`.

### Task 7: CLI

**Files:**
- Create: `cmd/parallax/main.go`, `cmd/parallax/record.go`, `cmd/parallax/main_test.go`

**Interfaces:**
- Consumes: `motor.NovoGravador`, `motor.NovoEscritor`, `motor.MaxCorpoPadrao`, constantes `veredito.Saida*`.
- Produces: `var versao = "dev"`; `func run(args []string, stdout, stderr io.Writer) int`; `func gravar(ctx context.Context, args []string, pronto func(endereco string), stderr io.Writer) int` (o `run` chama com `signal.NotifyContext(os.Interrupt, syscall.SIGTERM)` e `pronto` que imprime `gravando em <addr> → <upstream>` no stderr).

- [ ] **Step 1: Testes que falham:**
  - `TestVersion`: `run(["version"])` → 0, stdout `parallax dev\n`.
  - `TestHelp`: `help`, `-h`, `--help` → 0, stdout contém `record`, `replay`, `check`, `version`; sem argumentos → 4 e uso no stderr; `xyz` → 4, stderr contém `comando desconhecido`.
  - `TestNaoImplementado`: `replay` e `check` → 3, stderr contém `ainda não implementado`.
  - `TestRecordFlagsInvalidas` (tabela, todos → 4 com mensagem no stderr): sem `--upstream`; sem `--out`; `--upstream ftp://x`; `--upstream` sem host; `--sample 1.5`; `--max -1`; `--max-body -1`; flag desconhecida.
  - `TestRecordPontaAPonta`: upstream httptest; `gravar(ctx, ["--listen","127.0.0.1:0","--upstream",up.URL,"--out",tmp/"c.jsonl","--max","5"], pronto, &stderr)` numa goroutine; `pronto` envia o endereço num canal; faz 2 GETs com `Authorization`; cancela o `ctx`; espera retorno 0; o arquivo tem 2 linhas válidas (lidas com `motor.NovoLeitor`) com `Authorization` mascarado; stderr contém `2 trocas gravadas`. Rode também com o arquivo já existente com lixo para provar que é sobrescrito.
- [ ] **Step 2:** FAIL.
- [ ] **Step 3:** Implemente com um `flag.FlagSet` por subcomando (`ContinueOnError`, saída para `stderr`). Padrões: `--listen :9100`, `--sample 1`, `--max 0`, `--max-body` = `motor.MaxCorpoPadrao`. `gravar`: valida, `os.Create(--out)`, `net.Listen` (erro → 4), `http.Server{Handler: g}` com `Serve(ln)`; em `ctx.Done()`: `Shutdown` com prazo de 5 s → `g.Fechar()` → `f.Close()` → imprime `N trocas gravadas em <arquivo>`; se `g.Erro() != nil`, imprime e devolve 4. `main` = `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))`.
- [ ] **Step 4:** `go build ./... && go vet ./... && go test ./...` → PASS.
- [ ] **Step 5:** Commit `feat(cli): subcomandos record, replay, check, version e help`.

### Task 8: README e verificação manual

**Files:**
- Modify: `README.md`

- [ ] **Step 1:** Reescreva o README em português: o que é o Parallax (base × candidata, tráfego gravado, veredito; quem chama é o Solaris); estado atual (demanda 1/3: `record` pronto, `replay`/`check` saem com 3); estrutura de pacotes; como compilar (`go build -o parallax ./cmd/parallax`, no Windows `parallax.exe`) e testar (`go test ./...`); exemplo de `record` na frente de um serviço real (`python -m http.server 8080` como serviço de exemplo, `parallax record --listen :9100 --upstream http://localhost:8080 --out captura.jsonl`, `curl -H 'Authorization: Bearer x' 'http://localhost:9100/?q=1'`, Ctrl+C, ver a linha); tabela das flags com padrões; formato da captura (tabela de campos igual à spec, uma linha de exemplo real produzida no Step 2, como decodificar o corpo base64); o que não é gravado (falhas do proxy, trocas interrompidas, cliente desconectado, depois do `--max`); segurança (métodos inseguros são gravados mas a demanda 2 só repete GET/HEAD/OPTIONS por padrão); códigos de saída e link para `docs/contrato.md`.
- [ ] **Step 2:** Rode o exemplo de verdade: compile para um diretório fora do repositório (ou para `parallax.exe`, ignorado pelo `.gitignore`), suba um upstream (o `python -m http.server` do README, ou um `httptest`/`go run` temporário fora do repo se não houver Python), grave 2 requisições, encerre com Ctrl+C (ou `taskkill` / sinal equivalente — no Windows, se não der para mandar Ctrl+C a um processo em segundo plano, registre isso e confie no `TestRecordPontaAPonta`), confira o arquivo. Cole no README a linha real obtida.
- [ ] **Step 3:** `git status` sem binários nem capturas; commit `docs: README em português com exemplo de gravação`.

---

## Verificação final

`go build ./... && go vet ./... && go test ./... -count=1` com saída limpa; `go list -deps ./pkg/veredito` só mostra pacotes da stdlib; `grep -rn "fmt.Print\|os.Std\|\"flag\"\|\"log\"" pkg/motor` vazio (exceto testes); `ls go.sum` inexistente.
