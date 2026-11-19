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
    repository_model.go   tipo Troca (uma linha da captura)
    repository_jsonl.go   leitura e escrita da captura em JSONL
    service_gravador.go   o gravador: proxy reverso (http.Handler) que grava cada troca
    service_resposta.go   cópia limitada dos corpos e ResponseWriter que grava a resposta
    service_mascara.go    máscara dos cabeçalhos sensíveis (***)
  platform/             infraestrutura comum
    relogio_windows.go    relógio de alta resolução para latência (QueryPerformanceCounter)
    relogio_other.go      o mesmo fora do Windows (time.Now)
    service_id.go         id da execução (ULID)
pkg/
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
go test ./...
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
