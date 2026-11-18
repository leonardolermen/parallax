# Parallax

O Parallax é o juiz de comportamento das mudanças feitas por agentes de código.
Ele não sobe ambientes: recebe duas versões de um serviço que já estão rodando,
a **base** (a versão atual) e a **candidata** (a branch do agente), repete
contra as duas um tráfego real gravado, compara as respostas e devolve um
veredito estruturado: o que está errado, onde e como reproduzir.

Quem chama o Parallax é o Solaris, um orquestrador de agentes, que decide o que fazer com o veredito.

Go 1.22 ou mais novo, só a biblioteca padrão. Nenhuma chamada a modelo de IA.

## Estado atual

Esta é a parte 1 de 3:

| comando | estado |
|---|---|
| `parallax record` | pronto: grava o tráfego real como proxy reverso |
| `parallax replay` | ainda não implementado (sai com código 3) |
| `parallax check` | ainda não implementado (sai com código 3) |
| `parallax version`, `parallax help` | prontos |

O contrato do veredito (`veredito.json`) já está definido e documentado em
[docs/contrato.md](docs/contrato.md).

## Estrutura

| pacote | papel |
|---|---|
| `pkg/veredito` | tipos do contrato, regra do resultado e código de saída. Só depende da biblioteca padrão; é o pacote que o Solaris importa. |
| `pkg/motor` | o motor como biblioteca: tipo `Troca`, leitura e escrita JSONL, id da execução (ULID) e o gravador (`http.Handler`). Não lê flags nem escreve no terminal. |
| `cmd/parallax` | CLI fina por cima do motor. |

## Compilar e testar

```sh
go build -o parallax ./cmd/parallax     # no Windows: go build -o parallax.exe ./cmd/parallax
go test ./...
```

Para gravar a versão no binário:

```sh
go build -ldflags "-X main.versao=v0.1.0" -o parallax ./cmd/parallax
./parallax version    # parallax v0.1.0
```

## Gravar tráfego: `parallax record`

O `record` é um proxy reverso: fica na frente do serviço, repassa cada
requisição e grava cada troca (requisição + resposta) numa linha do arquivo de
captura. O cliente recebe exatamente o que o serviço respondeu, inclusive em
streaming (SSE).

Exemplo com um serviço qualquer na porta 8080 (aqui, o servidor de arquivos do
Python, num terminal bash):

```sh
# terminal 1: o serviço
mkdir /tmp/servico && cd /tmp/servico
echo '{"ok":true}' > status.json
python -m http.server 8080

# terminal 2: o Parallax na frente dele
./parallax record --listen :9100 --upstream http://localhost:8080 --out captura.jsonl
# gravando em [::]:9100 (Ctrl+C para encerrar)

# terminal 3: o tráfego passa pelo Parallax
curl -H 'Authorization: Bearer x' 'http://localhost:9100/status.json?q=1'
# {"ok":true}
```

Ao apertar Ctrl+C no terminal 2, o Parallax espera até 5 s pelas requisições
abertas, grava o que falta e fecha o arquivo:

```
1 trocas gravadas em captura.jsonl
```

### Flags

| flag | padrão | significado |
|---|---|---|
| `--listen` | `:9100` | endereço em que o proxy escuta |
| `--upstream` | (obrigatória) | URL do serviço, `http://` ou `https://`. Um caminho base é prefixado: com `http://h/api`, `/orders` vai para `/api/orders`. |
| `--out` | (obrigatória) | arquivo JSONL da captura. É sobrescrito se já existir. |
| `--sample` | `1` | fração das trocas gravadas, de 0 a 1, sorteada por troca. Com `0` nada é gravado. |
| `--max` | `0` | máximo de trocas gravadas; `0` = sem limite. Ao chegar no máximo o proxy continua repassando, mas para de gravar. |
| `--max-body` | `1048576` (1 MiB) | bytes gravados de cada corpo. Acima disso o cliente e o serviço recebem tudo, mas a troca é gravada com o corpo cortado e marcada como truncada. |

O upstream recebe o `Host` da URL do `--upstream` (funciona com ingress e TLS)
e o host original em `X-Forwarded-Host`. A captura guarda o `Host` que o
cliente mandou.

## Formato da captura

Um arquivo JSONL: uma troca por linha, em UTF-8, terminada em `\n`. O tipo em Go
é `motor.Troca`; para ler em streaming, use `motor.NovoLeitor`.

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

A troca do exemplo acima fica assim (quebrada em linhas aqui para leitura; no
arquivo é uma linha só):

```json
{"horario":"2026-11-18T14:02:31.418265Z","metodo":"GET","caminho":"/status.json","query":"q=1",
 "cabecalhos_requisicao":{"Accept":["*/*"],"Authorization":["***"],"Host":["localhost:9100"],"User-Agent":["curl/8.9.1"]},
 "corpo_requisicao":"","status":200,
 "cabecalhos_resposta":{"Content-Length":["12"],"Content-Type":["application/json"],"Date":["Wed, 18 Nov 2026 14:02:31 GMT"],"Last-Modified":["Wed, 18 Nov 2026 14:01:02 GMT"],"Server":["SimpleHTTP/0.6 Python/3.12.7"]},
 "corpo_resposta":"eyJvayI6dHJ1ZX0K","latencia_ms":3.0215}
```

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

## Códigos de saída

| código | quando |
|---|---|
| 0 | aprovado (ou comando concluído sem erro) |
| 1 | regressão |
| 2 | revisar |
| 3 | comando ainda não implementado |
| 4 | uso errado ou falha |

O significado de cada campo do veredito e a regra do resultado estão em
[docs/contrato.md](docs/contrato.md).
