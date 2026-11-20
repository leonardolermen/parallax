<p align="center">
  <img src="docs/assets/banner.svg" alt="Parallax: o juiz de comportamento das mudanças feitas por agentes de código" width="100%">
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.22+">
  <img src="https://img.shields.io/badge/depend%C3%AAncias-s%C3%B3%20a%20stdlib-8ab8f0" alt="Só a biblioteca padrão">
  <img src="https://img.shields.io/badge/IA-nenhuma%20chamada-c4b1f2" alt="Nenhuma chamada a modelo de IA">
</p>

# <img src="docs/assets/icon.svg" alt="" width="36" align="top"> Parallax

O Parallax é o juiz de comportamento das mudanças feitas por agentes de código.
Ele não sobe ambientes: recebe duas versões de um serviço que já estão rodando,
a **base** (a versão atual) e a **candidata** (a branch do agente), repete
contra as duas um tráfego real gravado, compara as respostas e devolve um
veredito estruturado: o que está errado, onde e como reproduzir.

<img src="docs/assets/solaris-icon.svg" alt="" width="20" align="top"> Quem chama o Parallax é o [Solaris](https://github.com/leonardolermen/solaris), um orquestrador de agentes, que decide o que fazer com o veredito.

Go 1.22 ou mais novo, só a biblioteca padrão. Nenhuma chamada a modelo de IA.

## Estado atual

As três partes estão prontas:

| comando | estado |
|---|---|
| `parallax record` | pronto: grava o tráfego real como proxy reverso |
| `parallax replay` | pronto: repete a captura contra base e candidata e produz achados JSON |
| `parallax check` | pronto: repete, compara, devolve o veredito com laudo e guarda no histórico |
| `parallax history` | pronto: lista as execuções guardadas e mostra uma delas |
| `parallax version`, `parallax help` | prontos |

O contrato do veredito (`veredito.json`) está documentado em
[docs/contrato.md](docs/contrato.md). Quem calcula o veredito é o `check`; o
`replay` só produz os achados.

## Estrutura

Mesmo padrão do Solaris: cada pasta de `internal/` é um contexto, com as
camadas dentro dela (`repository_*.go` para leitura e escrita em disco,
`service_*.go` para a lógica, testes `*_test.go` ao lado do arquivo que testam).
Arquivos de um sistema só terminam em `_windows.go` ou `_other.go` (este com
`//go:build !windows`). O que está em `internal/` não pode ser importado de
outro módulo; o que o Solaris importa fica em `pkg/`.

```
cmd/parallax/           CLI: lê as flags e liga as peças; não tem regra própria
internal/
  captura/              gravação do tráfego real
    repository_model.go   aliases para o formato em pkg/captura
    service_gravador.go   o gravador: proxy reverso (http.Handler) que grava cada troca
    service_resposta.go   cópia limitada dos corpos e ResponseWriter que grava a resposta
    service_mascara.go    máscara dos cabeçalhos sensíveis (***), a de pkg/captura
  repeticao/            validação integral, replay concorrente e comparação de respostas
  julgamento/           veredito, laudo e histórico
    service_veredito.go   monta o veredito.json, o resumo e o motivo
    service_laudo.go      laudo em Markdown
    repository_historico.go  histórico de execuções em .parallax/runs/<id>/
  platform/             infraestrutura comum
    relogio_windows.go    relógio de alta resolução para latência (QueryPerformanceCounter)
    relogio_other.go      o mesmo fora do Windows (time.Now)
    service_id.go         id da execução (ULID)
pkg/
  captura/              formato da captura: Troca, escrita e leitura em JSONL e máscara;
                        só a biblioteca padrão. Para quem grava tráfego fora do Parallax.
  veredito/             contrato do veredito, regra do resultado e código de saída;
                        só a biblioteca padrão. É o pacote que o Solaris importa.
docs/
  contrato.md           o veredito.json campo a campo
  captura.md            o formato da captura (JSONL) e o que não é gravado
.githooks/pre-push      compila para Linux e macOS antes do push
```

## Compilar e testar

```sh
go build -o parallax ./cmd/parallax     # no Windows: go build -o parallax.exe ./cmd/parallax
go test -timeout 120s ./...
```

Para gravar a versão no binário:

```sh
go build -ldflags "-X main.versao=v0.1.0" -o parallax ./cmd/parallax
./parallax version    # parallax v0.1.0
```

Antes de cada `git push`, o hook `.githooks/pre-push` compila o projeto (com os
testes) para Linux e macOS, porque os arquivos `*_windows.go` só entram no build
do Windows. Para ligar num clone novo: `git config core.hooksPath .githooks`
(pular uma vez: `git push --no-verify`).

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

O formato do arquivo (campo a campo, com uma linha real), o que não é gravado e
a segurança na repetição estão em [docs/captura.md](docs/captura.md).

## Repetir e comparar: `parallax replay`

Com a base na porta 8080, a candidata na 8081 e uma captura feita pelo
`record` (os dois serviços precisam estar rodando):

```sh
./parallax replay --capture captura.jsonl --base http://localhost:8080 --candidate http://localhost:8081 --out achados.json
```

No Windows, use `./parallax.exe`. Sem `--out`, o JSON vai para a saída padrão.
O arquivo de saída é sobrescrito se existir, mas não pode ser a própria captura.

### Flags do replay

| flag | padrão | significado |
|---|---|---|
| `--capture` | (obrigatória) | captura JSONL produzida pelo `record` |
| `--base` | (obrigatória) | URL absoluta HTTP(S) da versão base |
| `--candidate` | (obrigatória) | URL absoluta HTTP(S) da versão candidata |
| `--out` | saída padrão | arquivo JSON com estatísticas e achados |
| `--workers` | `4` | máximo de trabalhadores; cada um envia base e depois candidata |
| `--timeout` | `10s` | timeout por requisição, incluindo a leitura da resposta |
| `--latency-pct` | `50` | aumento percentual do p95 que precisa ser ultrapassado |
| `--latency-min-ms` | `20` | aumento mínimo do p95, em ms, junto com o percentual |
| `--latency-max-ms` | `0` | teto absoluto do p95 da candidata, em ms; `0` desativa |
| `--allow-unsafe` | `false` | permite métodos além de GET, HEAD e OPTIONS |
| `--header` | nenhum | `Nome: valor` para ambos os destinos, substitui o cabeçalho capturado; repetível |
| `--max-body` | `1048576` (1 MiB) | limite de bytes lidos por resposta, inclusive após descompactar gzip |

Workers, timeout, max-body, percentual e mínimo devem ser positivos; o teto
aceita zero. Não há leitura de `parallax.yaml` nesta etapa.

### Execução e segurança

A configuração e a captura inteira são validadas antes de qualquer envio:
uma linha inválida ou captura vazia aborta sem tráfego. Por padrão só GET,
HEAD e OPTIONS são repetidos; requisições com corpo truncado são sempre
puladas. O corpo da resposta gravada não é a referência: o replay compara
as respostas novas da base e da candidata.

O caminho das URLs de destino é prefixado ao caminho capturado; a query vem
da captura. Não coloque query em `--base` ou `--candidate`: ela é descartada.
Redirecionamentos não são seguidos. Cada requisição recebe um `traceparent`
novo, independente entre trocas e entre base e candidata. Os clientes têm
timeout e respeitam cancelamento (Ctrl+C).

Cabeçalhos mascarados como `***` não são enviados. Host, Content-Length,
cabeçalhos de conexão e traces anteriores são removidos/recalculados.
Para autenticar, use, por exemplo, `--header 'Authorization: Bearer TOKEN_DE_TESTE'`.
**Os valores de `--header` podem aparecer em texto puro no `curl` de reprodução
dos achados**, inclusive na saída padrão. Proteja o JSON e use credenciais de teste.
Veja também [segurança da captura](docs/captura.md#segurança-na-repetição).

### Regras de comparação

- Status diferentes geram `diferenca`. Com status igual, são comparados
  Content-Type, Location e Cache-Control, além do corpo.
- Corpos JSON ignoram a ordem das chaves; arrays mantêm a ordem. Campo removido
  ou tipo alterado (inclusive `null`) gera `contrato_quebrado`; outros valores
  alterados geram `diferenca`, com até 50 entradas de diff. `1` e `1.0` contam
  como diferentes. Corpos que não sejam JSON válido dos dois lados são comparados
  byte a byte. Gzip é descompactado; br e deflate ainda são comparados comprimidos.
- 5xx, timeout ou conexão recusada na candidata geram `erro_novo`, com origem
  `pre_existente` se a base também tem uma dessas falhas; senão, `branch`.
  Outras falhas de rede, como DNS ou conexão interrompida só na candidata,
  geram `diferenca`.
- Resposta acima de `--max-body` gera `diferenca` por comparação incompleta
  e a execução continua; igualdade dos prefixos não significa resposta idêntica.
  A classificação `erro_novo` tem prioridade sobre comparação incompleta.

O p95 é comparado por método e rota normalizada: segmentos numéricos, UUIDs,
ULIDs e hexadecimais de 16 ou mais caracteres viram `{id}` (ex.: `GET /orders/{id}`).
A query não separa endpoints. Só entram nos percentis pares completos e sem
erro de transporte; o cálculo usa nearest-rank (posição `ceil(p/100 × n)`
nas amostras ordenadas).

Há achado `latencia` quando o p95 da candidata é **mais de 50% maior E pelo
menos 20 ms acima** da base (limites configuráveis). Essa regra é avaliada
primeiro e gera origem `branch`, mesmo que a base já passe do teto absoluto:
assim, ligar o teto não esconde uma regressão relativa. Senão, ultrapassar
`--latency-max-ms` também gera achado: `pre_existente` se a base já excedia
o teto, ou `branch` caso contrário. Com teto 50 ms, 60→100 ms é `branch` e
60→79 ms é `pre_existente`.

### Saída

O JSON tem `id`, `estatisticas` e `achados` (lista vazia quando não há nenhum).
As estatísticas incluem `total` repetido, `identicas`, `diferentes`, `puladas`,
`incompletas` (trocas comparadas sem a resposta inteira de um dos lados, por
`--max-body` ou leitura interrompida, sem falha na candidata) e p50/p95 globais de base, candidata e delta. Achados de latência são agregados
à parte e não aumentam `diferentes`.

Os achados por troca seguem a ordem original da captura, embora os envios
possam intercalar trabalhadores. Os de latência vêm ao final, ordenados por
endpoint. Cada achado traz tipo, origem, endpoint, `curl` da candidata e dados
dos dois lados (status, corpo resumido a 500 caracteres, latência, trace ID e
eventual erro), além de diff quando aplicável. Os tipos dos achados estão em
[docs/contrato.md](docs/contrato.md#achados), mas a saída do replay é própria:
não contém `versao`, `resultado` ou `resumo`, nem grava histórico. Para o
veredito, use o `check`.

## Veredito e laudo: `parallax check`

O `check` faz o mesmo que o `replay` e depois julga: calcula o veredito do
[contrato](docs/contrato.md), imprime um laudo legível e guarda a execução no
histórico. É o comando para usar como portão no CI.

```sh
./parallax check --capture captura.jsonl --base http://localhost:8080 --candidate http://localhost:8081
```

O resultado segue a regra de `pkg/veredito` (`CalcularResultado`):

| resultado | quando | código |
|---|---|---|
| `aprovado` | nenhum achado | 0 |
| `regressao` (reprovado) | algum `erro_novo`, `contrato_quebrado` ou `latencia` com origem `branch` | 1 |
| `revisar` (inconclusivo) | só diferenças ou achados pré-existentes, inclusive comparações incompletas por `--max-body` | 2 |

Se nenhuma troca foi comparada (por exemplo, todas eram POST sem
`--allow-unsafe`), o `check` não aprova sem evidência: sai com 4 e não grava
nada no histórico.

### Flags do check

Todas as flags do replay, menos `--out`, e mais:

| flag | padrão | significado |
|---|---|---|
| `--json` | `false` | imprime o `veredito.json` no lugar do laudo |
| `--report` | nenhum | grava também o laudo Markdown neste arquivo (sobrescreve; não pode ser a captura) |
| `--runs-dir` | `.parallax/runs` | pasta do histórico |

O laudo (ou o JSON) vai para a saída padrão; o resultado e onde a execução foi
gravada vão para a saída de erro.

### O laudo

Markdown, nesta ordem:

- **cabeçalho**: id da execução, data (UTC), captura, base e candidata;
- **Veredito**: resultado em maiúsculas, resumo em uma frase e o motivo (a regra que decidiu);
- **Estatísticas**: total, idênticas, diferentes, puladas, incompletas e p50/p95 de base, candidata e delta;
- **Regressões de latência**: por endpoint, p95 antes e depois, variação em % e origem;
- **Diferenças por endpoint**: até 10 achados por endpoint (o resto fica no `veredito.json`), cada um com tipo, origem, status base → candidata, até 10 mudanças (`caminho: antes → depois`) e o `curl` de reprodução;
- **Comparações incompletas**: endpoints em que a base ou a candidata não foi lida inteira.

Seções sem itens mostram "Nenhuma.".

Os nomes de campo nas mudanças saem como vieram na resposta, sem escape. Um
nome com crase ou quebra de linha bagunça o Markdown, e um com caractere de
controle (um `ESC`, por exemplo) chega ao terminal quando o laudo sai na saída
padrão. O risco é baixo, porque a resposta vem do serviço que você está
testando; o `veredito.json` não tem esse problema.

## Histórico: `parallax history`

Cada `check` grava uma pasta `<runs-dir>/<id>/` com:

| arquivo | conteúdo |
|---|---|
| `veredito.json` | o veredito do [contrato](docs/contrato.md) |
| `laudo.md` | o laudo |
| `execucao.json` | id, data, captura (caminho absoluto), base, candidata, resultado, resumo e estatísticas |

```sh
./parallax history                          # lista, da mais recente para a mais antiga
./parallax history show 01JD3...            # mostra o laudo de uma execução
./parallax history show 01JD3... --json     # mostra o veredito.json
```

`history` e `history show` aceitam `--runs-dir` (padrão `.parallax/runs`).
Uma entrada ilegível aparece como aviso na saída de erro e não impede a
listagem. `history show` com id inexistente ou inválido sai com 4.

A pasta de cada execução tem permissão 0700 e os arquivos 0600, e cada arquivo
é gravado de forma atômica. No Windows essas permissões não valem: os arquivos
herdam as permissões (ACL) da pasta em que estão.

**O `curl` de reprodução contém os valores de `--header` (um token, por
exemplo) e a URL da candidata como foi passada.** Ele aparece no laudo, na
saída padrão (e portanto no log do CI), no `--report`, no `veredito.json` e no
histórico. Use credenciais de teste e não commite `.parallax/`, que já está no
`.gitignore`. Senhas em URLs (`http://user:senha@host`) são escondidas no
`execucao.json`, na listagem e no cabeçalho do laudo, mas não no `curl`; tokens
na query string das URLs não são escondidos.

Se o histórico não puder ser gravado (disco cheio, pasta sem permissão), o
laudo aparece mesmo assim, mas o `check` sai com 4: a execução não some sem
ninguém ver.

## Códigos de saída

**O `check` sai com o código do veredito: 0 aprovado, 1 regressão, 2 revisar.**
Sai com 4 em uso errado, captura inválida, falha de execução, nenhuma troca
comparada ou falha ao gravar o histórico.

**O `replay` retorna 0 ao concluir, mesmo com achados; 4 em caso de uso errado,
captura inválida ou falha de execução.** Erros HTTP comparados são achados e
não mudam o código de saída do replay.

| código | quando |
|---|---|
| 0 | aprovado (ou comando concluído sem erro) |
| 1 | regressão (só o `check`) |
| 2 | revisar (só o `check`) |
| 4 | uso errado ou falha |

O código 3 (`SaidaNaoImplementado`) continua no contrato, mas nenhum comando o
usa mais.

O significado de cada campo do veredito e a regra do resultado estão em
[docs/contrato.md](docs/contrato.md).

## Decisões

- **O `replay` continua saindo com 0 e o portão do CI é o `check`.** Fazer o
  `replay` sair com 1 ou 2 quebraria o comportamento documentado na parte 2;
  quem já usa o 0 do `replay` não é afetado.
- **"Reprovado" e "inconclusivo" são `regressao` e `revisar`.** O contrato e o
  `pkg/veredito` (que o Solaris importa) não mudaram: um estado novo quebraria
  o `Validar` e as integrações sem ganho, e `revisar` já cobre as comparações
  incompletas.
- **Nenhuma troca comparada dá 4.** O `check` não aprova sem evidência.
- **Histórico em arquivos JSON, sem banco.** O projeto usa só a biblioteca
  padrão do Go, então nada de SQLite ou outra dependência. Cada execução fica
  numa pasta própria, no formato que o contrato já previa
  (`.parallax/runs/<id>/`).
