package motor

import (
	"context"
	"errors"
	"io"
	"log"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// MaxCorpoPadrao é o limite de bytes gravados por corpo quando
// ConfigGravador.MaxCorpo é 0: 1 MiB.
const MaxCorpoPadrao = 1 << 20

// ConfigGravador configura o Gravador.
type ConfigGravador struct {
	// Upstream é o serviço que recebe o tráfego (obrigatório), ex.:
	// http://localhost:8080 ou http://h/api (o caminho base é prefixado).
	Upstream *url.URL
	// Saida recebe as trocas gravadas (obrigatório).
	Saida *Escritor
	// Amostra é a fração de trocas gravadas, de 0 a 1. Com 0 nada é gravado;
	// com 1 tudo é. Fora desse intervalo é erro.
	Amostra float64
	// Max é o número máximo de trocas gravadas; 0 = sem limite. Ao chegar no
	// máximo o gravador continua repassando, mas para de gravar.
	Max int
	// MaxCorpo é o limite de bytes gravados de cada corpo; 0 = MaxCorpoPadrao.
	// Acima dele o cliente e o upstream recebem tudo, mas a troca é gravada
	// com o corpo cortado e marcada como truncada.
	MaxCorpo int64
	// Sorteio devolve um número em [0, 1) para a amostragem. Opcional (para
	// testes); o padrão é math/rand/v2.
	Sorteio func() float64
	// Transport usado para falar com o upstream. Opcional; o padrão é
	// http.DefaultTransport.
	Transport http.RoundTripper
}

// Gravador é um proxy reverso (http.Handler) que repassa cada requisição ao
// upstream e grava a troca na Saida.
//
// O cliente sempre recebe a resposta original, sem máscara e sem corte. Não
// são gravadas: as falhas do próprio proxy (ex.: 502 com o upstream fora),
// as trocas interrompidas no meio, as requisições cujo cliente desconectou
// e as conexões sequestradas (upgrade para WebSocket).
type Gravador struct {
	cfg      ConfigGravador
	proxy    *httputil.ReverseProxy
	vagas    atomic.Int64 // trocas reservadas (em andamento ou gravadas)
	gravadas atomic.Int64

	muErro sync.Mutex
	erro   error
}

// chaveFalha marca no contexto da requisição que o proxy falhou.
type chaveFalha struct{}

// NovoGravador valida a configuração e monta o gravador.
func NovoGravador(cfg ConfigGravador) (*Gravador, error) {
	if cfg.Upstream == nil {
		return nil, errors.New("motor: upstream obrigatório")
	}
	if cfg.Upstream.Scheme != "http" && cfg.Upstream.Scheme != "https" {
		return nil, errors.New("motor: upstream precisa ser http:// ou https://")
	}
	if cfg.Upstream.Host == "" {
		return nil, errors.New("motor: upstream sem host")
	}
	if cfg.Saida == nil {
		return nil, errors.New("motor: saída obrigatória")
	}
	if math.IsNaN(cfg.Amostra) || cfg.Amostra < 0 || cfg.Amostra > 1 {
		return nil, errors.New("motor: amostra precisa estar entre 0 e 1")
	}
	if cfg.Max < 0 {
		return nil, errors.New("motor: max não pode ser negativo")
	}
	if cfg.MaxCorpo < 0 {
		return nil, errors.New("motor: max-body não pode ser negativo")
	}
	if cfg.MaxCorpo == 0 {
		cfg.MaxCorpo = MaxCorpoPadrao
	}
	if cfg.Sorteio == nil {
		cfg.Sorteio = rand.Float64
	}
	up := *cfg.Upstream
	g := &Gravador{cfg: cfg}
	g.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&up)
			pr.SetXForwarded()
		},
		Transport: cfg.Transport,
		// O motor não escreve no terminal: o log interno do proxy (ex.:
		// "read error during body copy") é descartado.
		ErrorLog: log.New(io.Discard, "", 0),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if falha, ok := r.Context().Value(chaveFalha{}).(*atomic.Bool); ok {
				falha.Store(true)
			}
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	return g, nil
}

// Gravadas devolve quantas trocas foram efetivamente escritas na Saida.
func (g *Gravador) Gravadas() int { return int(g.gravadas.Load()) }

// Erro devolve o primeiro erro de escrita na Saida, ou nil. Um erro de
// escrita não derruba o proxy: o tráfego continua sendo repassado.
func (g *Gravador) Erro() error {
	g.muErro.Lock()
	defer g.muErro.Unlock()
	return g.erro
}

// Fechar fecha a Saida (flush). Depois disso o gravador só repassa.
func (g *Gravador) Fechar() error { return g.cfg.Saida.Fechar() }

func (g *Gravador) registrarErro(err error) {
	g.muErro.Lock()
	defer g.muErro.Unlock()
	if g.erro == nil {
		g.erro = err
	}
}

// reservar ocupa uma vaga do Max; sem limite, sempre consegue.
func (g *Gravador) reservar() bool {
	if g.cfg.Max == 0 {
		g.vagas.Add(1)
		return true
	}
	for {
		atual := g.vagas.Load()
		if atual >= int64(g.cfg.Max) {
			return false
		}
		if g.vagas.CompareAndSwap(atual, atual+1) {
			return true
		}
	}
}

func (g *Gravador) liberar() { g.vagas.Add(-1) }

// ServeHTTP repassa a requisição ao upstream e grava a troca.
func (g *Gravador) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	falha := new(atomic.Bool)
	r = r.WithContext(context.WithValue(r.Context(), chaveFalha{}, falha))

	if !(g.cfg.Sorteio() < g.cfg.Amostra) || !g.reservar() {
		g.proxy.ServeHTTP(w, r)
		return
	}

	gravou := false
	// Em qualquer saída sem gravação (inclusive panic do proxy, como
	// http.ErrAbortHandler numa troca interrompida), a vaga volta.
	defer func() {
		if !gravou {
			g.liberar()
		}
	}()

	inicio := time.Now()
	cabReq := r.Header.Clone()
	if cabReq == nil {
		cabReq = http.Header{}
	}
	cabReq.Set("Host", r.Host)
	metodo, caminho, query := r.Method, r.URL.Path, r.URL.RawQuery

	copiaReq := &copiaLimitada{limite: g.cfg.MaxCorpo}
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = &corpoTee{ReadCloser: r.Body, copia: copiaReq}
	}
	rw := &respostaGravada{w: w, corpo: &copiaLimitada{limite: g.cfg.MaxCorpo}}

	g.proxy.ServeHTTP(rw, r)

	if falha.Load() || r.Context().Err() != nil || rw.sequestrou || !rw.escreveu {
		return
	}
	latencia := float64(time.Since(inicio)) / float64(time.Millisecond)
	corpoReq, reqTruncado := copiaReq.conteudo()
	corpoResp, respTruncado := rw.corpo.conteudo()
	troca := Troca{
		Horario:                 inicio.UTC(),
		Metodo:                  metodo,
		Caminho:                 caminho,
		Query:                   query,
		CabecalhosRequisicao:    mascarar(cabReq),
		CorpoRequisicao:         corpoReq,
		CorpoRequisicaoTruncado: reqTruncado,
		Status:                  rw.status,
		CabecalhosResposta:      mascarar(rw.cabecalhos),
		CorpoResposta:           corpoResp,
		CorpoRespostaTruncado:   respTruncado,
		LatenciaMs:              latencia,
	}
	if err := g.cfg.Saida.Escrever(troca); err != nil {
		if !errors.Is(err, ErrEscritorFechado) {
			g.registrarErro(err)
		}
		return
	}
	gravou = true
	g.gravadas.Add(1)
}
