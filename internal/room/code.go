package room

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"strings"
)

// codeAlphabet omits characters that get misheard or misread when someone is
// shouting a room code across a table: O/0, I/1, S/5.
const codeAlphabet = "ABCDEFGHJKLMNPQRTUVWXYZ2346789"

const codeLength = 4

// NewCode returns a random room code.
func NewCode() string {
	var sb strings.Builder
	for i := 0; i < codeLength; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			// crypto/rand does not fail in practice; degrade rather than panic.
			sb.WriteByte(codeAlphabet[i%len(codeAlphabet)])
			continue
		}
		sb.WriteByte(codeAlphabet[n.Int64()])
	}
	return sb.String()
}

// NormalizeCode makes user-typed codes forgiving about case, spaces and dashes.
// Characters outside the alphabet are dropped rather than guessed at — a wrong
// code should fail as "no such room", not silently open someone else's game.
func NormalizeCode(s string) string {
	var sb strings.Builder
	for _, c := range strings.ToUpper(strings.TrimSpace(s)) {
		if strings.ContainsRune(codeAlphabet, c) {
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// newToken returns the secret a browser stores in localStorage to reclaim its
// seat after a refresh or a dropped connection.
func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "t" + NewCode() + NewCode()
	}
	return hex.EncodeToString(b)
}
