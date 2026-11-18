package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/leonardolermen/parallax/pkg/motor"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

// prazoEncerramento é quanto o record espera as requisições abertas no
// Ctrl+C antes de fechar o arquivo. O que ainda estiver aberto é descartado.
const prazoEncerramento = 5 * time.Second

// gravar implementa "parallax record". Roda até ctx ser cancelado (Ctrl+C) e
// devolve o código de saída. pronto, se não for nil, recebe o endereço em que
// o proxy está ouvindo.
func gravar(ctx context.Context, args []string, pronto func(endereco string), stderr io.Writer) int {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, "Uso: parallax record --upstream URL --out ARQUIVO [flags]\n\n"+
			"Proxy reverso que repassa ao upstream e grava cada troca em JSONL.\n"+
			"Authorization, Cookie, Set-Cookie e X-Api-Key são gravados como ***.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	listen := fs.String("listen", ":9100", "endereço em que o proxy escuta")
	upstream := fs.String("upstream", "", "URL do serviço que recebe o tráfego (obrigatório), ex.: http://localhost:8080")
	saida := fs.String("out", "", "arquivo JSONL da captura (obrigatório; sobrescrito se existir)")
	amostra := fs.Float64("sample", 1, "fração das trocas gravadas, de 0 a 1")
	maximo := fs.Int("max", 0, "número máximo de trocas gravadas; 0 = sem limite (depois disso só repassa)")
	maxCorpo := fs.Int64("max-body", motor.MaxCorpoPadrao, "bytes gravados de cada corpo; acima disso o corpo é cortado e marcado como truncado")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return veredito.SaidaAprovado
		}
		return veredito.SaidaErro
	}

	falhar := func(formato string, a ...any) int {
		fmt.Fprintf(stderr, "parallax record: "+formato+"\n", a...)
		return veredito.SaidaErro
	}
	if fs.NArg() > 0 {
		return falhar("argumento inesperado %q", fs.Arg(0))
	}
	if *upstream == "" {
		return falhar("falta --upstream")
	}
	if *saida == "" {
		return falhar("falta --out")
	}
	up, err := url.Parse(*upstream)
	if err != nil {
		return falhar("--upstream inválido: %v", err)
	}
	if up.Scheme != "http" && up.Scheme != "https" {
		return falhar("--upstream precisa começar com http:// ou https://")
	}
	if up.Host == "" {
		return falhar("--upstream sem host")
	}
	if !(*amostra >= 0 && *amostra <= 1) {
		return falhar("--sample precisa estar entre 0 e 1")
	}
	if *maximo < 0 {
		return falhar("--max não pode ser negativo")
	}
	if *maxCorpo < 0 {
		return falhar("--max-body não pode ser negativo")
	}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return falhar("não consegui escutar em %s: %v", *listen, err)
	}
	arquivo, err := os.Create(*saida)
	if err != nil {
		ln.Close()
		return falhar("não consegui criar %s: %v", *saida, err)
	}
	g, err := motor.NovoGravador(motor.ConfigGravador{
		Upstream: up,
		Saida:    motor.NovoEscritor(arquivo),
		Amostra:  *amostra,
		Max:      *maximo,
		MaxCorpo: *maxCorpo,
	})
	if err != nil {
		ln.Close()
		arquivo.Close()
		return falhar("%v", err)
	}

	srv := &http.Server{Handler: g, ReadHeaderTimeout: 30 * time.Second}
	erroServidor := make(chan error, 1)
	go func() { erroServidor <- srv.Serve(ln) }()
	if pronto != nil {
		pronto(ln.Addr().String())
	}

	codigo := veredito.SaidaAprovado
	select {
	case <-ctx.Done():
	case err := <-erroServidor:
		fmt.Fprintf(stderr, "parallax record: servidor parou: %v\n", err)
		codigo = veredito.SaidaErro
	}

	// Espera as requisições abertas por até prazoEncerramento; o que ficar
	// aberto (ex.: SSE sem fim) é derrubado e não é gravado.
	ctxFim, cancelar := context.WithTimeout(context.Background(), prazoEncerramento)
	defer cancelar()
	if err := srv.Shutdown(ctxFim); err != nil {
		srv.Close()
	}
	errFechar := g.Fechar()
	errArquivo := arquivo.Close()

	fmt.Fprintf(stderr, "%d trocas gravadas em %s\n", g.Gravadas(), *saida)
	if err := errors.Join(g.Erro(), errFechar, errArquivo); err != nil {
		fmt.Fprintf(stderr, "parallax record: erro ao gravar a captura: %v\n", err)
		return veredito.SaidaErro
	}
	return codigo
}
