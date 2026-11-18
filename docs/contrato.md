# Contrato do veredito (`veredito.json`)

Ao fim de cada execução, o Parallax escreve um `veredito.json`: o que está
errado, onde e como reproduzir. Quem lê esse arquivo é o Solaris, que decide o
que fazer com ele.

Os tipos estão em Go no pacote `pkg/veredito`, que só depende da biblioteca
padrão:

```go
import "github.com/leonardolermen/parallax/pkg/veredito"

var v veredito.Veredito
err := json.Unmarshal(dados, &v)
if err == nil {
    err = v.Validar()
}
```

Versão atual do formato: `1` (`veredito.Versao`). Ela muda quando o contrato
muda de forma incompatível.

## Veredito (raiz)

| campo | tipo | significado |
|---|---|---|
| `versao` | inteiro | versão do formato; hoje sempre `1` |
| `id` | string | id da execução (ULID, 26 caracteres); a execução fica em `.parallax/runs/<id>/` |
| `resultado` | string | `aprovado`, `regressao` ou `revisar` (veja a regra abaixo) |
| `resumo` | string | uma frase para humanos |
| `estatisticas` | objeto | veja `estatisticas` |
| `achados` | lista | os achados; nunca é `null`, vira `[]` quando não há nenhum |

## `estatisticas`

| campo | tipo | significado |
|---|---|---|
| `total` | inteiro | total de trocas repetidas |
| `identicas` | inteiro | trocas com resposta igual nos dois lados |
| `diferentes` | inteiro | trocas com alguma diferença |
| `latencia.base` | percentis | latência da versão base |
| `latencia.candidata` | percentis | latência da versão candidata |
| `latencia.delta` | percentis | candidata menos base (positivo = candidata mais lenta) |

Cada objeto de percentis tem `p50_ms` (mediana) e `p95_ms` (percentil 95), em
milissegundos com casas decimais.

## `achados[]`

| campo | tipo | significado |
|---|---|---|
| `tipo` | string | `erro_novo`, `contrato_quebrado`, `latencia` ou `diferenca` |
| `origem` | string | `branch` ou `pre_existente` |
| `endpoint` | string | método e rota normalizada, ex.: `GET /orders/{id}` |
| `reproducao` | string | um `curl` pronto para rodar |
| `base` | lado | o que a versão base respondeu |
| `candidata` | lado | o que a versão candidata respondeu |
| `diff` | lista, opcional | caminhos JSON que mudaram (veja `diff[]`) |
| `local` | string, opcional | `arquivo:linha`, quando a resposta trouxe stack trace |

### Valores de `tipo`

| valor | significado |
|---|---|
| `erro_novo` | 5xx, timeout ou conexão recusada na candidata |
| `contrato_quebrado` | campo removido ou tipo trocado na resposta |
| `latencia` | p95 do endpoint acima do limite |
| `diferenca` | a resposta mudou, mas não dá para afirmar que é bug |

### Valores de `origem`

| valor | significado |
|---|---|
| `branch` | só a candidata falha: o problema veio da branch |
| `pre_existente` | a base também falha: o problema já existia |

### Lado (`base` e `candidata`)

| campo | tipo | significado |
|---|---|---|
| `status` | inteiro | status HTTP; `0` quando não houve resposta |
| `corpo` | string | corpo da resposta, resumido |
| `latencia_ms` | número | latência da requisição em ms, com casas decimais |
| `trace_id` | string | trace id (32 hex) do `traceparent` que o Parallax enviou; diferente em cada lado |
| `erro` | string, opcional | por que não houve resposta: `timeout`, `conexao_recusada`... |

### `diff[]`

| campo | tipo | significado |
|---|---|---|
| `caminho` | string | caminho JSON, ex.: `$.items[0].price` |
| `antes` | qualquer JSON, opcional | valor na base; ausente = o campo não existia na base |
| `depois` | qualquer JSON, opcional | valor na candidata; ausente = o campo foi removido |

`antes` e `depois` guardam o valor exato (número, string, objeto, `null`).
Campo ausente e `null` são coisas diferentes.

## Regra do resultado

Implementada em `veredito.CalcularResultado(achados)`:

1. `regressao`: existe algum achado de tipo `erro_novo`, `contrato_quebrado` ou
   `latencia` com origem `branch`.
2. `revisar`: há achados, mas só de `diferenca` ou de origem `pre_existente`.
3. `aprovado`: nenhum achado.

`Veredito.Validar()` confere a versão, os valores de `resultado`, `tipo` e
`origem`, e se o `resultado` bate com essa regra.

## Códigos de saída da CLI

Mapeados por `veredito.CodigoSaida(resultado)` e pelas constantes `Saida*`:

| código | constante | quando |
|---|---|---|
| 0 | `SaidaAprovado` | resultado `aprovado` |
| 1 | `SaidaRegressao` | resultado `regressao` |
| 2 | `SaidaRevisar` | resultado `revisar` |
| 3 | `SaidaNaoImplementado` | subcomando ainda não implementado |
| 4 | `SaidaErro` | uso errado da CLI ou falha de execução |

## Exemplo completo

Este bloco é igual a `pkg/veredito/testdata/exemplo.json` (um teste garante).

```json
{
  "versao": 1,
  "id": "01JDB7Q8Z3X4M5N6P7R8S9T0VW",
  "resultado": "regressao",
  "resumo": "3 regressões da branch em 120 trocas repetidas: erro 500 em GET /orders/{id}, campo removido em GET /customers/{id} e p95 acima do limite em GET /search.",
  "estatisticas": {
    "total": 120,
    "identicas": 112,
    "diferentes": 8,
    "latencia": {
      "base": { "p50_ms": 12.4, "p95_ms": 48.9 },
      "candidata": { "p50_ms": 13.1, "p95_ms": 97.3 },
      "delta": { "p50_ms": 0.7, "p95_ms": 48.4 }
    }
  },
  "achados": [
    {
      "tipo": "erro_novo",
      "origem": "branch",
      "endpoint": "GET /orders/{id}",
      "reproducao": "curl -sS -X GET 'http://localhost:8080/orders/42?expand=items' -H 'Accept: application/json'",
      "base": {
        "status": 200,
        "corpo": "{\"id\":42,\"total\":10.5,\"items\":[...]}",
        "latencia_ms": 11.8,
        "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"
      },
      "candidata": {
        "status": 500,
        "corpo": "panic: runtime error: invalid memory address or nil pointer dereference",
        "latencia_ms": 3.2,
        "trace_id": "0af7651916cd43dd8448eb211c80319c"
      },
      "local": "internal/orders/handler.go:87"
    },
    {
      "tipo": "contrato_quebrado",
      "origem": "branch",
      "endpoint": "GET /customers/{id}",
      "reproducao": "curl -sS -X GET 'http://localhost:8080/customers/7' -H 'Accept: application/json'",
      "base": {
        "status": 200,
        "corpo": "{\"id\":7,\"name\":\"Ana\",\"email\":\"ana@example.com\",\"age\":31}",
        "latencia_ms": 9.6,
        "trace_id": "5c0a9e1f2b3d4c5e6f708192a3b4c5d6"
      },
      "candidata": {
        "status": 200,
        "corpo": "{\"id\":7,\"name\":\"Ana\",\"age\":\"31\"}",
        "latencia_ms": 10.1,
        "trace_id": "e1d2c3b4a5968778695a4b3c2d1e0f11"
      },
      "diff": [
        { "caminho": "$.email", "antes": "\"ana@example.com\"" },
        { "caminho": "$.age", "antes": 31, "depois": "31" }
      ]
    },
    {
      "tipo": "latencia",
      "origem": "branch",
      "endpoint": "GET /search",
      "reproducao": "curl -sS -X GET 'http://localhost:8080/search?q=caneta'",
      "base": {
        "status": 200,
        "corpo": "{\"results\":[...]}",
        "latencia_ms": 45.2,
        "trace_id": "a1b2c3d4e5f60718293a4b5c6d7e8f90"
      },
      "candidata": {
        "status": 0,
        "corpo": "",
        "latencia_ms": 5000,
        "trace_id": "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
        "erro": "timeout"
      }
    },
    {
      "tipo": "diferenca",
      "origem": "pre_existente",
      "endpoint": "GET /health",
      "reproducao": "curl -sS -X GET 'http://localhost:8080/health'",
      "base": {
        "status": 200,
        "corpo": "{\"status\":\"ok\",\"uptime\":3600}",
        "latencia_ms": 1.1,
        "trace_id": "11111111111111111111111111111111"
      },
      "candidata": {
        "status": 200,
        "corpo": "{\"status\":\"ok\",\"uptime\":12}",
        "latencia_ms": 1.3,
        "trace_id": "22222222222222222222222222222222"
      },
      "diff": [
        { "caminho": "$.uptime", "antes": 3600, "depois": 12 }
      ]
    }
  ]
}
```
