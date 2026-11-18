package motor

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ambiente é um upstream httptest com o gravador na frente.
type ambiente struct {
	upstream *httptest.Server
	cliente  *httptest.Server // servidor do gravador; é nele que o teste faz as requisições
	captura  *bytes.Buffer
	g        *Gravador
}

// montar sobe o upstream e o gravador. Preenche cfg.Upstream e cfg.Saida.
func montar(t *testing.T, upstream http.Handler, cfg ConfigGravador) *ambiente {
	return montarComBase(t, upstream, cfg, "")
}

// montarComBase é montar com um caminho base no upstream (ex.: "/api").
func montarComBase(t *testing.T, upstream http.Handler, cfg ConfigGravador, base string) *ambiente {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	u, err := url.Parse(up.URL + base)
	if err != nil {
		t.Fatal(err)
	}
	captura := &bytes.Buffer{}
	cfg.Upstream = u
	cfg.Saida = NovoEscritor(captura)
	g, err := NovoGravador(cfg)
	if err != nil {
		t.Fatalf("NovoGravador: %v", err)
	}
	cliente := httptest.NewServer(g)
	t.Cleanup(cliente.Close)
	return &ambiente{upstream: up, cliente: cliente, captura: captura, g: g}
}

// encerrar espera as requisições abertas terminarem e fecha o gravador.
func (a *ambiente) encerrar(t *testing.T) {
	t.Helper()
	a.cliente.Close()
	if err := a.g.Fechar(); err != nil {
		t.Fatalf("Fechar: %v", err)
	}
}

// trocas encerra o ambiente e relê a captura.
func (a *ambiente) trocas(t *testing.T) []Troca {
	t.Helper()
	a.encerrar(t)
	return lerTrocas(t, a.captura)
}

func lerTrocas(t *testing.T, r io.Reader) []Troca {
	t.Helper()
	l := NovoLeitor(r)
	var trocas []Troca
	for {
		tr, err := l.Proxima()
		if errors.Is(err, io.EOF) {
			return trocas
		}
		if err != nil {
			t.Fatalf("captura inválida: %v", err)
		}
		trocas = append(trocas, tr)
	}
}

// pegar faz um GET e devolve status e corpo.
func pegar(t *testing.T, endereco string) (int, string) {
	t.Helper()
	resp, err := http.Get(endereco)
	if err != nil {
		t.Fatalf("GET %s: %v", endereco, err)
	}
	defer resp.Body.Close()
	corpo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: ler corpo: %v", endereco, err)
	}
	return resp.StatusCode, string(corpo)
}

func ok(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "ok "+r.URL.Path)
}

func TestNovoGravadorValida(t *testing.T) {
	up, _ := url.Parse("http://localhost:8080")
	saida := NovoEscritor(io.Discard)
	casos := []struct {
		nome string
		cfg  ConfigGravador
		erro bool
	}{
		{"válido", ConfigGravador{Upstream: up, Saida: saida, Amostra: 1}, false},
		{"amostra zero é válida", ConfigGravador{Upstream: up, Saida: saida}, false},
		{"sem upstream", ConfigGravador{Saida: saida, Amostra: 1}, true},
		{"upstream sem esquema http", ConfigGravador{Upstream: &url.URL{Scheme: "ftp", Host: "x"}, Saida: saida, Amostra: 1}, true},
		{"upstream sem host", ConfigGravador{Upstream: &url.URL{Scheme: "http"}, Saida: saida, Amostra: 1}, true},
		{"sem saída", ConfigGravador{Upstream: up, Amostra: 1}, true},
		{"amostra negativa", ConfigGravador{Upstream: up, Saida: saida, Amostra: -0.1}, true},
		{"amostra acima de 1", ConfigGravador{Upstream: up, Saida: saida, Amostra: 1.1}, true},
		{"amostra NaN", ConfigGravador{Upstream: up, Saida: saida, Amostra: math.NaN()}, true},
		{"max negativo", ConfigGravador{Upstream: up, Saida: saida, Amostra: 1, Max: -1}, true},
		{"max-body negativo", ConfigGravador{Upstream: up, Saida: saida, Amostra: 1, MaxCorpo: -1}, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := NovoGravador(c.cfg)
			if c.erro && err == nil {
				t.Error("esperado erro")
			}
			if !c.erro && err != nil {
				t.Errorf("erro inesperado: %v", err)
			}
		})
	}
}

// TestGravaGETePOST é o teste de ponta a ponta do critério de pronto.
func TestGravaGETePOST(t *testing.T) {
	var mu sync.Mutex
	var auths, chaves []string
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		chaves = append(chaves, r.Header.Get("X-Api-Key"))
		mu.Unlock()
		corpo, _ := io.ReadAll(r.Body)
		w.Header().Add("Set-Cookie", "s=sessao-secreta-1")
		w.Header().Add("Set-Cookie", "t=sessao-secreta-2")
		w.Header().Set("X-Api-Key", "segredo-resp")
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		if len(corpo) == 0 {
			corpo = []byte(`{"pedido":42}`)
		}
		w.Write(corpo)
	})
	a := montar(t, upstream, ConfigGravador{Amostra: 1})

	// GET com query e cabeçalhos sensíveis em grafias variadas.
	req, _ := http.NewRequest(http.MethodGet, a.cliente.URL+"/pedidos/42?expand=itens&x=1", nil)
	req.Header.Set("Authorization", "Bearer token-secreto-abc")
	req.Header["cookie"] = []string{"a=cookie-secreto"}
	req.Header["X-API-KEY"] = []string{"chave-secreta-k1"}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	corpoGET, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(corpoGET) != `{"pedido":42}` {
		t.Fatalf("GET: status %d corpo %q", resp.StatusCode, corpoGET)
	}
	if got := resp.Header.Values("Set-Cookie"); !reflect.DeepEqual(got, []string{"s=sessao-secreta-1", "t=sessao-secreta-2"}) {
		t.Errorf("cliente recebeu Set-Cookie %v, esperado os originais", got)
	}
	if got := resp.Header.Get("X-Api-Key"); got != "segredo-resp" {
		t.Errorf("cliente recebeu X-Api-Key %q, esperado o original", got)
	}

	// POST com corpo.
	corpoPOST := `{"total":10}`
	req, _ = http.NewRequest(http.MethodPost, a.cliente.URL+"/pedidos", strings.NewReader(corpoPOST))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token-secreto-abc")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ecoado, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 201 || string(ecoado) != corpoPOST {
		t.Fatalf("POST: status %d corpo %q", resp.StatusCode, ecoado)
	}

	mu.Lock()
	if auths[0] != "Bearer token-secreto-abc" || chaves[0] != "chave-secreta-k1" {
		t.Errorf("upstream recebeu Authorization %q e X-Api-Key %q, esperado os originais", auths[0], chaves[0])
	}
	mu.Unlock()

	a.encerrar(t)
	cru := a.captura.String()
	for _, segredo := range []string{"secreto", "secreta", "segredo"} {
		if strings.Contains(cru, segredo) {
			t.Errorf("captura contém %q:\n%s", segredo, cru)
		}
	}
	trocas := lerTrocas(t, strings.NewReader(cru))
	if len(trocas) != 2 {
		t.Fatalf("esperadas 2 trocas, vieram %d", len(trocas))
	}
	if a.g.Gravadas() != 2 {
		t.Errorf("Gravadas() = %d", a.g.Gravadas())
	}

	host := strings.TrimPrefix(a.cliente.URL, "http://")
	for i, tr := range trocas {
		if tr.LatenciaMs <= 0 {
			t.Errorf("troca %d: latencia_ms = %v", i, tr.LatenciaMs)
		}
		if tr.Horario.IsZero() {
			t.Errorf("troca %d: horario zero", i)
		}
		if got := tr.CabecalhosRequisicao.Values("Authorization"); !reflect.DeepEqual(got, []string{"***"}) {
			t.Errorf("troca %d: Authorization gravado %v", i, got)
		}
		if got := tr.CabecalhosRequisicao.Get("Host"); got != host {
			t.Errorf("troca %d: Host gravado %q, esperado %q", i, got, host)
		}
		if got := tr.CabecalhosResposta.Values("Set-Cookie"); !reflect.DeepEqual(got, []string{"***", "***"}) {
			t.Errorf("troca %d: Set-Cookie gravado %v", i, got)
		}
		if got := tr.CabecalhosResposta.Values("X-Api-Key"); !reflect.DeepEqual(got, []string{"***"}) {
			t.Errorf("troca %d: X-Api-Key da resposta gravado %v", i, got)
		}
		if got := tr.CabecalhosResposta.Get("Content-Type"); got != "application/json" {
			t.Errorf("troca %d: Content-Type gravado %q", i, got)
		}
	}

	get, post := trocas[0], trocas[1]
	if get.Metodo != "GET" || get.Caminho != "/pedidos/42" || get.Query != "expand=itens&x=1" {
		t.Errorf("GET gravado como %s %s ? %s", get.Metodo, get.Caminho, get.Query)
	}
	if get.Status != 200 || string(get.CorpoResposta) != `{"pedido":42}` || len(get.CorpoRequisicao) != 0 {
		t.Errorf("GET gravado com status %d, corpo req %q, corpo resp %q", get.Status, get.CorpoRequisicao, get.CorpoResposta)
	}
	if got := get.CabecalhosRequisicao.Values("Cookie"); !reflect.DeepEqual(got, []string{"***"}) {
		t.Errorf("Cookie gravado %v", got)
	}
	if got := get.CabecalhosRequisicao.Values("X-Api-Key"); !reflect.DeepEqual(got, []string{"***"}) {
		t.Errorf("X-Api-Key da requisição gravado %v", got)
	}
	if post.Metodo != "POST" || post.Caminho != "/pedidos" || post.Query != "" {
		t.Errorf("POST gravado como %s %s ? %s", post.Metodo, post.Caminho, post.Query)
	}
	if post.Status != 201 || string(post.CorpoRequisicao) != corpoPOST || string(post.CorpoResposta) != corpoPOST {
		t.Errorf("POST gravado com status %d, corpo req %q, corpo resp %q", post.Status, post.CorpoRequisicao, post.CorpoResposta)
	}
	if post.CorpoRequisicaoTruncado || post.CorpoRespostaTruncado {
		t.Error("POST pequeno marcado como truncado")
	}
	if got := post.CabecalhosRequisicao.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type da requisição gravado %q", got)
	}
}

func TestUpstreamComCaminhoBase(t *testing.T) {
	var mu sync.Mutex
	var recebido string
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		recebido = r.URL.Path + "?" + r.URL.RawQuery
		mu.Unlock()
		io.WriteString(w, "ok")
	})
	a := montarComBase(t, upstream, ConfigGravador{Amostra: 1}, "/api")
	if st, _ := pegar(t, a.cliente.URL+"/orders?x=1"); st != 200 {
		t.Fatalf("status %d", st)
	}
	trocas := a.trocas(t)
	mu.Lock()
	defer mu.Unlock()
	if recebido != "/api/orders?x=1" {
		t.Errorf("upstream recebeu %q, esperado /api/orders?x=1", recebido)
	}
	if len(trocas) != 1 || trocas[0].Caminho != "/orders" || trocas[0].Query != "x=1" {
		t.Errorf("captura: %+v", trocas)
	}
}

func TestSemWriteHeaderESemCorpo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/so-write", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/vazio", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/head", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "corpo que o HEAD não leva")
	})
	a := montar(t, mux, ConfigGravador{Amostra: 1})

	if st, corpo := pegar(t, a.cliente.URL+"/so-write"); st != 200 || corpo != "ok" {
		t.Fatalf("/so-write: %d %q", st, corpo)
	}
	if st, _ := pegar(t, a.cliente.URL+"/vazio"); st != 204 {
		t.Fatalf("/vazio: %d", st)
	}
	resp, err := http.Head(a.cliente.URL + "/head")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	a.encerrar(t)
	linhas := strings.Split(strings.TrimSpace(a.captura.String()), "\n")
	if len(linhas) != 3 {
		t.Fatalf("esperadas 3 linhas, vieram %d", len(linhas))
	}
	trocas := lerTrocas(t, strings.NewReader(a.captura.String()))
	if trocas[0].Status != 200 || string(trocas[0].CorpoResposta) != "ok" {
		t.Errorf("/so-write gravado: %d %q", trocas[0].Status, trocas[0].CorpoResposta)
	}
	if trocas[1].Status != 204 || len(trocas[1].CorpoResposta) != 0 {
		t.Errorf("/vazio gravado: %d %q", trocas[1].Status, trocas[1].CorpoResposta)
	}
	if trocas[2].Metodo != "HEAD" || trocas[2].Status != 200 || len(trocas[2].CorpoResposta) != 0 {
		t.Errorf("HEAD gravado: %s %d %q", trocas[2].Metodo, trocas[2].Status, trocas[2].CorpoResposta)
	}
	for _, i := range []int{1, 2} {
		if !strings.Contains(linhas[i], `"corpo_resposta":""`) {
			t.Errorf("linha %d sem \"corpo_resposta\":\"\": %s", i+1, linhas[i])
		}
	}
}

func TestAmostraZero(t *testing.T) {
	a := montar(t, http.HandlerFunc(ok), ConfigGravador{Amostra: 0})
	for i := 0; i < 5; i++ {
		if st, corpo := pegar(t, a.cliente.URL+"/a"); st != 200 || corpo != "ok /a" {
			t.Fatalf("requisição %d: %d %q", i, st, corpo)
		}
	}
	if trocas := a.trocas(t); len(trocas) != 0 {
		t.Errorf("esperada captura vazia, vieram %d trocas", len(trocas))
	}
	if a.g.Gravadas() != 0 {
		t.Errorf("Gravadas() = %d", a.g.Gravadas())
	}
}

func TestAmostraSorteio(t *testing.T) {
	var mu sync.Mutex
	sorteios := []float64{0.1, 0.9, 0.4, 0.6}
	sortear := func() float64 {
		mu.Lock()
		defer mu.Unlock()
		v := sorteios[0]
		sorteios = sorteios[1:]
		return v
	}
	a := montar(t, http.HandlerFunc(ok), ConfigGravador{Amostra: 0.5, Sorteio: sortear})
	for i := 1; i <= 4; i++ {
		pegar(t, fmt.Sprintf("%s/r%d", a.cliente.URL, i))
	}
	trocas := a.trocas(t)
	var caminhos []string
	for _, tr := range trocas {
		caminhos = append(caminhos, tr.Caminho)
	}
	if !reflect.DeepEqual(caminhos, []string{"/r1", "/r3"}) {
		t.Errorf("gravados %v, esperado [/r1 /r3]", caminhos)
	}
}

func TestMax(t *testing.T) {
	a := montar(t, http.HandlerFunc(ok), ConfigGravador{Amostra: 1, Max: 2})
	for i := 1; i <= 5; i++ {
		caminho := fmt.Sprintf("/m%d", i)
		if st, corpo := pegar(t, a.cliente.URL+caminho); st != 200 || corpo != "ok "+caminho {
			t.Fatalf("requisição %d: %d %q", i, st, corpo)
		}
	}
	trocas := a.trocas(t)
	if len(trocas) != 2 || trocas[0].Caminho != "/m1" || trocas[1].Caminho != "/m2" {
		t.Errorf("esperadas /m1 e /m2, veio %+v", trocas)
	}
	if a.g.Gravadas() != 2 {
		t.Errorf("Gravadas() = %d", a.g.Gravadas())
	}
}

func TestMaxCorpo(t *testing.T) {
	var mu sync.Mutex
	var recebidos []int
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corpo, _ := io.ReadAll(r.Body)
		mu.Lock()
		recebidos = append(recebidos, len(corpo))
		mu.Unlock()
		if len(corpo) == 25 {
			io.WriteString(w, "012345678901234567890123456789") // 30 bytes
			return
		}
		w.Write(corpo)
	})
	a := montar(t, upstream, ConfigGravador{Amostra: 1, MaxCorpo: 10})

	enviar := func(corpo string) string {
		resp, err := http.Post(a.cliente.URL+"/upload", "text/plain", strings.NewReader(corpo))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	grande := "abcdefghijklmnopqrstuvwxy" // 25 bytes
	if got := enviar(grande); got != "012345678901234567890123456789" {
		t.Errorf("cliente recebeu %q, esperado os 30 bytes inteiros", got)
	}
	if got := enviar("0123456789"); got != "0123456789" {
		t.Errorf("cliente recebeu %q", got)
	}
	trocas := a.trocas(t)
	mu.Lock()
	if !reflect.DeepEqual(recebidos, []int{25, 10}) {
		t.Errorf("upstream recebeu corpos de %v bytes, esperado [25 10]", recebidos)
	}
	mu.Unlock()
	if len(trocas) != 2 {
		t.Fatalf("esperadas 2 trocas, vieram %d", len(trocas))
	}
	cortada, exata := trocas[0], trocas[1]
	if string(cortada.CorpoRequisicao) != "abcdefghij" || !cortada.CorpoRequisicaoTruncado {
		t.Errorf("requisição gravada %q truncado=%v", cortada.CorpoRequisicao, cortada.CorpoRequisicaoTruncado)
	}
	if string(cortada.CorpoResposta) != "0123456789" || !cortada.CorpoRespostaTruncado {
		t.Errorf("resposta gravada %q truncado=%v", cortada.CorpoResposta, cortada.CorpoRespostaTruncado)
	}
	if string(exata.CorpoRequisicao) != "0123456789" || exata.CorpoRequisicaoTruncado ||
		string(exata.CorpoResposta) != "0123456789" || exata.CorpoRespostaTruncado {
		t.Errorf("corpo de exatamente 10 bytes: %+v", exata)
	}
}

func TestUpstreamFora(t *testing.T) {
	a := montar(t, http.HandlerFunc(ok), ConfigGravador{Amostra: 1})
	a.upstream.Close()
	if st, _ := pegar(t, a.cliente.URL+"/x"); st != http.StatusBadGateway {
		t.Errorf("status %d, esperado 502", st)
	}
	if trocas := a.trocas(t); len(trocas) != 0 {
		t.Errorf("falha do proxy gravada: %+v", trocas)
	}
	if a.g.Gravadas() != 0 {
		t.Errorf("Gravadas() = %d", a.g.Gravadas())
	}
}

type transporteQueFalhaUmaVez struct{ chamadas atomic.Int32 }

func (tf *transporteQueFalhaUmaVez) RoundTrip(r *http.Request) (*http.Response, error) {
	if tf.chamadas.Add(1) == 1 {
		return nil, errors.New("falha simulada")
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestFalhaDevolveVaga(t *testing.T) {
	a := montar(t, http.HandlerFunc(ok), ConfigGravador{Amostra: 1, Max: 1, Transport: &transporteQueFalhaUmaVez{}})
	if st, _ := pegar(t, a.cliente.URL+"/falha"); st != http.StatusBadGateway {
		t.Fatalf("1ª requisição: status %d, esperado 502", st)
	}
	if st, _ := pegar(t, a.cliente.URL+"/boa"); st != 200 {
		t.Fatalf("2ª requisição: status %d", st)
	}
	trocas := a.trocas(t)
	if len(trocas) != 1 || trocas[0].Caminho != "/boa" {
		t.Errorf("esperada só /boa, veio %+v", trocas)
	}
}

func TestTrocaInterrompida(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/cortada", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(200)
		w.Write(make([]byte, 10))
		rc := http.NewResponseController(w)
		rc.Flush()
		conn, _, err := rc.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		conn.Close()
	})
	mux.HandleFunc("/normal", ok)
	a := montar(t, mux, ConfigGravador{Amostra: 1, Max: 1})

	if resp, err := http.Get(a.cliente.URL + "/cortada"); err == nil {
		io.ReadAll(resp.Body) // erro de leitura esperado; ignorado
		resp.Body.Close()
	}
	if st, _ := pegar(t, a.cliente.URL+"/normal"); st != 200 {
		t.Fatalf("/normal: status %d", st)
	}
	trocas := a.trocas(t)
	if len(trocas) != 1 || trocas[0].Caminho != "/normal" {
		t.Errorf("esperada só /normal, veio %+v", trocas)
	}
}

func TestClienteDesconectou(t *testing.T) {
	chegou := make(chan struct{})
	liberar := make(chan struct{})
	var uma sync.Once
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uma.Do(func() { close(chegou) })
		<-liberar
		io.WriteString(w, "tarde demais")
	})
	a := montar(t, upstream, ConfigGravador{Amostra: 1})

	ctx, cancelar := context.WithCancel(context.Background())
	go func() {
		<-chegou
		cancelar()
	}()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.cliente.URL+"/lenta", nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
		t.Error("esperado erro de contexto cancelado no cliente")
	}
	close(liberar)
	if trocas := a.trocas(t); len(trocas) != 0 {
		t.Errorf("troca de cliente desconectado gravada: %+v", trocas)
	}
}

func TestSSEFlush(t *testing.T) {
	segunda := make(chan struct{})
	var fecharSegunda sync.Once
	soltar := func() { fecharSegunda.Do(func() { close(segunda) }) }
	defer soltar()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		io.WriteString(w, "data: 1\n\n")
		w.(http.Flusher).Flush()
		<-segunda
		io.WriteString(w, "data: 2\n\n")
	})
	a := montar(t, upstream, ConfigGravador{Amostra: 1})

	resp, err := http.Get(a.cliente.URL + "/eventos")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	br := bufio.NewReader(resp.Body)
	primeira := make(chan string, 1)
	go func() {
		linha, _ := br.ReadString('\n')
		primeira <- linha
	}()
	select {
	case linha := <-primeira:
		if linha != "data: 1\n" {
			t.Fatalf("primeira linha %q", linha)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a primeira mensagem não chegou antes do fim: o flush não atravessou o gravador")
	}
	soltar()
	resto, err := io.ReadAll(br)
	if err != nil {
		t.Fatal(err)
	}
	if string(resto) != "\ndata: 2\n\n" {
		t.Errorf("resto %q", resto)
	}
	resp.Body.Close()
	trocas := a.trocas(t)
	if len(trocas) != 1 || string(trocas[0].CorpoResposta) != "data: 1\n\ndata: 2\n\n" {
		t.Errorf("captura: %+v", trocas)
	}
}

func TestConcorrencia(t *testing.T) {
	a := montar(t, http.HandlerFunc(ok), ConfigGravador{Amostra: 1})
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := http.Get(fmt.Sprintf("%s/c/%d", a.cliente.URL, i))
			if err != nil {
				t.Error(err)
				return
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}(i)
	}
	wg.Wait()
	trocas := a.trocas(t)
	var obtidos, esperados []string
	for _, tr := range trocas {
		obtidos = append(obtidos, tr.Caminho)
	}
	for i := 0; i < n; i++ {
		esperados = append(esperados, fmt.Sprintf("/c/%d", i))
	}
	sort.Strings(obtidos)
	sort.Strings(esperados)
	if !reflect.DeepEqual(obtidos, esperados) {
		t.Errorf("caminhos gravados (%d) diferentes dos enviados (%d)", len(obtidos), len(esperados))
	}
}

func TestMaxConcorrente(t *testing.T) {
	a := montar(t, http.HandlerFunc(ok), ConfigGravador{Amostra: 1, Max: 3})
	const n = 20
	var wg sync.WaitGroup
	var sucesso atomic.Int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := http.Get(fmt.Sprintf("%s/p/%d", a.cliente.URL, i))
			if err != nil {
				t.Error(err)
				return
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				sucesso.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if sucesso.Load() != n {
		t.Errorf("%d de %d requisições com 200", sucesso.Load(), n)
	}
	if trocas := a.trocas(t); len(trocas) != 3 {
		t.Errorf("esperadas 3 trocas, vieram %d", len(trocas))
	}
	if a.g.Gravadas() != 3 {
		t.Errorf("Gravadas() = %d", a.g.Gravadas())
	}
}

func TestFechar(t *testing.T) {
	a := montar(t, http.HandlerFunc(ok), ConfigGravador{Amostra: 1})
	if err := a.g.Fechar(); err != nil {
		t.Fatal(err)
	}
	if st, corpo := pegar(t, a.cliente.URL+"/depois"); st != 200 || corpo != "ok /depois" {
		t.Errorf("depois de Fechar: %d %q", st, corpo)
	}
	a.cliente.Close()
	if a.captura.Len() != 0 {
		t.Errorf("gravou depois de Fechar: %s", a.captura.String())
	}
	if a.g.Gravadas() != 0 {
		t.Errorf("Gravadas() = %d", a.g.Gravadas())
	}
	if err := a.g.Erro(); err != nil {
		t.Errorf("Erro() = %v, esperado nil", err)
	}
}

func TestMascararNaoAlteraOriginal(t *testing.T) {
	h := http.Header{"Authorization": {"a"}, "Set-Cookie": {"x", "y"}, "Accept": {"*/*"}}
	m := mascarar(h)
	if h.Get("Authorization") != "a" || !reflect.DeepEqual(h.Values("Set-Cookie"), []string{"x", "y"}) {
		t.Errorf("original alterado: %v", h)
	}
	if !reflect.DeepEqual(m.Values("Set-Cookie"), []string{"***", "***"}) || m.Get("Accept") != "*/*" {
		t.Errorf("máscara errada: %v", m)
	}
	if mascarar(nil) == nil {
		t.Error("mascarar(nil) deveria devolver Header vazio")
	}
}
