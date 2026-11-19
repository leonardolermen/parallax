package repeticao

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leonardolermen/parallax/internal/captura"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

func capturaTeste(ts ...captura.Troca) *bytes.Buffer {
	b := new(bytes.Buffer)
	for _, v := range ts {
		_ = json.NewEncoder(b).Encode(v)
	}
	return b
}
func configTeste(url string) Config {
	c := ConfigPadrao()
	c.Base = url
	c.Candidata = url
	c.MinimoLatenciaMs = 1000000
	return c
}
func TestValidacaoAntesTrafego(t *testing.T) {
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1) }))
	defer s.Close()
	valid := captura.Troca{Metodo: "GET", Caminho: "/ok"}
	for _, bad := range []captura.Troca{{Metodo: "GET\n", Caminho: "/"}, {Metodo: "GET", Caminho: "http://evil"}, {Metodo: "POST", Caminho: "/", CabecalhosRequisicao: http.Header{"X-Bad": {"bad\r\nx:y"}}}, {Metodo: "GET", Caminho: "//evil"}, {Metodo: "GET", Caminho: "/", Query: "x=\n"}} {
		if _, err := Repetir(context.Background(), configTeste(s.URL), capturaTeste(valid, bad)); err == nil {
			t.Errorf("aceitou %+v", bad)
		}
	}
	for _, input := range []string{"", "\n", "{bad}", "{}"} {
		if _, err := Repetir(context.Background(), configTeste(s.URL), strings.NewReader(input)); err == nil {
			t.Errorf("aceitou %q", input)
		}
	}
	c := configTeste(s.URL)
	c.Cabecalhos = http.Header{"bad name": {"x"}}
	if _, err := Repetir(context.Background(), c, capturaTeste(valid)); err == nil {
		t.Error("header invalido")
	}
	if n.Load() != 0 {
		t.Fatal("enviou trafego antes de validar")
	}
	for _, mut := range []func(*Config){func(c *Config) { c.Base = "ftp://a" }, func(c *Config) { c.Trabalhadores = 0 }, func(c *Config) { c.Timeout = 0 }, func(c *Config) { c.MaxCorpo = 0 }, func(c *Config) { c.PercentualLatencia = math.NaN() }, func(c *Config) { c.MinimoLatenciaMs = -1 }, func(c *Config) { c.LimiteAbsolutoMs = math.Inf(1) }} {
		c := configTeste(s.URL)
		mut(&c)
		if c.Validar() == nil {
			t.Error("config invalida aceita")
		}
	}
}
func TestReplayPulaInsegurosETruncados(t *testing.T) {
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1); w.Write([]byte("ok")) }))
	defer s.Close()
	ts := []captura.Troca{{Metodo: "GET", Caminho: "/"}, {Metodo: "POST", Caminho: "/"}, {Metodo: "PUT", Caminho: "/", CorpoRequisicaoTruncado: true}}
	r, err := Repetir(context.Background(), configTeste(s.URL), capturaTeste(ts...))
	if err != nil {
		t.Fatal(err)
	}
	if r.Estatisticas.Total != 1 || r.Estatisticas.Puladas != 2 || r.Estatisticas.Identicas != 1 || n.Load() != 2 {
		t.Fatalf("%+v calls=%d", r, n.Load())
	}
	b, _ := json.Marshal(Resultado{})
	if !bytes.Contains(b, []byte(`"achados":[]`)) {
		t.Fatal(string(b))
	}
	c := configTeste(s.URL)
	c.PermitirInseguros = true
	r, err = Repetir(context.Background(), c, capturaTeste(ts...))
	if err != nil || r.Estatisticas.Total != 2 {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestEndpointPercentilRegressao(t *testing.T) {
	for in, want := range map[string]string{"/users/123": "GET /users/{id}", "/a/550e8400-e29b-41d4-a716-446655440000": "GET /a/{id}", "/a/01ARYZ6S41TSV4RRFFQ69G5FAV": "GET /a/{id}", "/a/abcdef0123456789": "GET /a/{id}", "/v1/items": "GET /v1/items"} {
		if got := Endpoint("GET", in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
	if Percentil([]float64{5, 1, 4, 2, 3}, 50) != 3 || Percentil([]float64{5, 1, 4, 2, 3}, 95) != 5 {
		t.Fatal("nearest rank")
	}
	c := ConfigPadrao()
	if ok, _ := Regressao(40, 60, c); ok {
		t.Error("50% exatos")
	}
	if ok, _ := Regressao(39, 59, c); !ok {
		t.Error("20 ms exatos")
	}
	c.LimiteAbsolutoMs = 50
	if ok, o := Regressao(60, 61, c); !ok || o != veredito.PreExistente {
		t.Error("teto pre-existente")
	}
}
