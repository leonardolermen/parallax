package platform

import (
	"crypto/rand"
	"fmt"
	"io"
	"time"
)

// alfabetoCrockford é o base32 de Crockford usado pelo ULID (sem I, L, O, U).
const alfabetoCrockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NovoID gera o id de uma execução: um ULID de 26 caracteres (48 bits de
// milissegundos desde a época Unix + 80 bits de crypto/rand), ordenável por
// tempo. Cada execução fica em .parallax/runs/<id>/.
//
// Dá panic se crypto/rand falhar, o que não acontece na prática.
func NovoID() string {
	id, err := NovoIDEm(time.Now(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return id
}

// NovoIDEm gera um ULID para o instante t, lendo os 10 bytes aleatórios de
// aleatorio. Útil em testes.
func NovoIDEm(t time.Time, aleatorio io.Reader) (string, error) {
	var bruto [16]byte
	ms := uint64(t.UnixMilli())
	for i := 0; i < 6; i++ {
		bruto[i] = byte(ms >> (8 * (5 - i)))
	}
	if _, err := io.ReadFull(aleatorio, bruto[6:]); err != nil {
		return "", fmt.Errorf("motor: gerar id: %w", err)
	}
	// 128 bits viram 26 símbolos de 5 bits (130 bits): o primeiro símbolo
	// usa só os 3 bits mais altos.
	var saida [26]byte
	for i := range saida {
		inicio := i*5 - 2
		v := 0
		for b := inicio; b < inicio+5; b++ {
			v <<= 1
			if b >= 0 {
				v |= int(bruto[b/8]>>(7-uint(b%8))) & 1
			}
		}
		saida[i] = alfabetoCrockford[v]
	}
	return string(saida[:]), nil
}
