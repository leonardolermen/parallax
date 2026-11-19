package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/leonardolermen/parallax/internal/repeticao"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

type cabecalhosReplay struct{ valores http.Header }

func (h *cabecalhosReplay) String() string { return "" }

func (h *cabecalhosReplay) Set(valor string) error {
	nome, conteudo, ok := strings.Cut(valor, ":")
	nome = strings.Trim(nome, " \t")
	if !ok || nome == "" {
		return fmt.Errorf("cabeçalho deve ter formato Nome: valor")
	}
	h.valores.Add(nome, strings.Trim(conteudo, " \t"))
	return nil
}

// repetir implementa replay sem calcular veredito ou gravar histórico.
func repetir(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg := repeticao.ConfigPadrao()
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Uso: parallax replay --capture ARQUIVO --base URL --candidate URL [flags]\n\nRepete e compara; produz achados JSON sem veredito final.\n\nFlags:")
		fs.PrintDefaults()
	}
	capture := fs.String("capture", "", "captura JSONL (obrigatória)")
	fs.StringVar(&cfg.Base, "base", "", "URL HTTP(S) da base (obrigatória)")
	fs.StringVar(&cfg.Candidata, "candidate", "", "URL HTTP(S) da candidata (obrigatória)")
	out := fs.String("out", "", "arquivo JSON de saída; vazio = stdout")
	fs.IntVar(&cfg.Trabalhadores, "workers", cfg.Trabalhadores, "máximo de trabalhadores")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "timeout por requisição (ex.: 10s)")
	fs.Float64Var(&cfg.PercentualLatencia, "latency-pct", cfg.PercentualLatencia, "aumento percentual do p95")
	fs.Float64Var(&cfg.MinimoLatenciaMs, "latency-min-ms", cfg.MinimoLatenciaMs, "aumento mínimo do p95 em ms")
	fs.Float64Var(&cfg.LimiteAbsolutoMs, "latency-max-ms", cfg.LimiteAbsolutoMs, "teto absoluto do p95 em ms; 0 desativa")
	fs.BoolVar(&cfg.PermitirInseguros, "allow-unsafe", false, "repetir também métodos inseguros")
	fs.Int64Var(&cfg.MaxCorpo, "max-body", cfg.MaxCorpo, "máximo de bytes por resposta; excesso gera comparação incompleta")
	headers := &cabecalhosReplay{valores: make(http.Header)}
	fs.Var(headers, "header", "cabeçalho Nome: valor para ambos os destinos; repetível")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return veredito.SaidaAprovado
		}
		return veredito.SaidaErro
	}
	falhar := func(err error) int {
		fmt.Fprintf(stderr, "parallax replay: %v\n", err)
		return veredito.SaidaErro
	}
	if fs.NArg() > 0 {
		return falhar(fmt.Errorf("argumento inesperado %q", fs.Arg(0)))
	}
	if *capture == "" {
		return falhar(fmt.Errorf("falta --capture"))
	}
	cfg.Cabecalhos = headers.valores
	if err := cfg.Validar(); err != nil {
		return falhar(err)
	}
	entrada, err := os.Open(*capture)
	if err != nil {
		return falhar(err)
	}
	defer entrada.Close()
	if *out != "" {
		origem, err := entrada.Stat()
		if err != nil {
			return falhar(err)
		}
		destino, statErr := os.Stat(*out)
		absEntrada, _ := filepath.Abs(*capture)
		absSaida, _ := filepath.Abs(*out)
		if absEntrada == absSaida || (statErr == nil && os.SameFile(origem, destino)) {
			return falhar(fmt.Errorf("--out não pode sobrescrever a captura"))
		}
	}
	resultado, err := repeticao.Repetir(ctx, cfg, entrada)
	if err != nil {
		return falhar(err)
	}
	var arquivo *os.File
	if *out != "" {
		arquivo, err = os.OpenFile(*out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return falhar(err)
		}
		stdout = arquivo
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	err = enc.Encode(resultado)
	if arquivo != nil {
		err = errors.Join(err, arquivo.Close())
	}
	if err != nil {
		return falhar(err)
	}
	return veredito.SaidaAprovado
}
