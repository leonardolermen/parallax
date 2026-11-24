package exploracao

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// servicoFalso é um serviço com página, JavaScript, lista e detalhe em JSON,
// rota só no código, SSE, logout e uma imagem.
func servicoFalso(t *testing.T, visitas *[]string) *httptest.Server {
	mux := http.NewServeMux()
	reg := func(p string, h http.HandlerFunc) {
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			*visitas = append(*visitas, r.URL.Path)
			h(w, r)
		})
	}
	reg("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<a href="/sobre">sobre</a> <a href="/logout">sair</a> <a href="https://externo.com/x">x</a>
<img src="/logo.png"><script src="/app.js"></script>`)
	})
	reg("GET /sobre", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "sobre")
	})
	reg("GET /logout", func(w http.ResponseWriter, r *http.Request) { t.Error("visitou /logout") })
	reg("GET /logo.png", func(w http.ResponseWriter, r *http.Request) { t.Error("visitou a imagem") })
	reg("GET /app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		fmt.Fprint(w, "const l = await fetch('/api/items'); const d = await fetch(`/api/items/${item.id}`); new EventSource('/events');")
	})
	reg("GET /api/items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[{"id":1,"nome":"a"},{"id":2,"nome":"b"},{"id":3},{"id":4},{"id":5}]}`)
	})
	reg("GET /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%s,"dono":"/api/users/ana"}`, r.PathValue("id"))
	})
	reg("GET /api/users/{user}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"user":%q}`, r.PathValue("user"))
	})
	reg("GET /api/so-no-codigo", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	reg("GET /events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestExplorarAchaRotasPorTodasAsFontes(t *testing.T) {
	var visitas []string
	srv := servicoFalso(t, &visitas)
	dir := t.TempDir()
	codigo := "mux.HandleFunc(\"GET /api/so-no-codigo\", h)\nmux.HandleFunc(\"GET /api/nao-existe\", h)\nmux.HandleFunc(\"POST /api/escrita\", h)\n"
	if err := os.WriteFile(filepath.Join(dir, "rotas.go"), []byte(codigo), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := ConfigPadrao()
	cfg.Alvo = srv.URL
	cfg.Fontes = []string{dir}
	cfg.Cabecalhos = http.Header{"Authorization": {"Bearer segredo"}}
	trocas, resumo, err := Explorar(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	var caminhos []string
	for _, tr := range trocas {
		caminhos = append(caminhos, tr.Caminho)
		if got := tr.CabecalhosRequisicao.Get("Authorization"); got != "***" {
			t.Errorf("%s: Authorization gravado como %q", tr.Caminho, got)
		}
	}
	sort.Strings(caminhos)
	esperado := []string{"/", "/api/items", "/api/items/1", "/api/items/2", "/api/items/3", "/api/so-no-codigo", "/api/users/ana", "/sobre"}
	if strings.Join(caminhos, " ") != strings.Join(esperado, " ") {
		t.Fatalf("caminhos = %v\nesperado  %v", caminhos, esperado)
	}
	for _, v := range visitas {
		if v == "/api/escrita" {
			t.Error("visitou rota de POST")
		}
	}
	if resumo.RotasCodigo != 2 || resumo.Gravadas != len(trocas) {
		t.Errorf("resumo = %+v", resumo)
	}
}

func TestExplorarRespeitaMaxEPrefixo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /svc/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		for i := 0; i < 20; i++ {
			fmt.Fprintf(w, `<a href="/svc/p%d">x</a><a href="/svc/n/%d">y</a><a href="/fora">z</a>`, i, i)
		}
	})
	mux.HandleFunc("GET /fora", func(w http.ResponseWriter, r *http.Request) { t.Error("saiu do prefixo") })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cfg := ConfigPadrao()
	cfg.Alvo = srv.URL + "/svc"
	cfg.Max = 10
	trocas, _, err := Explorar(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(trocas) != 10 {
		t.Fatalf("%d trocas; esperado 10", len(trocas))
	}
	numericas := 0
	for _, tr := range trocas {
		if strings.HasPrefix(tr.Caminho, "/svc") {
			t.Errorf("caminho com o prefixo: %s", tr.Caminho)
		}
		if strings.HasPrefix(tr.Caminho, "/n/") {
			numericas++
		}
	}
	if numericas > cfg.PorRota {
		t.Errorf("%d caminhos /n/{}; o limite é %d", numericas, cfg.PorRota)
	}
}

func TestExplorarAlvoInvalido(t *testing.T) {
	cfg := ConfigPadrao()
	cfg.Alvo = "localhost:8080"
	if _, _, err := Explorar(context.Background(), cfg); err == nil {
		t.Fatal("aceitou alvo sem esquema")
	}
}
