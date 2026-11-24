package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/leonardolermen/parallax/internal/captura"
	"github.com/leonardolermen/parallax/internal/exploracao"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

type pastas []string

func (p *pastas) String() string     { return strings.Join(*p, ",") }
func (p *pastas) Set(v string) error { *p = append(*p, v); return nil }

// explorar implementa "parallax explore": monta uma captura sem tráfego
// gravado, descobrindo as rotas GET do serviço.
func explorar(ctx context.Context, args []string, stderr io.Writer) int {
	cfg := exploracao.ConfigPadrao()
	fs := flag.NewFlagSet("explore", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, "Uso: parallax explore --target URL --out ARQUIVO [--source PASTA] [flags]\n\n"+
			"Monta uma captura sem tráfego gravado: acha as rotas GET no código (--source)\n"+
			"e nas respostas do serviço (links, chamadas do JavaScript, JSON, OpenAPI),\n"+
			"preenche os parâmetros com valores vistos nas respostas e grava o que o\n"+
			"serviço respondeu. Rode contra a base; depois use a captura no check.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	fs.StringVar(&cfg.Alvo, "target", "", "URL do serviço rodando, de preferência a base (obrigatório)")
	saida := fs.String("out", "", "arquivo JSONL da captura (obrigatório; sobrescrito se existir)")
	var fontes pastas
	fs.Var(&fontes, "source", "pasta com o código do serviço; repetível")
	fs.IntVar(&cfg.Max, "max", cfg.Max, "máximo de trocas gravadas")
	fs.IntVar(&cfg.PorRota, "per-route", cfg.PorRota, "máximo de trocas por rota (ex.: /pedidos/{id})")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "timeout por requisição")
	fs.Int64Var(&cfg.MaxCorpo, "max-body", cfg.MaxCorpo, "bytes gravados de cada resposta")
	excluir := fs.String("exclude", exploracao.ExcluirPadrao, "regex dos caminhos que não são visitados")
	fs.BoolVar(&cfg.Estaticos, "assets", false, "gravar também CSS, imagens, fontes e JavaScript")
	headers := &cabecalhosReplay{valores: make(http.Header)}
	fs.Var(headers, "header", "cabeçalho Nome: valor em cada requisição; repetível (gravado mascarado se for credencial)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return veredito.SaidaAprovado
		}
		return veredito.SaidaErro
	}
	falhar := func(formato string, a ...any) int {
		fmt.Fprintf(stderr, "parallax explore: "+formato+"\n", a...)
		return veredito.SaidaErro
	}
	if fs.NArg() > 0 {
		return falhar("argumento inesperado %q", fs.Arg(0))
	}
	if cfg.Alvo == "" {
		return falhar("falta --target")
	}
	if *saida == "" {
		return falhar("falta --out")
	}
	if *excluir != "" {
		re, err := regexp.Compile(*excluir)
		if err != nil {
			return falhar("--exclude inválido: %v", err)
		}
		cfg.Excluir = re
	} else {
		cfg.Excluir = nil
	}
	for _, f := range fontes {
		if st, err := os.Stat(f); err != nil || !st.IsDir() {
			return falhar("--source %s não é uma pasta", f)
		}
	}
	cfg.Fontes = fontes
	cfg.Cabecalhos = headers.valores

	trocas, resumo, err := exploracao.Explorar(ctx, cfg)
	if err != nil {
		return falhar("%v", err)
	}
	if len(trocas) == 0 {
		return falhar("nenhuma rota respondeu em %s (%d visitadas)", cfg.Alvo, resumo.Visitadas)
	}
	arquivo, err := os.Create(*saida)
	if err != nil {
		return falhar("não consegui criar %s: %v", *saida, err)
	}
	esc := captura.NovoEscritor(arquivo)
	for _, t := range trocas {
		if err := esc.Escrever(t); err != nil {
			arquivo.Close()
			return falhar("%v", err)
		}
	}
	if err := errors.Join(esc.Fechar(), arquivo.Close()); err != nil {
		return falhar("gravar %s: %v", *saida, err)
	}
	fmt.Fprintf(stderr, "%d trocas gravadas em %s (%d visitadas, %d rotas no código, %d modelos com parâmetro)\n",
		resumo.Gravadas, *saida, resumo.Visitadas, resumo.RotasCodigo, resumo.Modelos)
	if resumo.ComVolateis > 0 {
		fmt.Fprintf(stderr, "%d trocas têm campos que mudam sozinhos (horário, contador); o check não os compara\n", resumo.ComVolateis)
	}
	if len(resumo.SemValor) > 0 {
		fmt.Fprintf(stderr, "sem valor para os parâmetros, ficaram de fora: %s\n", strings.Join(resumo.SemValor, ", "))
	}
	return veredito.SaidaAprovado
}
