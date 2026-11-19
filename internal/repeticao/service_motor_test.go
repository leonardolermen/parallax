package repeticao

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leonardolermen/parallax/internal/captura"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

func TestClienteHeadersTracesGzipRedirect(t *testing.T) {
	var mu sync.Mutex
	traces := map[string]bool{}
	var bad string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		tr := r.Header.Get("Traceparent")
		if !regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`).MatchString(tr) || traces[tr] {
			bad = "trace repetido/invalido"
		}
		traces[tr] = true
		if r.Header.Get("X-Hop") != "" || r.Header.Get("Authorization") != "token" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Injected") != "yes" {
			bad = fmt.Sprint(r.Header)
		}
		w.Header().Set("Content-Encoding", "gzip")
		g := gzip.NewWriter(w)
		g.Write([]byte(strings.Repeat("a", 200)))
		g.Close()
	}))
	defer s.Close()
	c := configTeste(s.URL)
	c.MaxCorpo = 100
	c.Cabecalhos = http.Header{"Authorization": {"token"}, "X-Injected": {"yes"}}
	tr := captura.Troca{Metodo: "GET", Caminho: "/", CabecalhosRequisicao: http.Header{"Connection": {"X-Hop"}, "X-Hop": {"remove"}, "Cookie": {"***"}, "Host": {"wrong"}, "Traceparent": {"old"}}}
	r, e := Repetir(context.Background(), c, capturaTeste(tr))
	if e != nil {
		t.Fatal(e)
	}
	if bad != "" {
		t.Fatal(bad)
	}
	if len(traces) != 2 || len(r.Achados) != 1 || r.Achados[0].Tipo != veredito.Diferenca {
		t.Fatalf("%+v", r)
	}
	redirects := atomic.Int32{}
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirects.Add(1) }))
	defer dst.Close()
	red := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, dst.URL, 302) }))
	defer red.Close()
	r, e = Repetir(context.Background(), configTeste(red.URL), capturaTeste(tr))
	if e != nil || redirects.Load() != 0 || r.Estatisticas.Identicas != 1 {
		t.Fatalf("redirect seguido: %+v %v", r, e)
	}
}
func TestClienteTimeoutRecusaCancelamento(t *testing.T) {
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer fast.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	c := configTeste(fast.URL)
	c.Candidata = slow.URL
	c.Timeout = 200 * time.Millisecond
	tr := captura.Troca{Metodo: "GET", Caminho: "/"}
	start := time.Now()
	r, e := Repetir(context.Background(), c, capturaTeste(tr))
	if e != nil || len(r.Achados) != 1 || r.Achados[0].Candidata.Erro != "timeout" || r.Achados[0].Tipo != veredito.ErroNovo || time.Since(start) > time.Second {
		t.Fatalf("%+v %v", r, e)
	}
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := l.Addr().String()
	l.Close()
	c.Candidata = "http://" + addr
	r, e = Repetir(context.Background(), c, capturaTeste(tr))
	if e != nil || len(r.Achados) != 1 || r.Achados[0].Candidata.Erro != "conexao_recusada" {
		t.Fatalf("%+v %v", r, e)
	}
	c.Candidata = slow.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(20*time.Millisecond, cancel)
	defer timer.Stop()
	_, e = Repetir(ctx, c, capturaTeste(tr))
	if e == nil {
		t.Fatal("cancelamento ignorado")
	}
}
func TestExecutorLimiteOrdem(t *testing.T) {
	var active, peak atomic.Int32
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		w.Write([]byte("base"))
	}))
	defer base.Close()
	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(r.URL.Path)) }))
	defer candidate.Close()
	ts := []captura.Troca{}
	for i := 0; i < 20; i++ {
		ts = append(ts, captura.Troca{Metodo: "GET", Caminho: fmt.Sprintf("/x/%d", i)})
	}
	c := configTeste(base.URL)
	c.Candidata = candidate.URL
	c.Trabalhadores = 3
	r, e := Repetir(context.Background(), c, capturaTeste(ts...))
	if e != nil {
		t.Fatal(e)
	}
	if peak.Load() > 3 || peak.Load() < 2 {
		t.Fatalf("peak %d", peak.Load())
	}
	if len(r.Achados) != 20 {
		t.Fatalf("%+v", r)
	}
	for i, a := range r.Achados {
		if a.Candidata.Corpo != fmt.Sprintf("/x/%d", i) {
			t.Fatal("ordem", i, a)
		}
	}
}
func TestComparadorTiposHeadersNumerosLimite(t *testing.T) {
	cases := []struct {
		base, cand string
		want       veredito.Tipo
	}{{`{"a":1}`, `{}`, veredito.ContratoQuebrado}, {`{"a":null}`, `{"a":1}`, veredito.ContratoQuebrado}, {`{"a":[1]}`, `{"a":["1"]}`, veredito.ContratoQuebrado}, {`{"a":9007199254740992}`, `{"a":9007199254740993}`, veredito.Diferenca}, {`hello`, `bye`, veredito.Diferenca}}
	for _, tt := range cases {
		t.Run(tt.base+tt.cand, func(t *testing.T) {
			b := resposta{Lado: veredito.Lado{Status: 200}, Corpo: []byte(tt.base)}
			c := resposta{Lado: veredito.Lado{Status: 200}, Corpo: []byte(tt.cand)}
			as := comparar(captura.Troca{Metodo: "GET", Caminho: "/"}, ConfigPadrao(), b, c)
			if len(as) != 1 || as[0].Tipo != tt.want {
				t.Fatalf("%+v", as)
			}
		})
	}
	b := map[string]any{}
	c := map[string]any{}
	for i := 0; i < 60; i++ {
		key := fmt.Sprintf("a%02d", i)
		b[key] = 1
		c[key] = 2
	}
	b["zzRemoved"] = true
	bb, _ := json.Marshal(b)
	cc, _ := json.Marshal(c)
	as := comparar(captura.Troca{Metodo: "GET", Caminho: "/"}, ConfigPadrao(), resposta{Lado: veredito.Lado{Status: 200}, Corpo: bb}, resposta{Lado: veredito.Lado{Status: 200}, Corpo: cc})
	if len(as) != 1 || as[0].Tipo != veredito.ContratoQuebrado || len(as[0].Diff) != 50 {
		t.Fatalf("%+v", as)
	}
	for _, h := range []string{"Location", "Content-Type", "Cache-Control"} {
		a := resposta{Lado: veredito.Lado{Status: 200}, Cabecalhos: http.Header{h: {"old"}}}
		b := resposta{Lado: veredito.Lado{Status: 200}, Cabecalhos: http.Header{h: {"new"}}}
		if len(comparar(captura.Troca{}, ConfigPadrao(), a, b)) != 1 {
			t.Error(h)
		}
	}
	for _, status := range []int{200, 500} {
		as := comparar(captura.Troca{}, ConfigPadrao(), resposta{Lado: veredito.Lado{Status: status}}, resposta{Lado: veredito.Lado{Status: 500}})
		if len(as) != 1 || as[0].Tipo != veredito.ErroNovo {
			t.Fatal(as)
		}
		want := veredito.Branch
		if status == 500 {
			want = veredito.PreExistente
		}
		if as[0].Origem != want {
			t.Fatal(as)
		}
	}
}

func TestCaminhoDecodificadoEQueryPrefixo(t *testing.T) {
	var got string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.URL.EscapedPath() + "?" + r.URL.RawQuery }))
	defer s.Close()
	c := configTeste(s.URL + "/prefix")
	r, e := Repetir(context.Background(), c, capturaTeste(captura.Troca{Metodo: "GET", Caminho: "/a b?c#d", Query: "x=1&x=2"}))
	if e != nil || r.Estatisticas.Identicas != 1 || got != "/prefix/a%20b%3Fc%23d?x=1&x=2" {
		t.Fatalf("got=%q r=%+v err=%v", got, r, e)
	}
	for _, mut := range []func(*Config){func(c *Config) { c.PercentualLatencia = 0 }, func(c *Config) { c.MinimoLatenciaMs = 0 }} {
		bad := c
		mut(&bad)
		if bad.Validar() == nil {
			t.Error("limite zero aceito")
		}
	}
}

func TestStatusDiferenteNaoInfereContrato(t *testing.T) {
	a := resposta{Lado: veredito.Lado{Status: 200}, Corpo: []byte(`{"id":1}`)}
	b := resposta{Lado: veredito.Lado{Status: 404}, Corpo: []byte("not found")}
	as := comparar(captura.Troca{}, ConfigPadrao(), a, b)
	if len(as) != 1 || as[0].Tipo != veredito.Diferenca {
		t.Fatal(as)
	}
	if erroHTTP(fmt.Errorf("erro local")) != "falha" {
		t.Fatal("erroHTTP deve estabilizar erros desconhecidos")
	}
}

func TestDiffCabecalhoExplicaMudanca(t *testing.T) {
	a := resposta{Lado: veredito.Lado{Status: 200}, Cabecalhos: http.Header{"Location": {"/old"}}}
	b := resposta{Lado: veredito.Lado{Status: 200}, Cabecalhos: http.Header{"Location": {"/new"}}}
	as := comparar(captura.Troca{}, ConfigPadrao(), a, b)
	if len(as) != 1 || len(as[0].Diff) != 1 || as[0].Diff[0].Caminho != "$.cabecalhos_resposta.Location" || string(as[0].Diff[0].Antes) != `["/old"]` || string(as[0].Diff[0].Depois) != `["/new"]` {
		t.Fatalf("%+v", as)
	}
}

func TestOverridesNaoReintroduzemHopCapturado(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Private-Hop") != "" {
			t.Error("hop capturado vazou")
		}
	}))
	defer s.Close()
	c := configTeste(s.URL)
	c.Cabecalhos = http.Header{"Connection": {"keep-alive"}}
	_, err := Repetir(context.Background(), c, capturaTeste(captura.Troca{Metodo: "GET", Caminho: "/", CabecalhosRequisicao: http.Header{"Connection": {"X-Private-Hop"}, "X-Private-Hop": {"secret"}}}))
	if err != nil {
		t.Fatal(err)
	}
}

func TestErroRecusadoTextual(t *testing.T) {
	if erroHTTP(fmt.Errorf("connect: connection REFUSED")) != "conexao_recusada" {
		t.Fatal("fallback de conexao recusada")
	}
}
