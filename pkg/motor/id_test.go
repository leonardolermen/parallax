package motor

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNovoIDFormato(t *testing.T) {
	a, b := NovoID(), NovoID()
	for _, id := range []string{a, b} {
		if len(id) != 26 {
			t.Errorf("id %q com %d caracteres, esperado 26", id, len(id))
		}
		for _, c := range id {
			if !strings.ContainsRune(alfabetoCrockford, c) {
				t.Errorf("id %q com caractere fora do alfabeto: %q", id, c)
			}
		}
	}
	if a == b {
		t.Errorf("dois ids seguidos iguais: %s", a)
	}
}

func TestNovoIDEmConhecido(t *testing.T) {
	// Vetor da especificação ULID: 1469918176385 ms → "01ARYZ6S41".
	id, err := NovoIDEm(time.UnixMilli(1469918176385), bytes.NewReader(make([]byte, 10)))
	if err != nil {
		t.Fatal(err)
	}
	if id != "01ARYZ6S41"+"0000000000000000" {
		t.Errorf("id = %s", id)
	}
}

func TestNovoIDEmMaximo(t *testing.T) {
	id, err := NovoIDEm(time.UnixMilli(1<<48-1), bytes.NewReader(bytes.Repeat([]byte{0xFF}, 10)))
	if err != nil {
		t.Fatal(err)
	}
	if id != "7ZZZZZZZZZZZZZZZZZZZZZZZZZ" {
		t.Errorf("id = %s", id)
	}
}

func TestNovoIDOrdenado(t *testing.T) {
	base := time.UnixMilli(1700000000000)
	a, err := NovoIDEm(base, bytes.NewReader(bytes.Repeat([]byte{7}, 10)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NovoIDEm(base.Add(time.Millisecond), bytes.NewReader(bytes.Repeat([]byte{7}, 10)))
	if err != nil {
		t.Fatal(err)
	}
	if !(a < b) {
		t.Errorf("esperado %s < %s", a, b)
	}
}

type leitorQueFalha struct{}

func (leitorQueFalha) Read([]byte) (int, error) { return 0, errors.New("sem entropia") }

func TestNovoIDEmErroAleatorio(t *testing.T) {
	if _, err := NovoIDEm(time.Now(), leitorQueFalha{}); err == nil {
		t.Error("esperado erro quando o leitor aleatório falha")
	}
}
