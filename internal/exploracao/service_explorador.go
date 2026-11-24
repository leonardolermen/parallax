package exploracao

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/leonardolermen/parallax/pkg/captura"
)

// ExcluirPadrao são os caminhos que a exploração nunca visita: um segmento
// que encerra sessão ou apaga algo, ou um último segmento que é uma ação
// (/alerts/3/toggle, /tasks/9/fix). Mesmo em GET, um handler que não confere
// o método executaria a ação, e base e candidata podem dividir o mesmo banco.
const ExcluirPadrao = `(?i)((^|/)(logout|log-out|signout|sign-out|sair|shutdown|desligar|delete|remove|apagar|excluir|reset|destroy|purge|kill)([-_.][^/]*)?(/|$)` +
	`|/(toggle|fix|fixci|run|rodar|start|stop|restart|approve|aprovar|reject|recusar|merge|deploy|send|enviar|reply|responder|execute|executar|trigger|disparar|cancel|cancelar|analy[sz]e|analisar|ligar|limpar|clear|import|importar|sync|publish|publicar|upload|retry|rerun|archive|arquivar|accept|aceitar|confirm|confirmar|pay|pagar)([-_][^/]*)?$)`

// sondas são caminhos que costumam publicar a lista de rotas ou a saúde do
// serviço. Só entram na captura se responderem 2xx.
var sondas = []string{"/openapi.json", "/swagger.json", "/v3/api-docs", "/api-docs", "/swagger/v1/swagger.json", "/health", "/healthz"}

// extensoesEstaticas não são lidas nem gravadas (sem Estaticos), exceto
// JavaScript, que é lido para achar as chamadas à API mas não é gravado.
var extensoesEstaticas = map[string]bool{
	".css": true, ".map": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".ico": true, ".woff": true, ".woff2": true, ".ttf": true, ".eot": true, ".otf": true, ".webp": true,
	".avif": true, ".mp4": true, ".webm": true, ".mp3": true, ".pdf": true, ".zip": true, ".wasm": true,
	".js": true, ".mjs": true, ".cjs": true,
}

// Config é a configuração de uma exploração.
type Config struct {
	// Alvo é a URL do serviço rodando (a base). Um caminho na URL vira
	// prefixo: os caminhos gravados ficam relativos a ele, como no replay.
	Alvo string
	// Fontes são pastas com o código do serviço; vazio = só as respostas.
	Fontes []string
	// Max é o máximo de trocas gravadas.
	Max int
	// PorRota é o máximo de trocas por modelo ("/pedidos/{id}") ou por
	// formato de caminho (/pedidos/1, /pedidos/2...).
	PorRota int
	// Timeout por requisição.
	Timeout time.Duration
	// MaxCorpo é quanto de cada resposta é lido e gravado.
	MaxCorpo int64
	// Cabecalhos vão em cada requisição (credencial de teste, por exemplo);
	// os sensíveis ficam mascarados na captura.
	Cabecalhos http.Header
	// Excluir: caminhos que casam não são visitados.
	Excluir *regexp.Regexp
	// Estaticos grava também CSS, imagens, fontes e JavaScript.
	Estaticos bool
	// Cliente HTTP; nil = um padrão que não segue redirecionamentos.
	Cliente *http.Client
}

// ConfigPadrao devolve os valores padrão.
func ConfigPadrao() Config {
	return Config{Max: 300, PorRota: 3, Timeout: 10 * time.Second, MaxCorpo: 1 << 20,
		Excluir: regexp.MustCompile(ExcluirPadrao)}
}

// Resumo diz o que a exploração achou.
type Resumo struct {
	Gravadas    int
	RotasCodigo int
	Modelos     int
	SemValor    []string // modelos que ficaram sem valor para algum parâmetro
	Visitadas   int
	Descartadas int // 404/405 de caminho adivinhado, SSE, timeout, erro
	ComVolateis int // trocas com campos que mudam sozinhos (não comparados)
}

type item struct {
	caminho, query, origem string
	modelo                 *Rota
}

type explorador struct {
	cfg       Config
	alvo      *url.URL
	prefixo   string
	cliente   *http.Client
	fila      []item
	vistos    map[string]bool
	porForma  map[string]int
	modelos   []Rota
	porModelo map[string]int
	vals      *valores
	trocas    []captura.Troca
	resumo    Resumo
}

// Explorar descobre as rotas GET do alvo e devolve as trocas, prontas para
// gravar como captura.
func Explorar(ctx context.Context, cfg Config) ([]captura.Troca, Resumo, error) {
	alvo, err := url.Parse(strings.TrimRight(cfg.Alvo, "/"))
	if err != nil || (alvo.Scheme != "http" && alvo.Scheme != "https") || alvo.Host == "" {
		return nil, Resumo{}, fmt.Errorf("explorar: --target precisa ser uma URL http(s) absoluta")
	}
	if cfg.Max <= 0 || cfg.PorRota <= 0 || cfg.Timeout <= 0 || cfg.MaxCorpo <= 0 {
		return nil, Resumo{}, fmt.Errorf("explorar: max, per-route, timeout e max-body devem ser positivos")
	}
	e := &explorador{cfg: cfg, alvo: alvo, prefixo: alvo.Path, cliente: cfg.Cliente,
		vistos: map[string]bool{}, porForma: map[string]int{}, porModelo: map[string]int{}, vals: novosValores()}
	if e.cliente == nil {
		e.cliente = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	rotas, err := LerRotasDoCodigo(cfg.Fontes)
	if err != nil {
		return nil, Resumo{}, fmt.Errorf("explorar: ler o código: %w", err)
	}
	e.resumo.RotasCodigo = len(rotas)

	e.enfileirar(item{caminho: "/", origem: OrigemInicio})
	for _, r := range rotas {
		e.registrarRota(r)
	}
	for _, s := range sondas {
		e.enfileirar(item{caminho: s, origem: OrigemSonda})
	}

	limiteVisitas := cfg.Max * 4
	for rodada := 0; rodada < 6; rodada++ {
		for len(e.fila) > 0 && len(e.trocas) < cfg.Max && e.resumo.Visitadas < limiteVisitas {
			if err := ctx.Err(); err != nil {
				return nil, e.resumo, err
			}
			it := e.fila[0]
			e.fila = e.fila[1:]
			e.visitar(ctx, it)
		}
		if len(e.trocas) >= cfg.Max || e.resumo.Visitadas >= limiteVisitas || !e.preencherModelos() {
			break
		}
	}
	e.marcarVolateis(ctx)
	if err := ctx.Err(); err != nil {
		return nil, e.resumo, err
	}
	e.resumo.Modelos = len(e.modelos)
	for _, m := range e.modelos {
		if e.cfg.Excluir != nil && e.cfg.Excluir.MatchString(m.Modelo()) {
			continue
		}
		if e.porModelo[m.Modelo()] == 0 && len(e.candidatosModelo(m)) == 0 {
			e.resumo.SemValor = append(e.resumo.SemValor, m.Modelo())
		}
	}
	e.resumo.Gravadas = len(e.trocas)
	return e.trocas, e.resumo, nil
}

// registrarRota põe uma rota concreta na fila e guarda um modelo para
// preencher depois.
func (e *explorador) registrarRota(r Rota) {
	if r.Concreta() {
		e.enfileirar(item{caminho: r.Modelo(), origem: r.Origem})
		return
	}
	m := r.Modelo()
	for _, x := range e.modelos {
		if x.Modelo() == m {
			return
		}
	}
	e.modelos = append(e.modelos, r)
}

// registrarAchado resolve um caminho citado numa resposta da página atual.
func (e *explorador) registrarAchado(pagina *url.URL, a achado) {
	texto := a.texto
	if strings.Contains(texto, "${") || strings.ContainsAny(texto, "{}<>") || (strings.Contains(texto, "/:") && a.origem != OrigemLink) {
		if r, ok := AnalisarModelo(e.relativo(texto)); ok {
			r.Origem = a.origem
			e.registrarRota(r)
		}
		return
	}
	ref, err := url.Parse(texto)
	if err != nil {
		return
	}
	u := pagina.ResolveReference(ref)
	if u.Scheme != e.alvo.Scheme || u.Host != e.alvo.Host {
		return
	}
	caminho := e.relativo(u.Path)
	if caminho == "" {
		return
	}
	e.enfileirar(item{caminho: caminho, query: u.RawQuery, origem: a.origem})
}

// relativo tira o prefixo do alvo; "" se o caminho estiver fora dele.
func (e *explorador) relativo(p string) string {
	pre := strings.TrimRight(e.prefixo, "/")
	if pre == "" {
		return p
	}
	if p == pre {
		return "/"
	}
	if !strings.HasPrefix(p, pre+"/") {
		return ""
	}
	return strings.TrimPrefix(p, pre)
}

func (e *explorador) enfileirar(it item) {
	chave := it.caminho + "?" + it.query
	if e.vistos[chave] || !strings.HasPrefix(it.caminho, "/") {
		return
	}
	if e.cfg.Excluir != nil && e.cfg.Excluir.MatchString(it.caminho) {
		return
	}
	ext := strings.ToLower(path.Ext(it.caminho))
	if extensoesEstaticas[ext] && !e.cfg.Estaticos && ext != ".js" && ext != ".mjs" && ext != ".cjs" {
		return
	}
	e.vistos[chave] = true
	e.fila = append(e.fila, it)
}

var reSegmentoVariavel = regexp.MustCompile(`^([0-9]+|[0-9a-fA-F-]{16,}|[0-9A-HJKMNP-TV-Z]{26})$`)

// forma troca os segmentos que parecem identificador por "{}": /pedidos/1 e
// /pedidos/2 têm a mesma forma.
func forma(caminho string) string {
	partes := dividir(caminho)
	for i, p := range partes {
		if reSegmentoVariavel.MatchString(p) {
			partes[i] = "{}"
		}
	}
	return "/" + strings.Join(partes, "/")
}

func (e *explorador) modeloDe(caminho string) string {
	for _, m := range e.modelos {
		if m.Casa(caminho) {
			return m.Modelo()
		}
	}
	return ""
}

func (e *explorador) visitar(ctx context.Context, it item) {
	f := forma(it.caminho)
	mod := e.modeloDe(it.caminho)
	if e.porForma[f] >= e.cfg.PorRota || (mod != "" && e.porModelo[mod] >= e.cfg.PorRota) {
		return
	}
	e.resumo.Visitadas++
	tr, ok := e.buscar(ctx, it)
	if !ok {
		e.resumo.Descartadas++
		return
	}
	pagina := &url.URL{Scheme: e.alvo.Scheme, Host: e.alvo.Host, Path: strings.TrimRight(e.prefixo, "/") + it.caminho, RawQuery: it.query}
	ct := tr.CabecalhosResposta.Get("Content-Type")
	tipo, _, _ := mime.ParseMediaType(ct)
	corpo := tr.CorpoResposta
	var achados []achado
	if loc := tr.CabecalhosResposta.Get("Location"); loc != "" {
		achados = append(achados, achado{texto: loc, origem: OrigemLink})
	}
	switch {
	case tipo == "text/html":
		achados = append(achados, extrairDoHTML(corpo)...)
	case strings.Contains(tipo, "javascript"):
		achados = append(achados, extrairDoScript(corpo)...)
	default:
		if v, ok := decodificarJSON(corpo); ok {
			e.vals.colher(it.caminho, v)
			achados = append(achados, extrairDoJSON(v)...)
		}
	}
	for _, a := range achados {
		e.registrarAchado(pagina, a)
	}

	ext := strings.ToLower(path.Ext(it.caminho))
	estatico := extensoesEstaticas[ext] || strings.Contains(tipo, "javascript") || tipo == "text/css" || strings.HasPrefix(tipo, "image/") || strings.HasPrefix(tipo, "font/")
	if estatico && !e.cfg.Estaticos {
		return
	}
	if adivinhada(it.origem) && (tr.Status == http.StatusNotFound || tr.Status == http.StatusMethodNotAllowed) {
		e.resumo.Descartadas++
		return
	}
	if it.origem == OrigemSonda && (tr.Status < 200 || tr.Status > 299) {
		e.resumo.Descartadas++
		return
	}
	if len(e.trocas) >= e.cfg.Max {
		return
	}
	e.porForma[f]++
	if mod != "" {
		e.porModelo[mod]++
	}
	e.trocas = append(e.trocas, tr)
}

// buscar faz o GET e devolve a troca. false: não deu para usar (erro,
// timeout, SSE, websocket).
func (e *explorador) buscar(ctx context.Context, it item) (captura.Troca, bool) {
	ctx, cancelar := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancelar()
	u := *e.alvo
	u.Path = strings.TrimRight(e.alvo.Path, "/") + it.caminho
	u.RawPath = ""
	u.RawQuery = it.query
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return captura.Troca{}, false
	}
	req.Header.Set("Accept", "*/*")
	for k, vs := range e.cfg.Cabecalhos {
		req.Header[k] = append([]string(nil), vs...)
	}
	inicio := time.Now()
	resp, err := e.cliente.Do(req)
	if err != nil {
		return captura.Troca{}, false
	}
	defer resp.Body.Close()
	tipo, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if tipo == "text/event-stream" || resp.StatusCode == http.StatusSwitchingProtocols {
		return captura.Troca{}, false
	}
	corpo, err := io.ReadAll(io.LimitReader(resp.Body, e.cfg.MaxCorpo+1))
	if err != nil && !errors.Is(err, io.EOF) {
		return captura.Troca{}, false
	}
	truncado := int64(len(corpo)) > e.cfg.MaxCorpo
	if truncado {
		corpo = corpo[:e.cfg.MaxCorpo]
	}
	cabReq := req.Header.Clone()
	cabReq.Set("Host", e.alvo.Host)
	return captura.Troca{
		Horario:               inicio.UTC(),
		Metodo:                http.MethodGet,
		Caminho:               it.caminho,
		Query:                 it.query,
		CabecalhosRequisicao:  captura.Mascarar(cabReq),
		CorpoRequisicao:       []byte{},
		Status:                resp.StatusCode,
		CabecalhosResposta:    captura.Mascarar(resp.Header),
		CorpoResposta:         corpo,
		CorpoRespostaTruncado: truncado,
		LatenciaMs:            float64(time.Since(inicio).Microseconds()) / 1000,
	}, true
}

// preencherModelos põe na fila os modelos preenchidos com os valores vistos.
// Devolve se pôs algo novo.
func (e *explorador) preencherModelos() bool {
	antes := len(e.fila)
	for _, m := range e.modelos {
		if e.porModelo[m.Modelo()] >= e.cfg.PorRota {
			continue
		}
		for _, c := range e.candidatosModelo(m) {
			e.enfileirar(item{caminho: c, origem: OrigemModelo})
		}
	}
	return len(e.fila) > antes
}

// candidatosModelo devolve até PorRota caminhos concretos para o modelo. Um
// prefixo ainda não visitado que casa com uma rota conhecida vai para a fila,
// para os valores dele aparecerem na próxima rodada.
func (e *explorador) candidatosModelo(m Rota) []string {
	parciais := []string{""}
	for _, s := range m.Segmentos {
		var prox []string
		for _, p := range parciais {
			if !s.ehParametro() {
				prox = append(prox, p+"/"+s.Literal)
				continue
			}
			prefixo := p
			if prefixo == "" {
				prefixo = "/"
			}
			vals := e.vals.candidatos(prefixo, s.Parametro)
			if len(vals) == 0 && !e.vistos[prefixo+"?"] {
				e.enfileirar(item{caminho: prefixo, origem: OrigemModelo})
			}
			for _, v := range vals {
				prox = append(prox, p+"/"+v)
				if len(prox) >= e.cfg.PorRota*e.cfg.PorRota {
					break
				}
			}
		}
		parciais = prox
		if len(parciais) == 0 {
			return nil
		}
	}
	if len(parciais) > e.cfg.PorRota {
		parciais = parciais[:e.cfg.PorRota]
	}
	return parciais
}
