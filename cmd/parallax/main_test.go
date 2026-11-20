package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leonardolermen/parallax/internal/captura"
)

func rodar(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	codigo := run(args, &stdout, &stderr)
	return codigo, stdout.String(), stderr.String()
}

func TestVersion(t *testing.T) {
	codigo, stdout, _ := rodar("version")
	if codigo != 0 || stdout != "parallax dev\n" {
		t.Errorf("version: código %d, saída %q", codigo, stdout)
	}
}

func TestHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		codigo, stdout, _ := rodar(arg)
		if codigo != 0 {
			t.Errorf("%s: código %d", arg, codigo)
		}
		for _, cmd := range []string{"record", "replay", "check", "history", "version"} {
			if !strings.Contains(stdout, cmd) {
				t.Errorf("%s: ajuda sem %q", arg, cmd)
			}
		}
	}
	codigo, _, stderr := rodar()
	if codigo != 4 || !strings.Contains(stderr, "record") {
		t.Errorf("sem argumentos: código %d, stderr %q", codigo, stderr)
	}
	codigo, _, stderr = rodar("xyz")
	if codigo != 4 || !strings.Contains(stderr, "comando desconhecido") {
		t.Errorf("xyz: código %d, stderr %q", codigo, stderr)
	}
}

func TestCheckSemFlags(t *testing.T) {
	codigo, _, stderr := rodar("check")
	if codigo != 4 || !strings.Contains(stderr, "falta --capture") {
		t.Errorf("check: código %d, stderr %q", codigo, stderr)
	}
}

func TestRecordFlagsInvalidas(t *testing.T) {
	out := filepath.Join(t.TempDir(), "c.jsonl")
	casos := []struct {
		nome string
		args []string
	}{
		{"sem upstream", []string{"--out", out}},
		{"sem out", []string{"--upstream", "http://localhost:8080"}},
		{"upstream ftp", []string{"--upstream", "ftp://x", "--out", out}},
		{"upstream sem host", []string{"--upstream", "http://", "--out", out}},
		{"sample acima de 1", []string{"--upstream", "http://localhost:8080", "--out", out, "--sample", "1.5"}},
		{"sample negativo", []string{"--upstream", "http://localhost:8080", "--out", out, "--sample", "-0.5"}},
		{"max negativo", []string{"--upstream", "http://localhost:8080", "--out", out, "--max", "-1"}},
		{"max-body negativo", []string{"--upstream", "http://localhost:8080", "--out", out, "--max-body", "-1"}},
		{"flag desconhecida", []string{"--upstream", "http://localhost:8080", "--out", out, "--xyz"}},
		{"argumento sobrando", []string{"--upstream", "http://localhost:8080", "--out", out, "sobra"}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			codigo, _, stderr := rodar(append([]string{"record"}, c.args...)...)
			if codigo != 4 {
				t.Errorf("código %d, esperado 4", codigo)
			}
			if strings.TrimSpace(stderr) == "" {
				t.Error("stderr vazio, esperada mensagem de erro")
			}
		})
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("flags inválidas não deveriam criar o arquivo de saída (stat: %v)", err)
	}
}

func TestRecordPontaAPonta(t *testing.T) {
	for _, comLixo := range []bool{false, true} {
		nome := "arquivo novo"
		if comLixo {
			nome = "arquivo existente é sobrescrito"
		}
		t.Run(nome, func(t *testing.T) { gravarPontaAPonta(t, comLixo) })
	}
}

func gravarPontaAPonta(t *testing.T, comLixo bool) {
	var mu sync.Mutex
	var auths []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		io.WriteString(w, "olá "+r.URL.Path)
	}))
	defer up.Close()

	out := filepath.Join(t.TempDir(), "c.jsonl")
	if comLixo {
		if err := os.WriteFile(out, []byte("lixo que não é JSON\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	enderecos := make(chan string, 1)
	var stderr bytes.Buffer // escrito só pela goroutine de gravar; lido depois que ela termina
	fim := make(chan int, 1)
	go func() {
		fim <- gravar(ctx, []string{"--listen", "127.0.0.1:0", "--upstream", up.URL, "--out", out, "--max", "5"},
			func(endereco string) { enderecos <- endereco }, &stderr)
	}()
	var endereco string
	select {
	case endereco = <-enderecos:
	case codigo := <-fim:
		t.Fatalf("gravar terminou antes de ficar pronto: código %d, stderr %q", codigo, stderr.String())
	case <-time.After(10 * time.Second):
		t.Fatal("gravar não ficou pronto")
	}

	cliente := &http.Client{Transport: &http.Transport{}}
	for _, caminho := range []string{"/um", "/dois"} {
		req, _ := http.NewRequest(http.MethodGet, "http://"+endereco+caminho+"?q=1", nil)
		req.Header.Set("Authorization", "Bearer segredo")
		resp, err := cliente.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		corpo, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || string(corpo) != "olá "+caminho {
			t.Fatalf("%s: %d %q", caminho, resp.StatusCode, corpo)
		}
	}
	cliente.CloseIdleConnections()

	cancelar()
	select {
	case codigo := <-fim:
		if codigo != 0 {
			t.Fatalf("gravar devolveu %d, stderr %q", codigo, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("gravar não encerrou depois do cancelamento")
	}
	if !strings.Contains(stderr.String(), "2 trocas gravadas") {
		t.Errorf("stderr sem \"2 trocas gravadas\": %q", stderr.String())
	}

	mu.Lock()
	if !reflect.DeepEqual(auths, []string{"Bearer segredo", "Bearer segredo"}) {
		t.Errorf("upstream recebeu Authorization %v", auths)
	}
	mu.Unlock()

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	l := captura.NovoLeitor(f)
	var caminhos []string
	for {
		tr, err := l.Proxima()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("captura inválida: %v", err)
		}
		caminhos = append(caminhos, tr.Caminho)
		if got := tr.CabecalhosRequisicao.Values("Authorization"); !reflect.DeepEqual(got, []string{"***"}) {
			t.Errorf("Authorization gravado %v", got)
		}
		if tr.Query != "q=1" || tr.Status != 200 {
			t.Errorf("troca gravada: %+v", tr)
		}
	}
	if !reflect.DeepEqual(caminhos, []string{"/um", "/dois"}) {
		t.Errorf("caminhos gravados %v", caminhos)
	}
}
