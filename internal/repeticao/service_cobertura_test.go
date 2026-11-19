package repeticao

// Testes da etapa de testes (#26): completam a cobertura do replay com casos
// ponta a ponta que a implementação (#25) não exercitava. Tudo em httptest
// local, sem rede real, e com prazos curtos para nunca travar a suíte.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leonardolermen/parallax/internal/captura"
	"github.com/leonardolermen/parallax/pkg/veredito"
)

func TestConfigPadraoSegueDecisoes(t *testing.T) {
	c := ConfigPadrao()
	if c.Trabalhadores != 4 || c.PercentualLatencia != 50 || c.MinimoLatenciaMs != 20 || c.LimiteAbsolutoMs != 0 || c.PermitirInseguros || c.Timeout <= 0 {
		t.Fatalf("padroes divergem das decisoes do usuario: %+v", c)
	}
	for _, raw := range []string{"http://127.0.0.1/#frag", "127.0.0.1:8080", "http:///sem-host", "mailto:a@b"} {
		c := configTeste("http://127.0.0.1")
		c.Candidata = raw
		if c.Validar() == nil {
			t.Errorf("URL invalida aceita: %q", raw)
		}
	}
}

// Regressão relativa medida de verdade: a candidata é ~80 ms mais lenta.
func TestRegressaoLatenciaPontaAPonta(t *testing.T) {
	rapido := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer rapido.Close()
	lento := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.Write([]byte("ok"))
	}))
	defer lento.Close()
	ts := []captura.Troca{}
	for i := 1; i <= 4; i++ {
		ts = append(ts, captura.Troca{Metodo: "GET", Caminho: fmt.Sprintf("/lento/%d", i)})
	}

	c := ConfigPadrao()
	c.Base, c.Candidata = rapido.URL, lento.URL
	r, err := Repetir(context.Background(), c, capturaTeste(ts...))
	if err != nil {
		t.Fatal(err)
	}
	if r.Estatisticas.Total != 4 || r.Estatisticas.Identicas != 4 || len(r.Achados) != 1 {
		t.Fatalf("esperava so o achado de latencia: %+v", r)
	}
	a := r.Achados[0]
	if a.Tipo != veredito.Latencia || a.Origem != veredito.Branch || a.Endpoint != "GET /lento/{id}" {
		t.Fatalf("achado de latencia errado: %+v", a)
	}
	if a.Candidata.LatenciaMs < 70 || a.Base.LatenciaMs >= a.Candidata.LatenciaMs || !strings.Contains(a.Reproducao, lento.URL) {
		t.Fatalf("p95 ou reproducao incoerentes: %+v", a)
	}
	if r.Estatisticas.Latencia.Delta.P95 < 50 {
		t.Fatalf("delta global incoerente: %+v", r.Estatisticas.Latencia)
	}

	// O percentual é configurável: com 10^7 % a mesma lentidão não é regressão
	// (a base no loopback leva <1 ms, então percentuais "normais" ainda disparam).
	c.PercentualLatencia = 1e7
	r, err = Repetir(context.Background(), c, capturaTeste(ts...))
	if err != nil || len(r.Achados) != 0 {
		t.Fatalf("percentual configurado ignorado: %+v %v", r, err)
	}
	// O mínimo em ms também: exigir 1000 ms de aumento afasta o achado.
	c = ConfigPadrao()
	c.Base, c.Candidata = rapido.URL, lento.URL
	c.MinimoLatenciaMs = 1000
	r, err = Repetir(context.Background(), c, capturaTeste(ts...))
	if err != nil || len(r.Achados) != 0 {
		t.Fatalf("minimo em ms configurado ignorado: %+v %v", r, err)
	}
}

// Servidores igualmente rápidos não podem gerar regressão com os padrões.
func TestSemRegressaoQuandoIgual(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer s.Close()
	c := ConfigPadrao()
	c.Base, c.Candidata = s.URL, s.URL
	ts := []captura.Troca{}
	for i := 0; i < 10; i++ {
		ts = append(ts, captura.Troca{Metodo: "GET", Caminho: "/igual"})
	}
	r, err := Repetir(context.Background(), c, capturaTeste(ts...))
	if err != nil || len(r.Achados) != 0 || r.Estatisticas.Identicas != 10 {
		t.Fatalf("%+v %v", r, err)
	}
}

// Cada requisição (base e candidata de cada troca) leva um trace novo, e o
// trace informado no achado é o mesmo que o serviço recebeu.
func TestTracesIndependentesPorRequisicao(t *testing.T) {
	var mu sync.Mutex
	recebidos := map[string]int{}
	handler := func(corpo string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			partes := strings.Split(r.Header.Get("Traceparent"), "-")
			if len(partes) == 4 {
				mu.Lock()
				recebidos[partes[1]]++
				mu.Unlock()
			}
			w.Write([]byte(corpo))
		}
	}
	base := httptest.NewServer(handler("a"))
	defer base.Close()
	cand := httptest.NewServer(handler("b"))
	defer cand.Close()
	ts := []captura.Troca{}
	for i := 0; i < 6; i++ {
		ts = append(ts, captura.Troca{Metodo: "GET", Caminho: "/t", CabecalhosRequisicao: http.Header{"Traceparent": {"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}}})
	}
	c := configTeste(base.URL)
	c.Candidata = cand.URL
	r, err := Repetir(context.Background(), c, capturaTeste(ts...))
	if err != nil || len(r.Achados) != 6 {
		t.Fatalf("%+v %v", r, err)
	}
	vistos := map[string]bool{}
	for _, a := range r.Achados {
		for _, id := range []string{a.Base.TraceID, a.Candidata.TraceID} {
			if len(id) != 32 || vistos[id] || recebidos[id] != 1 {
				t.Fatalf("trace %q repetido ou nao recebido (recebidos=%v)", id, recebidos)
			}
			vistos[id] = true
		}
	}
	if recebidos["0123456789abcdef0123456789abcdef"] != 0 {
		t.Fatal("trace gravado na captura foi reenviado")
	}
}

// Dentro de um par, a base termina antes de a candidata começar.
func TestParEnviaBaseAntesDaCandidata(t *testing.T) {
	var mu sync.Mutex
	baseFeita := map[string]bool{}
	var foraDeOrdem atomic.Int32
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Millisecond)
		mu.Lock()
		baseFeita[r.URL.Path] = true
		mu.Unlock()
	}))
	defer base.Close()
	cand := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if !baseFeita[r.URL.Path] {
			foraDeOrdem.Add(1)
		}
		mu.Unlock()
	}))
	defer cand.Close()
	ts := []captura.Troca{}
	for i := 0; i < 12; i++ {
		ts = append(ts, captura.Troca{Metodo: "GET", Caminho: fmt.Sprintf("/p/%d", i)})
	}
	c := configTeste(base.URL)
	c.Candidata = cand.URL
	if _, err := Repetir(context.Background(), c, capturaTeste(ts...)); err != nil {
		t.Fatal(err)
	}
	if foraDeOrdem.Load() != 0 {
		t.Fatalf("%d candidatas chegaram antes da base", foraDeOrdem.Load())
	}
}

// Com --allow-unsafe, método, corpo e query capturados chegam iguais aos dois lados.
func TestReenviaMetodoCorpoEQuery(t *testing.T) {
	var mu sync.Mutex
	var vistos []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		vistos = append(vistos, r.Method+" "+r.URL.RequestURI()+" "+string(b)+" "+r.Header.Get("Content-Type"))
		mu.Unlock()
	}))
	defer s.Close()
	c := configTeste(s.URL)
	c.PermitirInseguros = true
	tr := captura.Troca{Metodo: "POST", Caminho: "/pedidos", Query: "a=1&b=2", CorpoRequisicao: []byte(`{"x":1}`), CabecalhosRequisicao: http.Header{"Content-Type": {"application/json"}}}
	r, err := Repetir(context.Background(), c, capturaTeste(tr))
	if err != nil || r.Estatisticas.Total != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	want := `POST /pedidos?a=1&b=2 {"x":1} application/json`
	if len(vistos) != 2 || vistos[0] != want || vistos[1] != want {
		t.Fatalf("requisicoes reenviadas: %q", vistos)
	}
}

// JSON igual em semântica (ordem de chaves, espaços) e cabeçalhos irrelevantes
// diferentes (Date, X-Request-Id) não geram achado.
func TestComparacaoIgnoraFormaJSONECabecalhosIrrelevantes(t *testing.T) {
	b := resposta{Lado: veredito.Lado{Status: 200}, Corpo: []byte(`{"a":1,"b":[1,2]}`), Cabecalhos: http.Header{"Date": {"ontem"}, "X-Request-Id": {"1"}, "Content-Type": {"application/json"}}}
	a := resposta{Lado: veredito.Lado{Status: 200}, Corpo: []byte("{ \"b\": [1, 2],\n \"a\": 1 }"), Cabecalhos: http.Header{"Date": {"hoje"}, "X-Request-Id": {"2"}, "Content-Type": {"application/json"}}}
	if as := comparar(captura.Troca{Metodo: "GET", Caminho: "/"}, ConfigPadrao(), b, a); len(as) != 0 {
		t.Fatalf("JSON equivalente gerou achado: %+v", as)
	}
	// Campo novo na candidata é diferença, não quebra de contrato.
	a.Corpo = []byte(`{"a":1,"b":[1,2],"c":true}`)
	if as := comparar(captura.Troca{Metodo: "GET", Caminho: "/"}, ConfigPadrao(), b, a); len(as) != 1 || as[0].Tipo != veredito.Diferenca || as[0].Diff[0].Caminho != "$.c" {
		t.Fatalf("campo adicionado: %+v", as)
	}
}

// Corpo acima de --max-body vira comparação incompleta e fica fora dos percentis.
func TestCorpoExcessivoIncompletoForaDaLatencia(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(bytes.Repeat([]byte("x"), 64)) }))
	defer s.Close()
	c := configTeste(s.URL)
	c.MaxCorpo = 10
	c.LimiteAbsolutoMs = 1e-9
	r, err := Repetir(context.Background(), c, capturaTeste(captura.Troca{Metodo: "GET", Caminho: "/grande"}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Estatisticas.Diferentes != 1 || len(r.Achados) != 1 || r.Achados[0].Tipo != veredito.Diferenca {
		t.Fatalf("%+v", r)
	}
	if !strings.HasSuffix(r.Achados[0].Candidata.Corpo, "[comparacao incompleta]") || !strings.HasPrefix(r.Achados[0].Candidata.Corpo, "xxxxxxxxxx ") {
		t.Fatalf("resumo do corpo: %q", r.Achados[0].Candidata.Corpo)
	}
	if r.Estatisticas.Latencia.Base.P95 != 0 || r.Estatisticas.Latencia.Candidata.P95 != 0 {
		t.Fatalf("resposta incompleta entrou nos percentis: %+v", r.Estatisticas.Latencia)
	}
}

// Timeout da candidata vira erro_novo e não contamina os percentis nem gera
// achado de latência, mesmo com teto absoluto minúsculo.
func TestTimeoutForaDaLatencia(t *testing.T) {
	rapido := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer rapido.Close()
	preso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer preso.Close()
	c := configTeste(rapido.URL)
	c.Candidata = preso.URL
	c.Timeout = 100 * time.Millisecond
	c.LimiteAbsolutoMs = 1e-9
	r, err := Repetir(context.Background(), c, capturaTeste(captura.Troca{Metodo: "GET", Caminho: "/a"}, captura.Troca{Metodo: "GET", Caminho: "/b"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Achados) != 2 || r.Estatisticas.Diferentes != 2 {
		t.Fatalf("%+v", r)
	}
	for _, a := range r.Achados {
		if a.Tipo != veredito.ErroNovo || a.Origem != veredito.Branch || a.Candidata.Erro != "timeout" || a.Candidata.Status != 0 {
			t.Fatalf("achado: %+v", a)
		}
	}
	if r.Estatisticas.Latencia.Candidata.P95 != 0 {
		t.Fatalf("timeout entrou nos percentis: %+v", r.Estatisticas.Latencia)
	}
}

// Contexto já cancelado: erro e nenhuma requisição.
func TestContextoCanceladoNaoEnvia(t *testing.T) {
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1) }))
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Repetir(ctx, configTeste(s.URL), capturaTeste(captura.Troca{Metodo: "GET", Caminho: "/"})); err == nil {
		t.Fatal("contexto cancelado aceito")
	}
	if n.Load() != 0 {
		t.Fatalf("enviou %d requisicoes com contexto cancelado", n.Load())
	}
}

// Cancelar no meio de uma captura longa encerra rápido, sem travar a suíte.
func TestCancelamentoNoMeioEncerraRapido(t *testing.T) {
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(50 * time.Millisecond):
		}
	}))
	defer s.Close()
	ts := []captura.Troca{}
	for i := 0; i < 200; i++ {
		ts = append(ts, captura.Troca{Metodo: "GET", Caminho: "/"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	inicio := time.Now()
	_, err := Repetir(ctx, configTeste(s.URL), capturaTeste(ts...))
	if err == nil {
		t.Fatal("cancelamento ignorado")
	}
	if d := time.Since(inicio); d > 2*time.Second {
		t.Fatalf("demorou %s para encerrar apos cancelamento", d)
	}
	if n.Load() >= 400 {
		t.Fatalf("captura inteira enviada apesar do cancelamento: %d", n.Load())
	}
}

// Uma linha inválida no fim de uma captura longa aborta sem enviar nada.
func TestLinhaInvalidaNoFimAbortaSemTrafego(t *testing.T) {
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1) }))
	defer s.Close()
	ts := []captura.Troca{}
	for i := 0; i < 50; i++ {
		ts = append(ts, captura.Troca{Metodo: "GET", Caminho: "/ok"})
	}
	entrada := capturaTeste(ts...)
	entrada.WriteString("{\"metodo\":\n")
	_, err := Repetir(context.Background(), configTeste(s.URL), entrada)
	if err == nil {
		t.Fatal("linha quebrada aceita")
	}
	if n.Load() != 0 {
		t.Fatalf("enviou %d requisicoes antes de validar a captura", n.Load())
	}
	// Linha 51 com método inválido: o erro aponta a troca.
	entrada = capturaTeste(append(ts, captura.Troca{Metodo: "G T", Caminho: "/"})...)
	_, err = Repetir(context.Background(), configTeste(s.URL), entrada)
	if err == nil || !strings.Contains(err.Error(), "troca 51") || n.Load() != 0 {
		t.Fatalf("err=%v enviadas=%d", err, n.Load())
	}
}

func TestPercentilBordas(t *testing.T) {
	if Percentil(nil, 95) != 0 {
		t.Error("vazio")
	}
	if Percentil([]float64{7}, 95) != 7 || Percentil([]float64{7}, 50) != 7 {
		t.Error("uma amostra")
	}
	s := []float64{}
	for i := 20; i >= 1; i-- {
		s = append(s, float64(i))
	}
	if Percentil(s, 95) != 19 || Percentil(s, 50) != 10 {
		t.Errorf("nearest rank com 20 amostras: p95=%v p50=%v", Percentil(s, 95), Percentil(s, 50))
	}
	if s[0] != 20 {
		t.Error("Percentil alterou as amostras")
	}
	// Base zero (Windows mede 0 em trocas muito rápidas se o relógio falhar):
	// só passa a ser regressão com o mínimo em ms atingido.
	c := ConfigPadrao()
	if ok, _ := Regressao(0, 19, c); ok {
		t.Error("base zero com 19 ms nao e regressao")
	}
	if ok, _ := Regressao(0, 20, c); !ok {
		t.Error("base zero com 20 ms e regressao")
	}
}
