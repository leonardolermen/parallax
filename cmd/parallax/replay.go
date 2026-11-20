package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/leonardolermen/parallax/internal/repeticao"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

// repetir implementa replay sem calcular veredito ou gravar histórico.
func repetir(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg := repeticao.ConfigPadrao()
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Uso: parallax replay --capture ARQUIVO --base URL --candidate URL [flags]\n\nRepete e compara; produz achados JSON sem veredito final.\n\nFlags:")
		fs.PrintDefaults()
	}
	capture, headers := registrarFlagsRepeticao(fs, &cfg)
	out := fs.String("out", "", "arquivo JSON de saída; vazio = stdout")
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
