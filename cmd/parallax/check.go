package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/leonardolermen/parallax/internal/julgamento"
	"github.com/leonardolermen/parallax/internal/repeticao"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

// runsDirPadrao é a pasta padrão do histórico de execuções.
const runsDirPadrao = ".parallax/runs"

// checar implementa o check: repete, julga, escreve o laudo e guarda a
// execução no histórico. Sai com o código do veredito (0, 1 ou 2) ou 4 em
// erro, inclusive quando o histórico não pôde ser gravado.
func checar(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg := repeticao.ConfigPadrao()
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Uso: parallax check --capture ARQUIVO --base URL --candidate URL [flags]\n\nRepete, compara e devolve o veredito: imprime o laudo e guarda a execução no histórico.\nSaída: 0 aprovado, 1 regressão, 2 revisar, 4 erro.\n\nFlags:")
		fs.PrintDefaults()
	}
	capture, headers := registrarFlagsRepeticao(fs, &cfg)
	comoJSON := fs.Bool("json", false, "imprime o veredito.json no lugar do laudo")
	report := fs.String("report", "", "grava também o laudo Markdown neste arquivo")
	runsDir := fs.String("runs-dir", runsDirPadrao, "pasta do histórico de execuções")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return veredito.SaidaAprovado
		}
		return veredito.SaidaErro
	}
	falhar := func(err error) int {
		fmt.Fprintf(stderr, "parallax check: %v\n", err)
		return veredito.SaidaErro
	}
	if fs.NArg() > 0 {
		return falhar(fmt.Errorf("argumento inesperado %q", fs.Arg(0)))
	}
	if *capture == "" {
		return falhar(fmt.Errorf("falta --capture"))
	}
	if *runsDir == "" {
		return falhar(fmt.Errorf("--runs-dir vazio"))
	}
	cfg.Cabecalhos = headers.valores
	if err := cfg.Validar(); err != nil {
		return falhar(err)
	}
	absCaptura, err := filepath.Abs(*capture)
	if err != nil {
		return falhar(err)
	}
	entrada, err := os.Open(*capture)
	if err != nil {
		return falhar(err)
	}
	defer entrada.Close()
	if *report != "" {
		if mesmoArquivo(entrada, absCaptura, *report) {
			return falhar(fmt.Errorf("--report não pode sobrescrever a captura"))
		}
	}

	resultado, err := repeticao.Repetir(ctx, cfg, entrada)
	if err != nil {
		return falhar(err)
	}
	v, err := julgamento.Julgar(resultado)
	if errors.Is(err, julgamento.ErrSemEvidencia) {
		return falhar(fmt.Errorf("%w (%d puladas; métodos inseguros só com --allow-unsafe)", err, resultado.Estatisticas.Puladas))
	}
	if err != nil {
		return falhar(err)
	}
	e := julgamento.Execucao{
		ID:           v.ID,
		Data:         time.Now().UTC(),
		Captura:      absCaptura,
		Base:         semSenha(cfg.Base),
		Candidata:    semSenha(cfg.Candidata),
		Resultado:    v.Resultado,
		Resumo:       v.Resumo,
		Estatisticas: resultado.Estatisticas,
	}
	var laudo bytes.Buffer
	if err := julgamento.EscreverLaudo(&laudo, e, v); err != nil {
		return falhar(err)
	}

	// O histórico é gravado antes de imprimir, mas a falha só é reportada
	// depois: o laudo aparece de qualquer jeito.
	h := julgamento.Historico{Dir: *runsDir}
	errHistorico := h.Salvar(e, v, laudo.Bytes())

	var errSaida error
	if *comoJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		errSaida = enc.Encode(v)
	} else {
		_, errSaida = stdout.Write(laudo.Bytes())
	}
	if *report != "" {
		errSaida = errors.Join(errSaida, os.WriteFile(*report, laudo.Bytes(), 0o600))
	}
	if errHistorico != nil {
		errSaida = errors.Join(errSaida, fmt.Errorf("execução %s não gravada no %v", v.ID, errHistorico))
	} else {
		fmt.Fprintf(stderr, "parallax check: %s; execução %s gravada em %s\n", v.Resultado, v.ID, filepath.Join(*runsDir, v.ID))
	}
	if errSaida != nil {
		return falhar(errSaida)
	}
	return veredito.CodigoSaida(v.Resultado)
}

// mesmoArquivo diz se caminho aponta para a captura já aberta.
func mesmoArquivo(captura *os.File, absCaptura, caminho string) bool {
	if abs, err := filepath.Abs(caminho); err == nil && abs == absCaptura {
		return true
	}
	origem, err := captura.Stat()
	if err != nil {
		return false
	}
	destino, err := os.Stat(caminho)
	return err == nil && os.SameFile(origem, destino)
}

// semSenha tira a senha de URLs como http://user:senha@host antes de gravar.
func semSenha(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return u
	}
	return p.Redacted()
}
