# Captura (`parallax record`)

O formato do arquivo que o `parallax record` grava e o que fica de fora. O código
está em `internal/captura`. Como usar o comando: [README](../README.md#gravar-tráfego-parallax-record).

## Formato

Um arquivo JSONL: uma troca por linha, em UTF-8, terminada em `\n`. Dentro do
Parallax, o tipo em Go é `captura.Troca` e a leitura em streaming é
`captura.NovoLeitor`. O pacote fica em `internal/` e não pode ser importado de
outro módulo: quem lê a captura de fora (o Solaris, por exemplo) decodifica as
linhas pelos campos abaixo.

| campo | tipo | significado |
|---|---|---|
| `horario` | string | início da requisição, RFC3339Nano em UTC |
| `metodo` | string | método HTTP (todos são gravados) |
| `caminho` | string | caminho da URL, sem a query |
| `query` | string | query crua, sem o `?`; `""` se não houver |
| `cabecalhos_requisicao` | objeto (nome → lista de valores) | cabeçalhos da requisição, incluindo `Host`, com os sensíveis mascarados |
| `corpo_requisicao` | string base64 | corpo da requisição (`""` se vazio) |
| `corpo_requisicao_truncado` | booleano, opcional | `true` se o corpo passou do `--max-body`; ausente quando `false` |
| `status` | número | status HTTP devolvido ao cliente |
| `cabecalhos_resposta` | objeto | cabeçalhos da resposta, com os sensíveis mascarados |
| `corpo_resposta` | string base64 | corpo da resposta como chegou ao cliente (`""` se vazio; comprimido se veio comprimido) |
| `corpo_resposta_truncado` | booleano, opcional | idem para a resposta |
| `latencia_ms` | número | do início da requisição até o fim da resposta, em ms com casas decimais |

Os valores de `Authorization`, `Cookie`, `Set-Cookie` e `X-Api-Key` (sem
diferenciar maiúsculas) são gravados como `***`, na requisição e na resposta.
O cliente e o serviço continuam vendo os valores originais.

Uma linha real, gravada pelo código do `parallax record` (a mesma função da
CLI, chamada num teste) com a mesma requisição do exemplo
(`GET /status.json?q=1` com `Authorization: Bearer x`). O serviço era o
`http.FileServer` do Go servindo o mesmo `status.json`, o cliente era o do Go e
o proxy escutava numa porta aleatória. Por isso `Host`, `User-Agent`,
`Accept`/`Accept-Encoding`, `Accept-Ranges` e `Server` diferem do que o curl e o
`python -m http.server` produzem. Aqui ela foi quebrada em linhas para leitura;
no arquivo é uma linha só:

```json
{"horario":"2026-11-18T04:40:32.8351407Z","metodo":"GET","caminho":"/status.json","query":"q=1",
 "cabecalhos_requisicao":{"Accept-Encoding":["gzip"],"Authorization":["***"],"Host":["127.0.0.1:59273"],"User-Agent":["Go-http-client/1.1"]},
 "corpo_requisicao":"","status":200,
 "cabecalhos_resposta":{"Accept-Ranges":["bytes"],"Content-Length":["12"],"Content-Type":["application/json"],"Date":["Wed, 18 Nov 2026 04:40:32 GMT"],"Last-Modified":["Wed, 18 Nov 2026 04:40:32 GMT"]},
 "corpo_resposta":"eyJvayI6dHJ1ZX0K","latencia_ms":39.4514}
```

O `Authorization` foi gravado como `***`, e `corpo_resposta` é o base64 de
`{"ok":true}\n`.

Para ver os corpos, decodifique o base64. Com `jq`:

```sh
jq -r '.corpo_resposta | @base64d' captura.jsonl
# {"ok":true}
```

### O que não é gravado

- As falhas do próprio proxy (ex.: `502` quando o serviço está fora do ar).
- Trocas interrompidas no meio (o serviço derrubou a conexão) e requisições em
  que o cliente desconectou antes do fim. A captura só guarda trocas completas.
- Conexões que viram WebSocket (upgrade).
- O que passar depois do `--max`, ou o que o `--sample` não sorteou.
- No Ctrl+C, o que ainda estiver aberto depois de 5 s (ex.: um SSE sem fim).

### Segurança na repetição

A gravação registra todos os métodos. A repetição (parte 2) só vai repetir
métodos seguros (`GET`, `HEAD`, `OPTIONS`) por padrão: repetir um `POST` pode
cobrar, enviar webhook ou mandar e-mail. Outros métodos só com `--allow-unsafe`
ou com a rota marcada como idempotente no `parallax.yaml`.

