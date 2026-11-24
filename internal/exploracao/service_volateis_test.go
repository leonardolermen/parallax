package exploracao

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestVolateisEntre(t *testing.T) {
	casos := []struct {
		a, b     string
		esperado []string
	}{
		{`{"a":1}`, `{"a":1}`, nil},
		{`{"a":1,"em":"x"}`, `{"a":1,"em":"y"}`, []string{"$.em"}},
		{`{"l":[{"v":1},{"v":2}]}`, `{"l":[{"v":1},{"v":3}]}`, []string{"$.l[1].v"}},
		{`{"l":[1]}`, `{"l":[1,2]}`, []string{"$.l"}},
		{`{"a":1}`, `{"b":1}`, []string{"$.a", "$.b"}},
		{"<p>1</p>", "<p>2</p>", []string{"$"}},
	}
	for _, c := range casos {
		if got := volateisEntre([]byte(c.a), []byte(c.b)); !reflect.DeepEqual(got, c.esperado) {
			t.Errorf("volateisEntre(%s, %s) = %v; esperado %v", c.a, c.b, got, c.esperado)
		}
	}
}

func TestExplorarMarcaCamposQueMudamSozinhos(t *testing.T) {
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"nome":"fixo","chamada":%d}`, n.Add(1))
	}))
	defer srv.Close()
	cfg := ConfigPadrao()
	cfg.Alvo = srv.URL
	trocas, resumo, err := Explorar(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(trocas) == 0 || !reflect.DeepEqual(trocas[0].CamposVolateis, []string{"$.chamada"}) || resumo.ComVolateis != len(trocas) {
		t.Fatalf("trocas[0].CamposVolateis = %v, resumo %+v", trocas[0].CamposVolateis, resumo)
	}
}
