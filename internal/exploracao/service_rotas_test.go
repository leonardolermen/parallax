package exploracao

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAnalisarModelo(t *testing.T) {
	casos := map[string]string{
		"/api/projects/{projeto}":              "/api/projects/{projeto}",
		"/files/{path...}":                     "/files/{path}",
		"/users/:id":                           "/users/{id}",
		"/users/<int:user_id>/posts":           "/users/{user_id}/posts",
		"/api/x/${encodeURIComponent(p.name)}": "/api/x/{name}",
		"/{$}":                                 "/",
		"/static/":                             "/static/",
		"/a?b=1":                               "/a",
	}
	for entrada, esperado := range casos {
		r, ok := AnalisarModelo(entrada)
		if !ok || r.Modelo() != esperado {
			t.Errorf("AnalisarModelo(%q) = %q, %v; esperado %q", entrada, r.Modelo(), ok, esperado)
		}
	}
	for _, ruim := range []string{"", "x/y", "//cdn.com/a", "/a b", "/static/*", "/a/\"b"} {
		if _, ok := AnalisarModelo(ruim); ok {
			t.Errorf("AnalisarModelo(%q) aceitou", ruim)
		}
	}
}

func TestRotaCasa(t *testing.T) {
	r, _ := AnalisarModelo("/api/projects/{p}/tasks")
	if !r.Casa("/api/projects/solaris/tasks") || r.Casa("/api/projects/solaris") || r.Casa("/api/projects//tasks") {
		t.Fatal("Casa errado")
	}
}

func modelos(rs []Rota) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Modelo())
	}
	return out
}

func TestRotasDaLinha(t *testing.T) {
	casos := map[string][]string{
		`mux.HandleFunc("GET /api/projects/{projeto}", h.projeto)`: {"/api/projects/{projeto}"},
		`mux.HandleFunc("POST /api/projects", h.criar)`:            nil,
		`mux.Handle("/static/", fs)`:                               {"/static/"},
		`app.get('/users/:id', (req, res) => {})`:                  {"/users/{id}"},
		`router.post('/users', criar)`:                             nil,
		`@app.route("/items/<int:item_id>")`:                       {"/items/{item_id}"},
		`@app.route("/items", methods=["POST"])`:                   nil,
		`@GetMapping("/orders/{id}")`:                              {"/orders/{id}"},
		`@PostMapping("/orders")`:                                  nil,
		"const r = await fetch(`/api/tasks/${t.id}/log`)":          {"/api/tasks/{id}/log"},
		`fetch("/api/save", {method: "POST"})`:                     nil,
		`x := "/etc/passwd"`:                                       nil, // sem registro de rota na linha
	}
	for linha, esperado := range casos {
		if got := modelos(RotasDaLinha(linha)); !reflect.DeepEqual(got, esperado) {
			t.Errorf("RotasDaLinha(%s) = %v; esperado %v", linha, got, esperado)
		}
	}
}

func TestLerRotasDoCodigoIgnoraTestesEDependencias(t *testing.T) {
	dir := t.TempDir()
	escrever := func(nome, conteudo string) {
		p := filepath.Join(dir, filepath.FromSlash(nome))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escrever("main.go", "mux.HandleFunc(\"GET /a\", a)\nmux.HandleFunc(\"GET /a\", a)\n")
	escrever("main_test.go", `mux.HandleFunc("GET /teste", a)`)
	escrever("node_modules/x/index.js", `app.get('/dep', f)`)
	escrever("web/app.js", "fetch('/b')")
	rs, err := LerRotasDoCodigo([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if got := modelos(rs); !reflect.DeepEqual(got, []string{"/a", "/b"}) {
		t.Fatalf("rotas = %v", got)
	}
}

func TestExcluirPadrao(t *testing.T) {
	cfg := ConfigPadrao()
	for _, p := range []string{"/logout", "/api/sessao/logout", "/api/x/delete-all", "/alerts/3/toggle", "/tasks/9/fixci",
		"/tasks/9/responder", "/servicos/loja/start", "/api/x/run-now", "/api/obs/anomalies/2/analyze", "/sair"} {
		if !cfg.Excluir.MatchString(p) {
			t.Errorf("não excluiu %s", p)
		}
	}
	for _, p := range []string{"/", "/api/projects", "/api/obs/alerts", "/fluxos", "/api/runs/3", "/api/runs", "/api/removals-report/x", "/starts/1", "/importantes", "/api/merges"} {
		if cfg.Excluir.MatchString(p) {
			t.Errorf("excluiu %s", p)
		}
	}
}
