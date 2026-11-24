# Captura (`parallax record`)

O formato do arquivo que o `parallax record` grava e o que fica de fora. O gravador
está em `internal/captura`; o formato, em `pkg/captura`. Como usar o comando: [README](../README.md#gravar-tráfego-parallax-record).

## Formato

Um arquivo JSONL: uma troca por linha, em UTF-8, terminada em `\n`. O tipo em Go
é `captura.Troca`, do pacote público `pkg/captura`, que também tem a escrita
(`captura.NovoEscritor`), a leitura em streaming (`captura.NovoLeitor`) e a
máscara dos cabeçalhos sensíveis (`captura.Mascarar`). Só depende da biblioteca
padrão. Quem grava tráfego fora do Parallax (o gateway do Solaris, por exemplo)
importa esse pacote para produzir uma captura que o `replay` e o `check` aceitam:

```go
import "github.com/leonardolermen/parallax/pkg/captura"

e := captura.NovoEscritor(arquivo)
_ = e.Escrever(captura.Troca{ /* ... */ CabecalhosRequisicao: captura.Mascarar(h) })
_ = e.Fechar()
```

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
| `campos_volateis` | lista de strings, opcional | campos da resposta que mudam sozinhos entre duas chamadas à mesma versão, em caminhos JSON (`$.geradoEm`, `$.itens[0].visto`; `$` é o corpo inteiro). O replay não os compara. Só o `parallax explore` preenche; ausente quando vazio |

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

A gravação registra todos os métodos. O replay só repete
métodos seguros (`GET`, `HEAD`, `OPTIONS`) por padrão: repetir um `POST` pode
cobrar, enviar webhook ou mandar e-mail. Outros métodos só com `--allow-unsafe`
(não há leitura de `parallax.yaml` nesta etapa). Requisições com corpo truncado
são sempre puladas; uma resposta gravada truncada não impede o replay, pois
as respostas comparadas são as novas da base e da candidata.

A captura inteira e a configuração são validadas antes de qualquer envio;
uma linha inválida aborta sem enviar tráfego. Cabeçalhos mascarados como `***`
não são reenviados. `--header 'Nome: valor'` injeta o valor nos dois destinos,
mas ele pode aparecer em texto puro no `curl` dos achados JSON. Use credenciais
de teste e proteja a captura e os resultados: a máscara cobre apenas os quatro
cabeçalhos listados acima, não corpos, query strings ou outros cabeçalhos.

Como rodar, flags e regras de comparação:
[replay no README](../README.md#repetir-e-comparar-parallax-replay).

