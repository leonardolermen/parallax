// Package repeticao repete capturas contra duas versoes HTTP e compara respostas.
package repeticao

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Base               string
	Candidata          string
	Trabalhadores      int
	Timeout            time.Duration
	PercentualLatencia float64
	MinimoLatenciaMs   float64
	LimiteAbsolutoMs   float64
	PermitirInseguros  bool
	Cabecalhos         http.Header
	MaxCorpo           int64
}

func ConfigPadrao() Config {
	return Config{Trabalhadores: 4, Timeout: 10 * time.Second, PercentualLatencia: 50, MinimoLatenciaMs: 20, MaxCorpo: 1048576}
}

func (c Config) Validar() error {
	for nome, raw := range map[string]string{"base": c.Base, "candidata": c.Candidata} {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || u.Fragment != "" {
			return fmt.Errorf("motor: URL %s deve ser http(s) absoluta sem fragmento", nome)
		}
		if porta := u.Port(); porta != "" {
			n, err := strconv.Atoi(porta)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("motor: porta da URL %s deve estar entre 1 e 65535", nome)
			}
		} else if strings.HasSuffix(u.Host, ":") {
			return fmt.Errorf("motor: porta vazia na URL %s", nome)
		}
		if _, err = http.NewRequest("GET", raw, nil); err != nil {
			return fmt.Errorf("motor: URL %s: %w", nome, err)
		}
	}
	if c.Trabalhadores <= 0 || c.Timeout <= 0 || c.MaxCorpo <= 0 || c.MaxCorpo == math.MaxInt64 {
		return fmt.Errorf("motor: workers, timeout e max-body devem ser positivos")
	}
	for nome, n := range map[string]float64{"latency-pct": c.PercentualLatencia, "latency-min-ms": c.MinimoLatenciaMs, "latency-max-ms": c.LimiteAbsolutoMs} {
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || (n == 0 && nome != "latency-max-ms") {
			return fmt.Errorf("motor: %s deve ser finito e positivo (latency-max-ms aceita zero)", nome)
		}
	}
	return validarCabecalhos(c.Cabecalhos)
}

func token(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c > 127 || !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}
func validarCabecalhos(h http.Header) error {
	for k, vs := range h {
		if !token(k) {
			return fmt.Errorf("motor: nome de cabecalho invalido %q", k)
		}
		for _, v := range vs {
			for _, c := range v {
				if (c < 32 && c != '\t') || c == 127 {
					return fmt.Errorf("motor: valor de cabecalho invalido em %q", k)
				}
			}
		}
	}
	return nil
}
