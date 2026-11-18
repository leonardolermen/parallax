# Parallax 1/3: base, contrato do veredito e gravação — design

Data: 2026-11-18 · Demanda #18 · Branch `solaris/task-18`

## Intenção

O Parallax julga o comportamento das mudanças feitas por agentes de código. Ele
recebe duas versões de um serviço que já estão rodando (base e candidata),
repete contra as duas um tráfego real gravado, compara as respostas e devolve um
veredito estruturado. Quem chama é o Solaris, que decide o que fazer com o
veredito.

Esta demanda (1 de 3) entrega a estrutura do projeto, o contrato do veredito e a
gravação de tráfego. Repetição, comparação, laudo e histórico ficam para as
demandas 2 e 3, mas o esqueleto já os recebe.

## Objetivos

- Módulo Go `github.com/leonardolermen/parallax`, Go 1.22+, só biblioteca padrão.
- `pkg/veredito`: tipos do contrato, regra do resultado e código de saída, sem
  nenhuma dependência interna (o Solaris importa este pacote).
- `pkg/motor`: biblioteca com o tipo `Troca`, leitor e escritor JSONL em
  streaming, gerador de id (ULID) e o gravador (`http.Handler`).
- `cmd/parallax`: CLI fina com `record`, `replay`, `check`, `version`, `help`.
- `docs/contrato.md` e `pkg/veredito/testdata/exemplo.json`.
- README em português com exemplos que rodam.

## Não objetivos

- Replay, comparação, classificação de achados, laudo, histórico em
  `.parallax/runs/` (demandas 2 e 3).
- Leitura do `parallax.yaml` (rotas idempotentes) — demanda 2.
- Servidor, interface, fila, GitHub App. Nenhuma chamada a modelo de IA.

## Anti-objetivos (falha mesmo que "funcione")

- O cliente receber algo diferente do que o upstream mandou (cabeçalho
  mascarado, corpo cortado, streaming atrasado).
- Segredos (Authorization, Cookie, Set-Cookie, X-Api-Key) aparecendo na captura.
- Linhas do JSONL corrompidas ou intercaladas por concorrência, ou perdidas no
  Ctrl+C.
- `pkg/veredito` importando qualquer coisa de `pkg/motor` ou `cmd/`.
- `pkg/motor` lendo flags ou escrevendo no terminal.

## Abordagens consideradas (captura de cada troca)

- **A — envolver o `ResponseWriter` (escolhida pelo usuário).** O gravador põe
  um escritor de resposta entre o proxy e o cliente: tudo vai direto ao cliente
  e uma cópia (até o limite) fica guardada. O corpo da requisição é lido com um
  tee, também até o limite. Mantém streaming e SSE (o `Flush` é repassado).
  Custo: o proxy também escreve as próprias falhas (502 com upstream fora); elas
  são identificadas e não são gravadas.
- **B — `ModifyResponse` do proxy.** Também mantém streaming, mas o status, os
  cabeçalhos e o fim do corpo ficam espalhados entre ganchos diferentes e a
  gravação acontece no `Close` do corpo: mais espaço para bugs de concorrência
  e troca perdida.
- **C — ler a resposta inteira antes de repassar.** Quebra streaming e SSE.
  Descartada.

## Decisões do usuário

- Corpos grandes: limite por corpo `--max-body` (padrão 1 MiB). Acima dele o
  cliente recebe tudo, mas a troca é gravada com o corpo cortado e marcada com
  `corpo_requisicao_truncado` / `corpo_resposta_truncado`.
- Captura pela abordagem A.

## Formato da captura (JSONL)

Uma troca por linha, UTF-8, terminada em `\n`. Tipo `motor.Troca`:

| campo json | tipo Go | significado |
|---|---|---|
| `horario` | `time.Time` | início da requisição, RFC3339Nano (UTC) |
| `metodo` | `string` | método HTTP |
| `caminho` | `string` | caminho da URL, sem query (`r.URL.Path`) |
| `query` | `string` | query crua, sem `?` (`r.URL.RawQuery`); `""` se não houver |
| `cabecalhos_requisicao` | `http.Header` | cabeçalhos da requisição, sensíveis mascarados |
| `corpo_requisicao` | `[]byte` (base64 no JSON) | corpo da requisição |
| `corpo_requisicao_truncado` | `bool`, omitempty | `true` se o corpo passou do limite |
| `status` | `int` | status devolvido ao cliente |
| `cabecalhos_resposta` | `http.Header` | cabeçalhos da resposta, sensíveis mascarados |
| `corpo_resposta` | `[]byte` (base64) | corpo da resposta como chegou ao cliente |
| `corpo_resposta_truncado` | `bool`, omitempty | idem para a resposta |
| `latencia_ms` | `float64` | do início da requisição até o fim da resposta, com casas decimais |

`[]byte` vira base64 padrão (`encoding/base64.StdEncoding`) pelo próprio
`encoding/json`; corpo vazio é `""` (o campo não é omitido). O `Host` da
requisição entra em `cabecalhos_requisicao` como `Host`, porque o Go o retira de
`r.Header`. As tags json são estáveis: mudá-las quebra capturas antigas.

Corpos comprimidos (ex.: `Content-Encoding: gzip`) são gravados como vieram,
sem descompactar (decisão minha; a demanda 2 decide como comparar).

## Contrato do veredito (`pkg/veredito`)

Tipos string nomeados com constantes:

- `Resultado`: `aprovado`, `regressao`, `revisar`.
- `Tipo`: `erro_novo`, `contrato_quebrado`, `latencia`, `diferenca`.
- `Origem`: `branch`, `pre_existente`.

`const Versao = 1`.

```go
type Veredito struct {
    Versao       int          `json:"versao"`
    ID           string       `json:"id"`
    Resultado    Resultado    `json:"resultado"`
    Resumo       string       `json:"resumo"`
    Estatisticas Estatisticas `json:"estatisticas"`
    Achados      []Achado     `json:"achados"` // nunca null: [] quando vazio
}

type Estatisticas struct {
    Total      int       `json:"total"`
    Identicas  int       `json:"identicas"`
    Diferentes int       `json:"diferentes"`
    Latencia   Latencias `json:"latencia"`
}

type Latencias struct {
    Base      Percentis `json:"base"`
    Candidata Percentis `json:"candidata"`
    Delta     Percentis `json:"delta"` // candidata - base
}

type Percentis struct {
    P50 float64 `json:"p50_ms"`
    P95 float64 `json:"p95_ms"`
}

type Achado struct {
    Tipo       Tipo        `json:"tipo"`
    Origem     Origem      `json:"origem"`
    Endpoint   string      `json:"endpoint"`   // "GET /orders/{id}"
    Reproducao string      `json:"reproducao"` // curl pronto
    Base       Lado        `json:"base"`
    Candidata  Lado        `json:"candidata"`
    Diff       []Mudanca   `json:"diff,omitempty"`
    Local      string      `json:"local,omitempty"` // "arquivo.go:42"
}

type Lado struct {
    Status     int     `json:"status"`      // 0 = sem resposta (timeout, conexão recusada)
    Corpo      string  `json:"corpo"`       // resumido
    LatenciaMs float64 `json:"latencia_ms"`
    TraceID    string  `json:"trace_id"`
    Erro       string  `json:"erro,omitempty"` // "timeout", "conexao_recusada"...
}

type Mudanca struct {
    Caminho string          `json:"caminho"`          // "$.items[0].price"
    Antes   json.RawMessage `json:"antes,omitempty"`  // ausente = campo não existia
    Depois  json.RawMessage `json:"depois,omitempty"` // ausente = campo removido
}
```

Decisões minhas no contrato:
- `endpoint` é uma string só (`"GET /orders/{id}"`), não um objeto.
- `Lado.Erro` (opcional) diferencia timeout e conexão recusada, já que o status
  é 0 nesses casos.
- `antes`/`depois` como `json.RawMessage` preservam o valor exato (número,
  string, objeto) e permitem distinguir "ausente" de `null`.
- Percentis em ms com casas decimais, nomes `p50_ms`/`p95_ms`.

Funções puras:
- `CalcularResultado(achados []Achado) Resultado`
  - `regressao` se algum achado tem tipo `erro_novo`, `contrato_quebrado` ou
    `latencia` **e** origem `branch`;
  - senão `revisar` se há qualquer achado (diferença ou pré-existente);
  - senão `aprovado`.
- `CodigoSaida(r Resultado) int`: aprovado 0, regressao 1, revisar 2;
  qualquer outro valor → 4 (erro).
- Constantes de saída: `SaidaAprovado=0`, `SaidaRegressao=1`, `SaidaRevisar=2`,
  `SaidaNaoImplementado=3`, `SaidaErro=4` (decisão minha: 3 é o "ainda não
  implementado" pedido; 4 cobre uso errado e falha de execução, para não
  colidir com 0/1/2).
- `(Veredito) Validar() error`: confere versão, enums conhecidos e se
  `resultado` bate com `CalcularResultado(achados)`.

`docs/contrato.md` documenta cada campo, a regra, os códigos de saída e um JSON
completo idêntico a `pkg/veredito/testdata/exemplo.json` (um teste garante que
o bloco JSON do doc e o arquivo são iguais, para não divergirem).

## Motor (`pkg/motor`)

### JSONL

- `NovoEscritor(w io.Writer) *Escritor` — `Escrever(Troca) error` com mutex,
  sobre `bufio.Writer`; `Flush() error`. O escritor não fecha `w`.
- `NovoLeitor(r io.Reader) *Leitor` — `Proxima() (Troca, error)`, devolve
  `io.EOF` no fim. Usa `bufio.Reader.ReadBytes('\n')` (não `bufio.Scanner`,
  cujo limite de 64 KiB por linha quebraria corpos de 1 MiB em base64). Linhas
  em branco são ignoradas; linha inválida devolve erro com o número da linha.

### Id da execução

- `NovoID() string` — ULID: 48 bits de milissegundos + 80 bits de
  `crypto/rand`, em base32 Crockford, 26 caracteres, ordenável por tempo.
- `NovoIDEm(t time.Time, aleatorio io.Reader) (string, error)` para teste.
- Monotonicidade dentro do mesmo milissegundo não é garantida (não é
  necessária: um id por execução).

### Gravador

```go
type ConfigGravador struct {
    Upstream  *url.URL     // obrigatório
    Saida     *Escritor    // obrigatório
    Amostra   float64      // 0..1; fora disso é erro
    Max       int          // 0 = sem limite; <0 é erro
    MaxCorpo  int64        // 0 = padrão 1 MiB; <0 é erro
    Sorteio   func() float64 // opcional (teste); padrão math/rand/v2
    Transport http.RoundTripper // opcional (teste)
}

func NovoGravador(cfg ConfigGravador) (*Gravador, error) // *Gravador é http.Handler
func (g *Gravador) Gravadas() int
```

Fluxo por requisição:
1. Sorteia a amostragem (`Sorteio() < Amostra`; com 0 nunca grava, com 1
   sempre). Se não vai gravar, só repassa ao proxy.
2. Reserva uma vaga no `Max` com contador atômico (compare-and-swap). Sem vaga,
   só repassa. A vaga é reservada antes de saber se a troca será descartada
   (falha do proxy); nesse caso a vaga é devolvida.
3. Envolve `r.Body` num tee limitado a `MaxCorpo` (o proxy lê o corpo inteiro;
   a cópia para no limite e marca truncado).
4. Envolve o `ResponseWriter`: guarda status, captura cabeçalhos no
   `WriteHeader`, copia o corpo até o limite, repassa `Flush` (via
   `http.ResponseController`/`Unwrap`) para SSE.
5. `ErrorHandler` do proxy marca a requisição como "falha do proxy" (via
   contexto) e responde 502; essas trocas não são gravadas.
6. No fim, mascara cópias dos cabeçalhos (nunca os originais) e escreve a linha.

Mascaramento: `Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, comparando
sem diferenciar maiúsculas; cada valor vira `***`. Requisição e resposta. O
upstream e o cliente recebem os valores originais.

Trocas incompletas não são gravadas (decisão minha): se o upstream derruba a
conexão no meio do corpo, o `ReverseProxy` dá `panic(http.ErrAbortHandler)`; o
gravador faz `recover`, descarta a troca, devolve a vaga e repassa o `panic`.
Também não grava se o cliente desconectou antes do fim (contexto cancelado).
Assim a captura só tem respostas completas, e não aparecem diferenças falsas na
demanda 2. Nenhum campo novo no formato.

Host e cabeçalhos repassados (decisão minha): o proxy usa `Rewrite` com
`SetURL(upstream)`, que manda ao upstream o host do `--upstream` (funciona com
ingress, virtual host e TLS/SNI), e `SetXForwarded()`, que preserva o host
original em `X-Forwarded-Host`. Na captura fica o `Host` que o cliente mandou.

Máscara e replay: a demanda pede o valor `***`, e ele é o marcador definitivo.
Na demanda 2, o replay pode identificar `***` e trocar pela credencial
configurada. Esta demanda não faz essa troca.

Erro ao escrever no arquivo: não derruba o proxy; o gravador guarda o primeiro
erro e expõe `Erro() error` para a CLI reportar no encerramento (decisão minha).

## CLI (`cmd/parallax`)

- `parallax record --listen :9100 --upstream URL --out captura.jsonl
  [--sample 1] [--max 0] [--max-body 1048576]`
  - abre/cria o arquivo (trunca se existir — decisão minha; documentado),
    monta o gravador, sobe `http.Server`;
  - `signal.NotifyContext` (Ctrl+C/SIGTERM) → `Shutdown` com prazo de 5 s →
    `Gravador.Fechar()` → `Flush` → `Close`; imprime no stderr quantas trocas
    foram gravadas.
  - Requisições ainda abertas depois do prazo (ex.: SSE infinito) são
    descartadas. `Gravador.Fechar()` marca o gravador como fechado com o mutex,
    e daí em diante ele recusa escritas (não grava em arquivo fechado). Todas
    as trocas completas antes do Ctrl+C ficam no arquivo. (Decisão minha.)
  - Ao chegar no `--max`, o processo continua no ar repassando sem gravar, como
    pede a demanda. Ele não encerra sozinho.
- `parallax replay`, `parallax check`: "ainda não implementado" no stderr,
  saída 3.
- `parallax version`: imprime `parallax <versão>`; `var versao = "dev"`,
  sobrescrevível por `-ldflags "-X main.versao=..."`.
- `parallax help` (e sem argumentos, `-h`, `--help`): uso geral, saída 0 para
  `help`, 4 sem argumentos ou comando desconhecido.
- Erros de flag/validação: mensagem no stderr, saída 4.
- `main` só chama `run(args, stdout, stderr) int` para ser testável.

## Testes (httptest, ponta a ponta)

- Gravador: upstream httptest que ecoa e define `Set-Cookie`; gravador na
  frente; GET com query e POST com corpo, com Authorization/Cookie/X-Api-Key em
  maiúsculas/minúsculas variadas. Relê o JSONL e confere método, caminho,
  query, corpos decodificados, status, `latencia_ms > 0`, máscara nos dois
  lados, e que o cliente recebeu `Set-Cookie` original.
- `Max` (ex.: 2 de 5 requisições gravadas, todas respondidas), `Amostra 0`
  (nada gravado, tudo respondido), truncamento por `MaxCorpo`, upstream fora
  (502, nada gravado), concorrência (N goroutines, N linhas válidas, `-race`
  se disponível), SSE com flush chegando antes do fim.
- JSONL: ida e volta; linha > 64 KiB; linha inválida com número.
- ULID: 26 caracteres, alfabeto Crockford, ordenação por tempo.
- Veredito: ida e volta de um veredito de exemplo; `testdata/exemplo.json`
  carrega e passa em `Validar`; bloco do `docs/contrato.md` igual ao arquivo;
  tabela de `CalcularResultado` e `CodigoSaida`.
- CLI: `run` com `version`, `help`, `replay`/`check` (3), comando desconhecido (4).

## Deixado ao construtor

Nomes internos, divisão em arquivos, mensagens exatas da CLI, conteúdo do
veredito de exemplo, ordem dos commits.
