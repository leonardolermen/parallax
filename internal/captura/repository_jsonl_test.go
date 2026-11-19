package captura

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func trocaDeTeste(caminho string) Troca {
	return Troca{
		Horario:              time.Date(2026, 11, 18, 10, 30, 0, 123456789, time.UTC),
		Metodo:               http.MethodGet,
		Caminho:              caminho,
		Query:                "a=1&b=2",
		CabecalhosRequisicao: http.Header{"Accept": {"application/json"}, "Authorization": {"***"}},
		CorpoRequisicao:      []byte{},
		Status:               200,
		CabecalhosResposta:   http.Header{"Content-Type": {"application/json"}},
		CorpoResposta:        []byte(`{"ok":true}`),
		LatenciaMs:           1.25,
	}
}

// compararTrocas compara campo a campo (Horario com Equal).
func compararTrocas(t *testing.T, esperado, obtido Troca) {
	t.Helper()
	if !esperado.Horario.Equal(obtido.Horario) {
		t.Errorf("horario: esperado %v, veio %v", esperado.Horario, obtido.Horario)
	}
	esperado.Horario, obtido.Horario = time.Time{}, time.Time{}
	if !reflect.DeepEqual(esperado, obtido) {
		t.Errorf("troca diferente:\nesperado: %+v\nobtido:   %+v", esperado, obtido)
	}
}

func TestJSONLIdaEVolta(t *testing.T) {
	binaria := trocaDeTeste("/binario")
	binaria.Metodo = http.MethodPost
	binaria.CorpoRequisicao = []byte{0, 255, 10}
	vazia := trocaDeTeste("/vazio")
	vazia.CorpoResposta = []byte{}
	truncada := trocaDeTeste("/truncado")
	truncada.CorpoResposta = []byte("0123456789")
	truncada.CorpoRespostaTruncado = true
	truncada.CorpoRequisicaoTruncado = true
	trocas := []Troca{binaria, vazia, truncada}

	var buf bytes.Buffer
	e := NovoEscritor(&buf)
	for _, tr := range trocas {
		if err := e.Escrever(tr); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Flush(); err != nil {
		t.Fatal(err)
	}

	l := NovoLeitor(&buf)
	for i, esperado := range trocas {
		obtido, err := l.Proxima()
		if err != nil {
			t.Fatalf("troca %d: %v", i, err)
		}
		compararTrocas(t, esperado, obtido)
	}
	if _, err := l.Proxima(); !errors.Is(err, io.EOF) {
		t.Errorf("esperado io.EOF no fim, veio %v", err)
	}
}

func TestJSONLCampos(t *testing.T) {
	var buf bytes.Buffer
	e := NovoEscritor(&buf)
	tr := trocaDeTeste("/campos")
	tr.CorpoResposta = nil // nil também vira ""
	if err := e.Escrever(tr); err != nil {
		t.Fatal(err)
	}
	if err := e.Flush(); err != nil {
		t.Fatal(err)
	}
	linha := buf.String()
	for _, trecho := range []string{
		`"horario":"2026-11-18T10:30:00.123456789Z"`,
		`"corpo_resposta":""`,
		`"corpo_requisicao":""`,
		`"latencia_ms":1.25`,
		`"query":"a=1&b=2"`,
	} {
		if !strings.Contains(linha, trecho) {
			t.Errorf("linha sem %s: %s", trecho, linha)
		}
	}
	if strings.Contains(linha, "_truncado") {
		t.Errorf("campos _truncado deveriam ser omitidos quando false: %s", linha)
	}
	if !strings.HasSuffix(linha, "}\n") || strings.Count(linha, "\n") != 1 {
		t.Errorf("esperada uma única linha terminada em \\n: %q", linha)
	}
}

func TestJSONLLinhaGrande(t *testing.T) {
	tr := trocaDeTeste("/grande")
	tr.CorpoResposta = bytes.Repeat([]byte("abcdefgh"), 1<<17) // 1 MiB
	var buf bytes.Buffer
	e := NovoEscritor(&buf)
	if err := e.Escrever(tr); err != nil {
		t.Fatal(err)
	}
	if err := e.Flush(); err != nil {
		t.Fatal(err)
	}
	obtido, err := NovoLeitor(&buf).Proxima()
	if err != nil {
		t.Fatal(err)
	}
	compararTrocas(t, tr, obtido)
}

func TestJSONLLinhaInvalida(t *testing.T) {
	var buf bytes.Buffer
	e := NovoEscritor(&buf)
	if err := e.Escrever(trocaDeTeste("/ok")); err != nil {
		t.Fatal(err)
	}
	if err := e.Flush(); err != nil {
		t.Fatal(err)
	}
	buf.WriteString("\n{quebrado\n")
	l := NovoLeitor(&buf)
	if _, err := l.Proxima(); err != nil {
		t.Fatalf("primeira linha: %v", err)
	}
	_, err := l.Proxima()
	if err == nil || !strings.Contains(err.Error(), "linha 3") {
		t.Fatalf("esperado erro citando a linha 3, veio %v", err)
	}
}

func TestJSONLUltimaLinhaSemQuebra(t *testing.T) {
	var buf bytes.Buffer
	e := NovoEscritor(&buf)
	if err := e.Escrever(trocaDeTeste("/a")); err != nil {
		t.Fatal(err)
	}
	if err := e.Escrever(trocaDeTeste("/b")); err != nil {
		t.Fatal(err)
	}
	if err := e.Flush(); err != nil {
		t.Fatal(err)
	}
	texto := strings.TrimSuffix(buf.String(), "\n") + "\n\n   \n"
	texto = strings.Replace(texto, "\n", "\n\n", 1) // linha em branco no meio
	texto = strings.TrimRight(texto, "\n ")         // última sem \n
	l := NovoLeitor(strings.NewReader(texto))
	var caminhos []string
	for {
		tr, err := l.Proxima()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		caminhos = append(caminhos, tr.Caminho)
	}
	if !reflect.DeepEqual(caminhos, []string{"/a", "/b"}) {
		t.Errorf("caminhos = %v", caminhos)
	}
}

func TestEscritorConcorrente(t *testing.T) {
	var buf bytes.Buffer
	e := NovoEscritor(&buf)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tr := trocaDeTeste(fmt.Sprintf("/c/%d", i))
			tr.CorpoResposta = bytes.Repeat([]byte{byte(i)}, 5000)
			if err := e.Escrever(tr); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if err := e.Flush(); err != nil {
		t.Fatal(err)
	}
	l := NovoLeitor(&buf)
	vistos := map[string]bool{}
	for {
		tr, err := l.Proxima()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		vistos[tr.Caminho] = true
	}
	if len(vistos) != 50 {
		t.Errorf("esperadas 50 trocas distintas, vieram %d", len(vistos))
	}
}

func TestEscritorFechado(t *testing.T) {
	var buf bytes.Buffer
	e := NovoEscritor(&buf)
	if err := e.Escrever(trocaDeTeste("/antes")); err != nil {
		t.Fatal(err)
	}
	if err := e.Fechar(); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Error("Fechar deveria fazer flush")
	}
	if err := e.Escrever(trocaDeTeste("/depois")); !errors.Is(err, ErrEscritorFechado) {
		t.Errorf("esperado ErrEscritorFechado, veio %v", err)
	}
	if err := e.Fechar(); err != nil {
		t.Errorf("segundo Fechar: %v", err)
	}
}
