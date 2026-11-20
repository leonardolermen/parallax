package main

import (
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/leonardolermen/parallax/internal/repeticao"
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

// registrarFlagsRepeticao registra as flags comuns a replay e check, para as
// duas listas não divergirem. Os cabeçalhos ficam em headers.valores e devem
// ir para cfg.Cabecalhos depois do Parse.
func registrarFlagsRepeticao(fs *flag.FlagSet, cfg *repeticao.Config) (capture *string, headers *cabecalhosReplay) {
	capture = fs.String("capture", "", "captura JSONL (obrigatória)")
	fs.StringVar(&cfg.Base, "base", "", "URL HTTP(S) da base (obrigatória)")
	fs.StringVar(&cfg.Candidata, "candidate", "", "URL HTTP(S) da candidata (obrigatória)")
	fs.IntVar(&cfg.Trabalhadores, "workers", cfg.Trabalhadores, "máximo de trabalhadores")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "timeout por requisição (ex.: 10s)")
	fs.Float64Var(&cfg.PercentualLatencia, "latency-pct", cfg.PercentualLatencia, "aumento percentual do p95")
	fs.Float64Var(&cfg.MinimoLatenciaMs, "latency-min-ms", cfg.MinimoLatenciaMs, "aumento mínimo do p95 em ms")
	fs.Float64Var(&cfg.LimiteAbsolutoMs, "latency-max-ms", cfg.LimiteAbsolutoMs, "teto absoluto do p95 em ms; 0 desativa")
	fs.BoolVar(&cfg.PermitirInseguros, "allow-unsafe", false, "repetir também métodos inseguros")
	fs.Int64Var(&cfg.MaxCorpo, "max-body", cfg.MaxCorpo, "máximo de bytes por resposta; excesso gera comparação incompleta")
	headers = &cabecalhosReplay{valores: make(http.Header)}
	fs.Var(headers, "header", "cabeçalho Nome: valor para ambos os destinos; repetível")
	return capture, headers
}
