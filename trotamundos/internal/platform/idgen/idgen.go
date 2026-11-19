package idgen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"sync/atomic"
	"time"
)

// readRandom se mantiene como variable para poder simular una falla de
// crypto/rand en los tests. En producción apunta siempre a rand.Read.
var readRandom = rand.Read

var fallbackCounter atomic.Uint64

// UUID devuelve un UUID v4. Nunca hace panic: si crypto/rand falla luego de
// varios reintentos, se deriva un valor del hash de tiempo+PID+contador.
func UUID() string {
	var b [16]byte
	fillRandom(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}

// Token devuelve un token aleatorio de nBytes bytes en hexadecimal.
func Token(nBytes int) string {
	if nBytes <= 0 {
		nBytes = 32
	}
	b := make([]byte, nBytes)
	fillRandom(b)
	return hex.EncodeToString(b)
}

func fillRandom(b []byte) {
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := readRandom(b); err == nil {
			return
		}
	}
	fallbackFill(b)
}

// fallbackFill genera bytes sin depender de crypto/rand usando SHA-256 sobre
// tiempo, PID y un contador atómico. No es criptográficamente seguro, pero es
// suficiente como último recurso para evitar un panic que tumbaría el proceso;
// el contador garantiza unicidad dentro del proceso incluso si el reloj no
// avanza.
func fallbackFill(dst []byte) {
	var seed [32]byte
	for len(dst) > 0 {
		h := sha256.New()
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(time.Now().UnixNano()))
		_, _ = h.Write(buf[:])
		binary.LittleEndian.PutUint64(buf[:], uint64(os.Getpid()))
		_, _ = h.Write(buf[:])
		binary.LittleEndian.PutUint64(buf[:], fallbackCounter.Add(1))
		_, _ = h.Write(buf[:])
		_, _ = h.Write(seed[:])

		sum := h.Sum(nil)
		n := copy(dst, sum)
		dst = dst[n:]
		copy(seed[:], sum)
	}
}
