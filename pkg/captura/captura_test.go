package captura

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIdaEVoltaJSONL(t *testing.T) {
	trocas := []Troca{
		{
			Horario: time.Date(2026, 11, 20, 10, 0, 0, 123, time.UTC), Metodo: "GET", Caminho: "/orders/7", Query: "x=1&y=2",
			CabecalhosRequisicao: http.Header{"Accept": {"application/json"}}, CorpoRequisicao: []byte{},
			Status: 200, CabecalhosResposta: http.Header{"Content-Type": {"application/json"}},
			CorpoResposta: []byte(`{"id":7}`), LatenciaMs: 12.5,
		},
		{
			Horario: time.Date(2026, 11, 20, 10, 0, 1, 0, time.UTC), Metodo: "POST", Caminho: "/orders",
			CabecalhosRequisicao: http.Header{}, CorpoRequisicao: []byte("abc"), CorpoRequisicaoTruncado: true,
			Status: 201, CabecalhosResposta: http.Header{}, CorpoResposta: []byte{}, LatenciaMs: 3,
		},
	}
	var buf bytes.Buffer
	e := NovoEscritor(&buf)
	for _, tr := range trocas {
		if err := e.Escrever(tr); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Fechar(); err != nil {
		t.Fatal(err)
	}
	l := NovoLeitor(&buf)
	for i, quer := range trocas {
		tem, err := l.Proxima()
		if err != nil {
			t.Fatalf("troca %d: %v", i, err)
		}
		if !reflect.DeepEqual(tem, quer) {
			t.Fatalf("troca %d:\ntem  %+v\nquer %+v", i, tem, quer)
		}
	}
	if _, err := l.Proxima(); !errors.Is(err, io.EOF) {
		t.Fatalf("terceira leitura: quer io.EOF, tem %v", err)
	}
}

func TestMascarar(t *testing.T) {
	h := http.Header{"Authorization": {"Bearer x"}, "cookie": {"a", "b"}, "Set-Cookie": {"s"}, "X-API-KEY": {"k"}, "Accept": {"*/*"}}
	m := Mascarar(h)
	for _, nome := range []string{"Authorization", "Set-Cookie", "X-API-KEY"} {
		if !reflect.DeepEqual(m[nome], []string{Mascara}) {
			t.Errorf("%s: %v", nome, m[nome])
		}
	}
	if !reflect.DeepEqual(m["cookie"], []string{Mascara, Mascara}) {
		t.Errorf("cookie: %v", m["cookie"])
	}
	if !reflect.DeepEqual(m["Accept"], []string{"*/*"}) {
		t.Errorf("Accept: %v", m["Accept"])
	}
	if h.Get("Authorization") != "Bearer x" {
		t.Error("Mascarar alterou o header de entrada")
	}
	if !Sensivel("x-api-key") || Sensivel("Accept") {
		t.Error("Sensivel")
	}
	if Mascarar(nil) == nil {
		t.Error("Mascarar(nil) deve devolver header vazio")
	}
}

// A linha de exemplo do docs/captura.md: os nomes dos campos não podem mudar.
const linhaDoc = `{"horario":"2026-11-18T04:40:32.8351407Z","metodo":"GET","caminho":"/status.json","query":"q=1","cabecalhos_requisicao":{"Accept-Encoding":["gzip"],"Authorization":["***"],"Host":["127.0.0.1:59273"],"User-Agent":["Go-http-client/1.1"]},"corpo_requisicao":"","status":200,"cabecalhos_resposta":{"Accept-Ranges":["bytes"],"Content-Length":["12"],"Content-Type":["application/json"],"Date":["Wed, 18 Nov 2026 04:40:32 GMT"],"Last-Modified":["Wed, 18 Nov 2026 04:40:32 GMT"]},"corpo_resposta":"eyJvayI6dHJ1ZX0K","latencia_ms":39.4514}`

func TestFormatoEstavel(t *testing.T) {
	tr, err := NovoLeitor(strings.NewReader(linhaDoc + "\n")).Proxima()
	if err != nil {
		t.Fatal(err)
	}
	if string(tr.CorpoResposta) != "{\"ok\":true}\n" || tr.Query != "q=1" || tr.Status != 200 {
		t.Fatalf("troca lida: %+v", tr)
	}
	var buf bytes.Buffer
	e := NovoEscritor(&buf)
	if err := e.Escrever(tr); err != nil {
		t.Fatal(err)
	}
	_ = e.Fechar()
	var a, b map[string]any
	if err := json.Unmarshal([]byte(linhaDoc), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(buf.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("reescrita difere:\n%s", buf.String())
	}
}
