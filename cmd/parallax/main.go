// Comando parallax: CLI fina por cima de captura, repeticao e pkg/veredito.
//
// Uso: parallax <comando> [flags]. Veja "parallax help".
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/leonardolermen/parallax/pkg/veredito"
)

// versao é sobrescrita na compilação com -ldflags "-X main.versao=v0.1.0".
var versao = "dev"

const uso = `parallax: juiz de comportamento entre a versão base e a candidata de um serviço.

Uso:
  parallax <comando> [flags]

Comandos:
  record    grava o tráfego real como proxy reverso na frente de um serviço
  replay    repete uma captura contra base e candidata e produz achados JSON
  check     repete, compara e devolve o veredito com laudo; guarda no histórico
  history   lista as execuções guardadas; "history show ID" mostra uma delas
  version   mostra a versão
  help      mostra esta ajuda

Exemplo:
  parallax record --listen :9100 --upstream http://localhost:8080 --out captura.jsonl
  parallax replay --capture captura.jsonl --base http://localhost:8080 --candidate http://localhost:8081
  parallax check --capture captura.jsonl --base http://localhost:8080 --candidate http://localhost:8081
  parallax history

Flags de cada comando: parallax <comando> -h

Códigos de saída: 0 aprovado, 1 regressão, 2 revisar, 4 uso errado ou falha.
O replay sai com 0 mesmo com achados; quem decide o resultado é o check.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executa a CLI e devolve o código de saída.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, uso)
		return veredito.SaidaErro
	}
	switch args[0] {
	case "help", "-h", "--help", "-help":
		fmt.Fprint(stdout, uso)
		return veredito.SaidaAprovado
	case "version", "--version", "-version":
		fmt.Fprintf(stdout, "parallax %s\n", versao)
		return veredito.SaidaAprovado
	case "record":
		ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer parar()
		return gravar(ctx, args[1:], func(endereco string) {
			fmt.Fprintf(stderr, "gravando em %s (Ctrl+C para encerrar)\n", endereco)
		}, stderr)
	case "replay":
		ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer parar()
		return repetir(ctx, args[1:], stdout, stderr)
	case "check":
		ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer parar()
		return checar(ctx, args[1:], stdout, stderr)
	case "history":
		return historico(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "parallax: comando desconhecido %q\n\n%s", args[0], uso)
		return veredito.SaidaErro
	}
}
