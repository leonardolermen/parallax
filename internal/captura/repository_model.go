// Package captura grava o tráfego real: o gravador (proxy reverso) que
// produz a captura. O formato (Troca, leitura e escrita em JSONL, máscara)
// fica em pkg/captura, público; aqui ficam aliases para o resto do Parallax
// não mudar.
//
// Não lê flags nem escreve no terminal: recebe configuração por structs e
// devolve valores e erros. A CLI (cmd/parallax) é uma camada fina por cima.
package captura

import pub "github.com/leonardolermen/parallax/pkg/captura"

// Troca é pkg/captura.Troca.
type Troca = pub.Troca

// Escritor e Leitor são os de pkg/captura.
type (
	Escritor = pub.Escritor
	Leitor   = pub.Leitor
)

// ErrEscritorFechado é pkg/captura.ErrEscritorFechado.
var ErrEscritorFechado = pub.ErrEscritorFechado

// NovoEscritor e NovoLeitor são os de pkg/captura.
var (
	NovoEscritor = pub.NovoEscritor
	NovoLeitor   = pub.NovoLeitor
)
