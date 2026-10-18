package devserver

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
)

func newID() string              { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func formatUint(n uint64) string { return strconv.FormatUint(n, 10) }
