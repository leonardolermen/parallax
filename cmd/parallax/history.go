package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/leonardolermen/parallax/internal/julgamento"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

const usoHistory = `Uso:
  parallax history [--runs-dir PASTA]                 lista as execuções (mais recente primeiro)
  parallax history show ID [--json] [--runs-dir PASTA] mostra o laudo (ou o veredito.json) de uma execução

Flags:`

// historico implementa "history" e "history show".
func historico(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, usoHistory)
		fs.PrintDefaults()
	}
	runsDir := fs.String("runs-dir", runsDirPadrao, "pasta do histórico de execuções")
	comoJSON := fs.Bool("json", false, "show: imprime o veredito.json no lugar do laudo")
	falhar := func(err error) int {
		fmt.Fprintf(stderr, "parallax history: %v\n", err)
		return veredito.SaidaErro
	}
	// Aceita flags antes e depois dos argumentos: history show ID --json.
	var posicionais []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return veredito.SaidaAprovado
			}
			return veredito.SaidaErro
		}
		if fs.NArg() == 0 {
			break
		}
		posicionais = append(posicionais, fs.Arg(0))
		args = fs.Args()[1:]
	}
	h := julgamento.Historico{Dir: *runsDir}
	switch {
	case len(posicionais) == 0:
		if *comoJSON {
			return falhar(fmt.Errorf("--json só vale com show"))
		}
		return listarHistorico(h, stdout, stderr)
	case posicionais[0] != "show":
		return falhar(fmt.Errorf("argumento inesperado %q", posicionais[0]))
	case len(posicionais) != 2:
		return falhar(fmt.Errorf("uso: parallax history show ID"))
	}
	_, v, laudo, err := h.Ler(posicionais[1])
	if err != nil {
		return falhar(err)
	}
	if *comoJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		err = enc.Encode(v)
	} else {
		_, err = stdout.Write(laudo)
	}
	if err != nil {
		return falhar(err)
	}
	return veredito.SaidaAprovado
}

func listarHistorico(h julgamento.Historico, stdout, stderr io.Writer) int {
	es, errs := h.Listar()
	for _, err := range errs {
		fmt.Fprintf(stderr, "parallax history: aviso: %v\n", err)
	}
	if len(es) == 0 {
		fmt.Fprintf(stderr, "parallax history: nenhuma execução em %s\n", h.Dir)
		return veredito.SaidaAprovado
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tDATA\tRESULTADO\tBASE → CANDIDATA\tRESUMO")
	for _, e := range es {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s → %s\t%s\n", e.ID, e.Data.UTC().Format(time.RFC3339), e.Resultado, e.Base, e.Candidata, e.Resumo)
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(stderr, "parallax history: %v\n", err)
		return veredito.SaidaErro
	}
	return veredito.SaidaAprovado
}
