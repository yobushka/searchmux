package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

// AuthMiddleware authenticates gateway tokens, never upstream credentials.
// An empty hash list denies every request.
func AuthMiddleware(tokenHashes []string, next http.Handler) http.Handler {
	hashes := make([][]byte, 0, len(tokenHashes))
	for _, encoded := range tokenHashes {
		decoded, err := hex.DecodeString(encoded)
		if err == nil && len(decoded) == sha256.Size {
			hashes = append(hashes, decoded)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		valid := 0
		if len(r.Header.Values("Authorization")) == 1 && len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			digest := sha256.Sum256([]byte(parts[1]))
			for _, hash := range hashes {
				valid |= subtle.ConstantTimeCompare(digest[:], hash)
			}
		}
		if valid != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
