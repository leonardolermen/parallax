package repeticao

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/leonardolermen/parallax/internal/captura"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

func TestGzipSemCorpoPermitido(t *testing.T) {
	for _, tc := range []struct {
		metodo string
		status int
	}{{"HEAD", 200}, {"GET", 204}, {"GET", 304}} {
		t.Run(tc.metodo+http.StatusText(tc.status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", "gzip")
				w.WriteHeader(tc.status)
			}))
			defer s.Close()
			c := configTeste(s.URL)
			tr := captura.Troca{Metodo: tc.metodo, Caminho: "/"}
			client := novoCliente(c)
			defer client.CloseIdleConnections()
			got := enviar(context.Background(), client, c, s.URL, tr)
			if got.Incompleta || got.Erro != "" || len(got.Corpo) != 0 {
				t.Errorf("resposta valida tratada como falha: %+v", got)
			}
			r, err := Repetir(context.Background(), c, capturaTeste(tr))
			if err != nil {
				t.Fatal(err)
			}
			if r.Estatisticas.Identicas != 1 || len(r.Achados) != 0 || r.Estatisticas.Latencia.Base.P95 <= 0 || r.Estatisticas.Latencia.Candidata.P95 <= 0 {
				t.Fatalf("par sem corpo deve ser identico e elegivel a latencia: %+v", r)
			}
		})
	}
}

func TestRegressaoPrioridadeTeto(t *testing.T) {
	for _, tc := range []struct {
		nome                  string
		base, candidata, teto float64
		ok                    bool
		origem                veredito.Origem
	}{
		{"relativo vence teto preexistente", 60, 100, 50, true, veredito.Branch},
		{"piora grande com teto preexistente", 60, 1000, 50, true, veredito.Branch},
		{"teto preexistente sem minimo em ms", 60, 79, 50, true, veredito.PreExistente},
		{"somente teto preexistente", 60, 61, 50, true, veredito.PreExistente},
		{"ambos criterios novos", 30, 80, 50, true, veredito.Branch},
		{"somente relativo", 40, 80, 100, true, veredito.Branch},
		{"somente teto novo", 49, 51, 50, true, veredito.Branch},
		{"base exatamente teto", 50, 100, 50, true, veredito.Branch},
		{"candidata exatamente teto", 49, 50, 50, false, veredito.Branch},
		{"teto desativado", 60, 100, 0, true, veredito.Branch},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			c := ConfigPadrao()
			c.LimiteAbsolutoMs = tc.teto
			ok, o := Regressao(tc.base, tc.candidata, c)
			if ok != tc.ok || o != tc.origem {
				t.Fatalf("got (%v,%s), want (%v,%s)", ok, o, tc.ok, tc.origem)
			}
		})
	}
}

func TestPortasInvalidasZeroTrafego(t *testing.T) {
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1) }))
	defer s.Close()
	for _, raw := range []string{"http://127.0.0.1:99999", "http://127.0.0.1:0", "http://127.0.0.1:-1", "http://127.0.0.1:abc", "http://127.0.0.1:", "http://[::1]:65536", "http://[::1]:"} {
		for _, lado := range []string{"base", "candidata"} {
			t.Run(lado+raw, func(t *testing.T) {
				n.Store(0)
				c := configTeste(s.URL)
				if lado == "base" {
					c.Base = raw
				} else {
					c.Candidata = raw
				}
				if c.Validar() == nil {
					t.Errorf("porta invalida aceita: %q", raw)
				}
				_, err := Repetir(context.Background(), c, capturaTeste(captura.Troca{Metodo: "GET", Caminho: "/"}))
				if err == nil {
					t.Error("esperava erro de configuracao")
				}
				if n.Load() != 0 {
					t.Fatalf("trafego enviado antes de validar porta: %d", n.Load())
				}
			})
		}
	}
	for _, raw := range []string{"http://127.0.0.1", "https://example.test", "http://127.0.0.1:80", "https://example.test:443", "http://127.0.0.1:65535", "http://[::1]", "http://[::1]:80", "https://[2001:db8::1]:443"} {
		c := configTeste(raw)
		if err := c.Validar(); err != nil {
			t.Errorf("URL valida %q rejeitada: %v", raw, err)
		}
	}
}

func TestLatenciaAgrupadaOrdenadaNoResultado(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer s.Close()
	c := configTeste(s.URL)
	c.LimiteAbsolutoMs = 1e-9
	r, err := Repetir(context.Background(), c, capturaTeste(captura.Troca{Metodo: "GET", Caminho: "/x/1"}, captura.Troca{Metodo: "GET", Caminho: "/x/2"}, captura.Troca{Metodo: "GET", Caminho: "/outro"}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Estatisticas.Total != 3 || r.Estatisticas.Identicas != 3 || r.Estatisticas.Diferentes != 0 || len(r.Achados) != 2 {
		t.Fatalf("%+v", r)
	}
	for i, ep := range []string{"GET /outro", "GET /x/{id}"} {
		a := r.Achados[i]
		if a.Tipo != veredito.Latencia || a.Origem != veredito.PreExistente || a.Endpoint != ep || a.Base.LatenciaMs <= 0 || a.Candidata.LatenciaMs <= 0 {
			t.Fatalf("achado %d: %+v", i, a)
		}
	}
	if r.Estatisticas.Latencia.Base.P95 <= 0 || r.Estatisticas.Latencia.Candidata.P95 <= 0 {
		t.Fatalf("percentis ausentes: %+v", r.Estatisticas.Latencia)
	}
}
